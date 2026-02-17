package assemblyai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	providerconfig "github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/config"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "stt-assemblyai"

const (
	defaultPollInterval = 1200 * time.Millisecond
	minPollIntervalMS   = 200
	maxPollIntervalMS   = 5000
	pollIntervalEnvVar  = "RSPP_STT_ASSEMBLYAI_POLL_INTERVAL_MS"
)

type Config struct {
	APIKey       string
	Endpoint     string
	AudioURL     string
	SpeechModels []string
	Timeout      time.Duration
	PollInterval time.Duration
}

type Adapter struct {
	cfg   Config
	unary *httpadapter.Adapter
	http  *http.Client
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:       providerconfig.ResolveEnvValue("RSPP_STT_ASSEMBLYAI_API_KEY", "RSPP_STT_ASSEMBLYAI_API_KEY_REF", ""),
		Endpoint:     providerconfig.ResolveEnvValue("RSPP_STT_ASSEMBLYAI_ENDPOINT", "RSPP_STT_ASSEMBLYAI_ENDPOINT_REF", "https://api.assemblyai.com/v2/transcript"),
		AudioURL:     defaultString(os.Getenv("RSPP_STT_ASSEMBLYAI_AUDIO_URL"), "https://static.deepgram.com/examples/Bueller-Life-moves-pretty-fast.wav"),
		SpeechModels: parseSpeechModels(os.Getenv("RSPP_STT_ASSEMBLYAI_SPEECH_MODELS")),
		Timeout:      45 * time.Second,
		PollInterval: pollIntervalFromEnv(),
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	if len(cfg.SpeechModels) == 0 {
		cfg.SpeechModels = []string{"universal-2"}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	unary, err := httpadapter.New(httpadapter.Config{
		ProviderID:    ProviderID,
		Modality:      contracts.ModalitySTT,
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		APIKeyHeader:  "Authorization",
		Timeout:       cfg.Timeout,
		StaticHeaders: map[string]string{"Accept": "application/json"},
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"audio_url":     cfg.AudioURL,
				"speech_models": cfg.SpeechModels,
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
	return contracts.ModalitySTT
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

	requestPayload := map[string]any{
		"audio_url":     a.cfg.AudioURL,
		"speech_models": a.cfg.SpeechModels,
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return contracts.Outcome{}, err
	}
	inputPayload, inputTruncated := httpadapter.CapturePayload(body, false)

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()

	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return contracts.Outcome{}, err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Accept", "application/json")
	if a.cfg.APIKey != "" {
		createReq.Header.Set("Authorization", a.cfg.APIKey)
	}

	createResp, err := a.http.Do(createReq)
	if err != nil {
		outcome := httpadapter.NormalizeNetworkError(err)
		outcome.InputPayload = inputPayload
		outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(fmt.Sprintf("network_error=%v", err)), false)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		return outcome, nil
	}
	defer createResp.Body.Close()

	if err := observer.OnStart(streamChunkFromRequest(req, contracts.StreamChunkStart, 0, "", "")); err != nil {
		return contracts.Outcome{}, err
	}

	createBody, createTruncated, readErr := httpadapter.ReadBodySample(createResp.Body, httpadapter.ResolveProviderIOCaptureMaxBytes()*8)
	if readErr != nil {
		outcome := contracts.Outcome{
			Class:            contracts.OutcomeInfrastructureFailure,
			Retryable:        true,
			Reason:           "provider_response_read_error",
			InputPayload:     inputPayload,
			OutputStatusCode: createResp.StatusCode,
		}
		outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(fmt.Sprintf("response_read_error=%v", readErr)), false)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
		return outcome, nil
	}
	if createResp.StatusCode < 200 || createResp.StatusCode > 299 {
		outcome := httpadapter.NormalizeStatus(createResp.StatusCode, createResp.Header.Get("Retry-After"))
		outcome.InputPayload = inputPayload
		outputPayload, outputTruncated := httpadapter.CapturePayload(createBody, createTruncated)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		if outcome.Reason == "" {
			outcome.Reason = "provider_stream_http_error"
		}
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
		return outcome, nil
	}

	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		outcome := contracts.Outcome{
			Class:            contracts.OutcomeInfrastructureFailure,
			Retryable:        true,
			Reason:           "provider_response_parse_error",
			InputPayload:     inputPayload,
			OutputStatusCode: createResp.StatusCode,
		}
		outputPayload, outputTruncated := httpadapter.CapturePayload(createBody, createTruncated)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
		return outcome, nil
	}
	if created.ID == "" {
		outcome := contracts.Outcome{
			Class:            contracts.OutcomeInfrastructureFailure,
			Retryable:        true,
			Reason:           "provider_transcript_id_missing",
			InputPayload:     inputPayload,
			OutputStatusCode: createResp.StatusCode,
		}
		outputPayload, outputTruncated := httpadapter.CapturePayload(createBody, createTruncated)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
		return outcome, nil
	}

	pollURL := strings.TrimRight(a.cfg.Endpoint, "/") + "/" + created.ID
	sequence := 1
	lastStatus := created.Status

	for {
		status := strings.ToLower(strings.TrimSpace(lastStatus))
		switch status {
		case "completed":
			final := streamChunkFromRequest(req, contracts.StreamChunkFinal, sequence, "", "")
			final.TextFinal = strings.TrimSpace(created.Text)
			if err := observer.OnComplete(final); err != nil {
				return contracts.Outcome{}, err
			}
			outputPayload, outputTruncated := httpadapter.CapturePayload(createBody, createTruncated)
			return contracts.Outcome{
				Class:            contracts.OutcomeSuccess,
				InputPayload:     inputPayload,
				OutputPayload:    outputPayload,
				OutputStatusCode: createResp.StatusCode,
				PayloadTruncated: inputTruncated || outputTruncated,
			}, nil
		case "error", "failed":
			reason := created.Error
			if strings.TrimSpace(reason) == "" {
				reason = "provider_transcription_failed"
			}
			outcome := contracts.Outcome{
				Class:            contracts.OutcomeBlocked,
				Retryable:        false,
				Reason:           reason,
				InputPayload:     inputPayload,
				OutputStatusCode: createResp.StatusCode,
			}
			outputPayload, outputTruncated := httpadapter.CapturePayload(createBody, createTruncated)
			outcome.OutputPayload = outputPayload
			outcome.PayloadTruncated = inputTruncated || outputTruncated
			_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, "", reason))
			return outcome, nil
		}

		meta := streamChunkFromRequest(req, contracts.StreamChunkMetadata, sequence, "", "")
		meta.Metadata = map[string]string{
			"status": lastStatus,
		}
		if err := observer.OnChunk(meta); err != nil {
			return contracts.Outcome{}, err
		}
		sequence++

		select {
		case <-ctx.Done():
			outcome := contracts.Outcome{
				Class:            contracts.OutcomeTimeout,
				Retryable:        true,
				Reason:           "provider_timeout",
				InputPayload:     inputPayload,
				OutputStatusCode: createResp.StatusCode,
			}
			outputPayload, outputTruncated := httpadapter.CapturePayload(createBody, createTruncated)
			outcome.OutputPayload = outputPayload
			outcome.PayloadTruncated = inputTruncated || outputTruncated
			_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, "", outcome.Reason))
			return outcome, nil
		case <-time.After(a.cfg.PollInterval):
		}

		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			return contracts.Outcome{}, err
		}
		pollReq.Header.Set("Accept", "application/json")
		if a.cfg.APIKey != "" {
			pollReq.Header.Set("Authorization", a.cfg.APIKey)
		}
		pollResp, err := a.http.Do(pollReq)
		if err != nil {
			outcome := httpadapter.NormalizeNetworkError(err)
			outcome.InputPayload = inputPayload
			outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(fmt.Sprintf("network_error=%v", err)), false)
			outcome.OutputPayload = outputPayload
			outcome.PayloadTruncated = inputTruncated || outputTruncated
			_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, "", outcome.Reason))
			return outcome, nil
		}

		pollBody, pollTruncated, pollReadErr := httpadapter.ReadBodySample(pollResp.Body, httpadapter.ResolveProviderIOCaptureMaxBytes()*8)
		_ = pollResp.Body.Close()
		if pollReadErr != nil {
			outcome := contracts.Outcome{
				Class:            contracts.OutcomeInfrastructureFailure,
				Retryable:        true,
				Reason:           "provider_response_read_error",
				InputPayload:     inputPayload,
				OutputStatusCode: pollResp.StatusCode,
			}
			outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(fmt.Sprintf("response_read_error=%v", pollReadErr)), false)
			outcome.OutputPayload = outputPayload
			outcome.PayloadTruncated = inputTruncated || outputTruncated
			_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, "", outcome.Reason))
			return outcome, nil
		}
		if pollResp.StatusCode < 200 || pollResp.StatusCode > 299 {
			outcome := httpadapter.NormalizeStatus(pollResp.StatusCode, pollResp.Header.Get("Retry-After"))
			outcome.InputPayload = inputPayload
			outputPayload, outputTruncated := httpadapter.CapturePayload(pollBody, pollTruncated)
			outcome.OutputPayload = outputPayload
			outcome.PayloadTruncated = inputTruncated || outputTruncated
			if outcome.Reason == "" {
				outcome.Reason = "provider_stream_http_error"
			}
			_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, "", outcome.Reason))
			return outcome, nil
		}
		if err := json.Unmarshal(pollBody, &created); err != nil {
			outcome := contracts.Outcome{
				Class:            contracts.OutcomeInfrastructureFailure,
				Retryable:        true,
				Reason:           "provider_response_parse_error",
				InputPayload:     inputPayload,
				OutputStatusCode: pollResp.StatusCode,
			}
			outputPayload, outputTruncated := httpadapter.CapturePayload(pollBody, pollTruncated)
			outcome.OutputPayload = outputPayload
			outcome.PayloadTruncated = inputTruncated || outputTruncated
			_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, "", outcome.Reason))
			return outcome, nil
		}
		lastStatus = created.Status
		createResp.StatusCode = pollResp.StatusCode
		createBody = pollBody
		createTruncated = pollTruncated
	}
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

func defaultString(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func parseSpeechModels(raw string) []string {
	models := make([]string, 0, 2)
	for _, part := range strings.Split(raw, ",") {
		model := strings.TrimSpace(part)
		if model == "" {
			continue
		}
		models = append(models, model)
	}
	if len(models) == 0 {
		return []string{"universal-2"}
	}
	return models
}

func pollIntervalFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv(pollIntervalEnvVar))
	if raw == "" {
		return defaultPollInterval
	}
	valueMS, err := strconv.Atoi(raw)
	if err != nil {
		return defaultPollInterval
	}
	if valueMS < minPollIntervalMS {
		valueMS = minPollIntervalMS
	}
	if valueMS > maxPollIntervalMS {
		valueMS = maxPollIntervalMS
	}
	return time.Duration(valueMS) * time.Millisecond
}
