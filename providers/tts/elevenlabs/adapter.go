package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	providerconfig "github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/config"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "tts-elevenlabs"

type Config struct {
	APIKey   string
	Endpoint string
	VoiceID  string
	ModelID  string
	Text     string
	Timeout  time.Duration
}

type Adapter struct {
	cfg   Config
	unary *httpadapter.Adapter
	http  *http.Client
}

func ConfigFromEnv() Config {
	voiceID := defaultString(os.Getenv("RSPP_TTS_ELEVENLABS_VOICE_ID"), "EXAVITQu4vr4xnSDxMaL")
	return Config{
		APIKey:   providerconfig.ResolveEnvValue("RSPP_TTS_ELEVENLABS_API_KEY", "RSPP_TTS_ELEVENLABS_API_KEY_REF", ""),
		Endpoint: providerconfig.ResolveEnvValue("RSPP_TTS_ELEVENLABS_ENDPOINT", "RSPP_TTS_ELEVENLABS_ENDPOINT_REF", "https://api.elevenlabs.io/v1/text-to-speech/"+voiceID),
		VoiceID:  voiceID,
		ModelID:  defaultString(os.Getenv("RSPP_TTS_ELEVENLABS_MODEL"), "eleven_multilingual_v2"),
		Text:     defaultString(os.Getenv("RSPP_TTS_ELEVENLABS_TEXT"), "Realtime speech pipeline live smoke test."),
		Timeout:  15 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	unary, err := httpadapter.New(httpadapter.Config{
		ProviderID:    ProviderID,
		Modality:      contracts.ModalityTTS,
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		APIKeyHeader:  "xi-api-key",
		Timeout:       cfg.Timeout,
		StaticHeaders: map[string]string{"Accept": "audio/mpeg"},
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"model_id": cfg.ModelID,
				"text":     cfg.Text,
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
	return contracts.ModalityTTS
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
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 0, nil, outcome.Reason))
		return outcome, nil
	}

	payload := map[string]any{
		"model_id": a.cfg.ModelID,
		"text":     a.cfg.Text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return contracts.Outcome{}, err
	}
	inputPayload, inputTruncated := httpadapter.CapturePayload(body, false)

	streamEndpoint := strings.TrimRight(a.cfg.Endpoint, "/")
	if !strings.HasSuffix(streamEndpoint, "/stream") {
		streamEndpoint += "/stream"
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, streamEndpoint, bytes.NewReader(body))
	if err != nil {
		return contracts.Outcome{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/mpeg")
	if a.cfg.APIKey != "" {
		httpReq.Header.Set("xi-api-key", a.cfg.APIKey)
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

	if err := observer.OnStart(streamChunkFromRequest(req, contracts.StreamChunkStart, 0, nil, "")); err != nil {
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
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, nil, outcome.Reason))
		return outcome, nil
	}

	const chunkSize = 4096
	buf := make([]byte, chunkSize)
	sequence := 1
	totalBytes := 0
	captureLimit := httpadapter.ResolveProviderIOCaptureMaxBytes()
	sample := make([]byte, 0, captureLimit)
	outputTruncated := false
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			part := make([]byte, n)
			copy(part, buf[:n])
			totalBytes += n

			if len(sample) < captureLimit {
				remaining := captureLimit - len(sample)
				if n > remaining {
					sample = append(sample, part[:remaining]...)
					outputTruncated = true
				} else {
					sample = append(sample, part...)
				}
			} else {
				outputTruncated = true
			}

			if err := observer.OnChunk(streamChunkFromRequest(req, contracts.StreamChunkAudio, sequence, part, "")); err != nil {
				return contracts.Outcome{}, err
			}
			sequence++
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			outcome := contracts.Outcome{
				Class:            contracts.OutcomeInfrastructureFailure,
				Retryable:        true,
				Reason:           "provider_audio_stream_read_error",
				InputPayload:     inputPayload,
				OutputStatusCode: resp.StatusCode,
			}
			capturedOutput, capTruncated := httpadapter.CapturePayload(sample, outputTruncated)
			outcome.OutputPayload = capturedOutput
			outcome.PayloadTruncated = inputTruncated || capTruncated
			_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, sequence, nil, outcome.Reason))
			return outcome, nil
		}
	}

	finalChunk := streamChunkFromRequest(req, contracts.StreamChunkFinal, sequence, nil, "")
	finalChunk.Metadata = map[string]string{
		"audio_bytes": strconv.Itoa(totalBytes),
		"mime_type":   "audio/mpeg",
	}
	if err := observer.OnComplete(finalChunk); err != nil {
		return contracts.Outcome{}, err
	}

	outputPayload, capTruncated := httpadapter.CapturePayload(sample, outputTruncated)
	return contracts.Outcome{
		Class:            contracts.OutcomeSuccess,
		InputPayload:     inputPayload,
		OutputPayload:    outputPayload,
		OutputStatusCode: resp.StatusCode,
		PayloadTruncated: inputTruncated || capTruncated,
	}, nil
}

func streamChunkFromRequest(req contracts.InvocationRequest, kind contracts.StreamChunkKind, sequence int, audio []byte, reason string) contracts.StreamChunk {
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
	}
	if kind == contracts.StreamChunkAudio {
		chunk.AudioBytes = audio
		chunk.MimeType = "audio/mpeg"
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
