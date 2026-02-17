package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	providerconfig "github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/config"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
	"github.com/tiger/realtime-speech-pipeline/providers/common/streamsse"
)

const ProviderID = "llm-gemini"

type Config struct {
	APIKey   string
	Endpoint string
	Prompt   string
	Timeout  time.Duration
}

type Adapter struct {
	cfg   Config
	unary *httpadapter.Adapter
	http  *http.Client
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:   providerconfig.ResolveEnvValue("RSPP_LLM_GEMINI_API_KEY", "RSPP_LLM_GEMINI_API_KEY_REF", ""),
		Endpoint: providerconfig.ResolveEnvValue("RSPP_LLM_GEMINI_ENDPOINT", "RSPP_LLM_GEMINI_ENDPOINT_REF", "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent"),
		Prompt:   defaultString(os.Getenv("RSPP_LLM_GEMINI_PROMPT"), "Reply with the word: ok"),
		Timeout:  10 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	unary, err := httpadapter.New(httpadapter.Config{
		ProviderID:       ProviderID,
		Modality:         contracts.ModalityLLM,
		Endpoint:         cfg.Endpoint,
		APIKey:           cfg.APIKey,
		QueryAPIKeyParam: "key",
		Timeout:          cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"contents": []map[string]any{
					{"parts": []map[string]any{{"text": cfg.Prompt}}},
				},
			}
		},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{
		cfg:   cfg,
		unary: unary,
		http:  &http.Client{},
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
		"contents": []map[string]any{
			{"parts": []map[string]any{{"text": a.cfg.Prompt}}},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return contracts.Outcome{}, err
	}
	inputPayload, inputTruncated := httpadapter.CapturePayload(body, false)

	streamEndpoint := strings.Replace(a.cfg.Endpoint, ":generateContent", ":streamGenerateContent", 1)
	streamEndpoint, err = httpadapter.WithQuery(streamEndpoint, "alt", "sse")
	if err != nil {
		return contracts.Outcome{}, err
	}
	if a.cfg.APIKey != "" {
		streamEndpoint, err = httpadapter.WithQuery(streamEndpoint, "key", a.cfg.APIKey)
		if err != nil {
			return contracts.Outcome{}, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, streamEndpoint, bytes.NewReader(body))
	if err != nil {
		return contracts.Outcome{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

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
		delta, parseErr := geminiDelta(ev.Data)
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

func geminiDelta(raw string) (string, error) {
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", err
	}
	if len(payload.Candidates) == 0 || len(payload.Candidates[0].Content.Parts) == 0 {
		return "", nil
	}
	return payload.Candidates[0].Content.Parts[0].Text, nil
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
