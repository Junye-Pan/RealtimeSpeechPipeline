package cohere

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	providerconfig "github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/config"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
	"github.com/tiger/realtime-speech-pipeline/providers/common/streamsse"
)

const ProviderID = "llm-cohere"

type Config struct {
	APIKey            string
	Endpoint          string
	Model             string
	Prompt            string
	OpenRouter        bool
	OpenRouterReferer string
	OpenRouterTitle   string
	MaxTokens         int
	Timeout           time.Duration
}

type Adapter struct {
	cfg         Config
	openRouter  bool
	unary       *httpadapter.Adapter
	http        *http.Client
	staticHeads map[string]string
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:            providerconfig.ResolveEnvValue("RSPP_LLM_COHERE_API_KEY", "RSPP_LLM_COHERE_API_KEY_REF", ""),
		Endpoint:          providerconfig.ResolveEnvValue("RSPP_LLM_COHERE_ENDPOINT", "RSPP_LLM_COHERE_ENDPOINT_REF", "https://openrouter.ai/api/v1/chat/completions"),
		Model:             defaultString(os.Getenv("RSPP_LLM_COHERE_MODEL"), "cohere/command-r-08-2024"),
		Prompt:            defaultString(os.Getenv("RSPP_LLM_COHERE_PROMPT"), "Reply with the word: ok"),
		OpenRouter:        defaultBool(os.Getenv("RSPP_LLM_COHERE_OPENROUTER"), false),
		OpenRouterReferer: os.Getenv("RSPP_LLM_COHERE_OPENROUTER_REFERER"),
		OpenRouterTitle:   defaultString(os.Getenv("RSPP_LLM_COHERE_OPENROUTER_TITLE"), "RealtimeSpeechPipeline"),
		MaxTokens:         16,
		Timeout:           10 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	openRouter := shouldUseOpenRouter(cfg)
	staticHeaders := map[string]string{}
	if openRouter {
		if cfg.OpenRouterReferer != "" {
			staticHeaders["HTTP-Referer"] = cfg.OpenRouterReferer
		}
		if cfg.OpenRouterTitle != "" {
			staticHeaders["X-Title"] = cfg.OpenRouterTitle
		}
	}

	unary, err := httpadapter.New(httpadapter.Config{
		ProviderID:    ProviderID,
		Modality:      contracts.ModalityLLM,
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		APIKeyHeader:  "Authorization",
		APIKeyPrefix:  "Bearer ",
		StaticHeaders: staticHeaders,
		Timeout:       cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			if openRouter {
				return map[string]any{
					"model":      cfg.Model,
					"max_tokens": cfg.MaxTokens,
					"messages": []map[string]any{
						{"role": "user", "content": cfg.Prompt},
					},
				}
			}
			return map[string]any{
				"model": cfg.Model,
				"messages": []map[string]any{
					{"role": "user", "content": cfg.Prompt},
				},
			}
		},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{
		cfg:         cfg,
		openRouter:  openRouter,
		unary:       unary,
		http:        &http.Client{},
		staticHeads: staticHeaders,
	}, nil
}

func NewAdapterFromEnv() (contracts.Adapter, error) {
	return NewAdapter(ConfigFromEnv())
}

func (a *Adapter) ProviderID() string {
	return ProviderID
}

func (a *Adapter) Modality() contracts.Modality {
	return contracts.ModalityLLM
}

func (a *Adapter) Invoke(req contracts.InvocationRequest) (contracts.Outcome, error) {
	return a.unary.Invoke(req)
}

func (a *Adapter) InvokeStream(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
	if observer == nil {
		observer = contracts.NoopStreamObserver{}
	}
	if err := req.Validate(); err != nil {
		return contracts.Outcome{}, err
	}
	if req.CancelRequested {
		outcome := contracts.Outcome{Class: contracts.OutcomeCancelled, Retryable: false, Reason: "provider_cancelled"}
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 0, "", outcome.Reason))
		return outcome, nil
	}

	payload := map[string]any{
		"model":  a.cfg.Model,
		"stream": true,
		"messages": []map[string]any{
			{"role": "user", "content": a.cfg.Prompt},
		},
	}
	if a.openRouter {
		payload["max_tokens"] = a.cfg.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return contracts.Outcome{}, err
	}
	inputPayload, inputTruncated := httpadapter.CapturePayload(body, false)

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return contracts.Outcome{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if a.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	}
	for key, value := range a.staticHeads {
		httpReq.Header.Set(key, value)
	}

	resp, err := a.http.Do(httpReq)
	if err != nil {
		outcome := httpadapter.NormalizeNetworkError(err)
		outcome.InputPayload = inputPayload
		outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(fmt.Sprintf("network_error=%v", err)), false)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		return outcome, nil
	}
	defer resp.Body.Close()

	if err := observer.OnStart(streamChunkFromRequest(req, contracts.StreamChunkStart, 0, "", "")); err != nil {
		return contracts.Outcome{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		sample, sampleTruncated, readErr := httpadapter.ReadBodySample(resp.Body, httpadapter.ResolveProviderIOCaptureMaxBytes())
		if readErr != nil {
			sample = []byte(fmt.Sprintf("response_read_error=%v", readErr))
			sampleTruncated = false
		}
		outcome := httpadapter.NormalizeStatus(resp.StatusCode, resp.Header.Get("Retry-After"))
		outcome.InputPayload = inputPayload
		outputPayload, outputTruncated := httpadapter.CapturePayload(sample, sampleTruncated)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		if outcome.Reason == "" {
			outcome.Reason = "provider_stream_http_error"
		}
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
		return outcome, nil
	}

	finalText := strings.Builder{}
	outputSample := strings.Builder{}
	captureLimit := httpadapter.ResolveProviderIOCaptureMaxBytes()
	sequence := 1
	streamErr := streamsse.Parse(resp.Body, func(ev streamsse.Event) error {
		if ev.Data == "" || ev.Data == "[DONE]" {
			return nil
		}
		appendSample(&outputSample, captureLimit, ev.Data)
		delta, parseErr := cohereDelta(ev.Data)
		if parseErr != nil {
			return parseErr
		}
		if delta == "" {
			return nil
		}
		finalText.WriteString(delta)
		if err := observer.OnChunk(streamChunkFromRequest(req, contracts.StreamChunkDelta, sequence, delta, "")); err != nil {
			return err
		}
		sequence++
		return nil
	})
	if streamErr != nil {
		outcome := contracts.Outcome{
			Class:            contracts.OutcomeInfrastructureFailure,
			Retryable:        true,
			Reason:           "provider_stream_parse_error",
			OutputStatusCode: resp.StatusCode,
		}
		outcome.InputPayload = inputPayload
		outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(fmt.Sprintf("stream_parse_error=%v", streamErr)), false)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, "", outcome.Reason))
		return outcome, nil
	}

	final := streamChunkFromRequest(req, contracts.StreamChunkFinal, sequence, "", "")
	final.TextFinal = finalText.String()
	if err := observer.OnComplete(final); err != nil {
		return contracts.Outcome{}, err
	}

	outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(outputSample.String()), len(outputSample.String()) >= captureLimit)
	outcome := contracts.Outcome{
		Class:            contracts.OutcomeSuccess,
		InputPayload:     inputPayload,
		OutputPayload:    outputPayload,
		OutputStatusCode: resp.StatusCode,
		PayloadTruncated: inputTruncated || outputTruncated,
	}
	return outcome, nil
}

func cohereDelta(raw string) (string, error) {
	var payload struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
		Type  string `json:"type"`
		Delta struct {
			Message struct {
				Content struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", err
	}
	if len(payload.Choices) > 0 {
		if payload.Choices[0].Delta.Content != "" {
			return payload.Choices[0].Delta.Content, nil
		}
		if payload.Choices[0].Message.Content != "" {
			return payload.Choices[0].Message.Content, nil
		}
		if payload.Choices[0].Text != "" {
			return payload.Choices[0].Text, nil
		}
	}
	if payload.Delta.Text != "" {
		return payload.Delta.Text, nil
	}
	if payload.Delta.Message.Content.Text != "" {
		return payload.Delta.Message.Content.Text, nil
	}
	return "", nil
}

func streamChunkFromRequest(req contracts.InvocationRequest, kind contracts.StreamChunkKind, sequence int, delta string, reason string) contracts.StreamChunk {
	chunk := contracts.StreamChunk{
		SessionID:            req.SessionID,
		TurnID:               req.TurnID,
		PipelineVersion:      req.PipelineVersion,
		EventID:              req.EventID,
		ProviderInvocationID: req.ProviderInvocationID,
		ProviderID:           req.ProviderID,
		Modality:             req.Modality,
		Attempt:              req.Attempt,
		Sequence:             sequence,
		RuntimeTimestampMS:   req.RuntimeTimestampMS,
		WallClockTimestampMS: req.WallClockTimestampMS,
		Kind:                 kind,
		TextDelta:            delta,
	}
	if kind == contracts.StreamChunkError {
		chunk.ErrorReason = reason
	}
	return chunk
}

func appendSample(builder *strings.Builder, limit int, part string) {
	if limit < 1 || builder.Len() >= limit {
		return
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	remaining := limit - builder.Len()
	if len(part) > remaining {
		builder.WriteString(part[:remaining])
		return
	}
	builder.WriteString(part)
}

func defaultString(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func defaultBool(v string, fallback bool) bool {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func shouldUseOpenRouter(cfg Config) bool {
	if cfg.OpenRouter {
		return true
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "openrouter.ai")
}
