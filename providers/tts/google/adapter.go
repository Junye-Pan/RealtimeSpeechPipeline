package google

import (
	"bytes"
	"context"
	"encoding/base64"
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

const ProviderID = "tts-google"

type Config struct {
	APIKey      string
	Endpoint    string
	VoiceName   string
	Language    string
	SampleText  string
	AudioFormat string
	Timeout     time.Duration
}

type Adapter struct {
	cfg   Config
	unary *httpadapter.Adapter
	http  *http.Client
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:      providerconfig.ResolveEnvValue("RSPP_TTS_GOOGLE_API_KEY", "RSPP_TTS_GOOGLE_API_KEY_REF", ""),
		Endpoint:    providerconfig.ResolveEnvValue("RSPP_TTS_GOOGLE_ENDPOINT", "RSPP_TTS_GOOGLE_ENDPOINT_REF", "https://texttospeech.googleapis.com/v1/text:synthesize"),
		VoiceName:   defaultString(os.Getenv("RSPP_TTS_GOOGLE_VOICE"), "en-US-Chirp3-HD-Achernar"),
		Language:    defaultString(os.Getenv("RSPP_TTS_GOOGLE_LANGUAGE"), "en-US"),
		SampleText:  defaultString(os.Getenv("RSPP_TTS_GOOGLE_TEXT"), "Realtime speech pipeline live smoke test."),
		AudioFormat: defaultString(os.Getenv("RSPP_TTS_GOOGLE_AUDIO_ENCODING"), "MP3"),
		Timeout:     15 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	unary, err := httpadapter.New(httpadapter.Config{
		ProviderID:       ProviderID,
		Modality:         contracts.ModalityTTS,
		Endpoint:         cfg.Endpoint,
		APIKey:           cfg.APIKey,
		QueryAPIKeyParam: "key",
		Timeout:          cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"input":       map[string]any{"text": cfg.SampleText},
				"voice":       map[string]any{"name": cfg.VoiceName, "languageCode": cfg.Language},
				"audioConfig": map[string]any{"audioEncoding": cfg.AudioFormat},
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

	bodyObj := map[string]any{
		"input":       map[string]any{"text": a.cfg.SampleText},
		"voice":       map[string]any{"name": a.cfg.VoiceName, "languageCode": a.cfg.Language},
		"audioConfig": map[string]any{"audioEncoding": a.cfg.AudioFormat},
	}
	body, err := json.Marshal(bodyObj)
	if err != nil {
		return contracts.Outcome{}, err
	}
	inputPayload, inputTruncated := httpadapter.CapturePayload(body, false)

	endpoint := a.cfg.Endpoint
	if a.cfg.APIKey != "" {
		endpoint, err = httpadapter.WithQuery(endpoint, "key", a.cfg.APIKey)
		if err != nil {
			return contracts.Outcome{}, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return contracts.Outcome{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

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

	responseBytes, responseTruncated, readErr := httpadapter.ReadBodySample(resp.Body, httpadapter.ResolveProviderIOCaptureMaxBytes()*8)
	if readErr != nil {
		outcome := contracts.Outcome{
			Class:            contracts.OutcomeInfrastructureFailure,
			Retryable:        true,
			Reason:           "provider_response_read_error",
			InputPayload:     inputPayload,
			OutputStatusCode: resp.StatusCode,
		}
		outputPayload, outputTruncated := httpadapter.CapturePayload([]byte(fmt.Sprintf("response_read_error=%v", readErr)), false)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, nil, outcome.Reason))
		return outcome, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		outcome := httpadapter.NormalizeStatus(resp.StatusCode, resp.Header.Get("Retry-After"))
		outcome.InputPayload = inputPayload
		outputPayload, outputTruncated := httpadapter.CapturePayload(responseBytes, responseTruncated)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		if outcome.Reason == "" {
			outcome.Reason = "provider_stream_http_error"
		}
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, nil, outcome.Reason))
		return outcome, nil
	}

	var parsed struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.Unmarshal(responseBytes, &parsed); err != nil {
		outcome := contracts.Outcome{
			Class:            contracts.OutcomeInfrastructureFailure,
			Retryable:        true,
			Reason:           "provider_response_parse_error",
			InputPayload:     inputPayload,
			OutputStatusCode: resp.StatusCode,
		}
		outputPayload, outputTruncated := httpadapter.CapturePayload(responseBytes, responseTruncated)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, nil, outcome.Reason))
		return outcome, nil
	}
	audioBytes, err := base64.StdEncoding.DecodeString(parsed.AudioContent)
	if err != nil {
		outcome := contracts.Outcome{
			Class:            contracts.OutcomeInfrastructureFailure,
			Retryable:        true,
			Reason:           "provider_audio_decode_error",
			InputPayload:     inputPayload,
			OutputStatusCode: resp.StatusCode,
		}
		outputPayload, outputTruncated := httpadapter.CapturePayload(responseBytes, responseTruncated)
		outcome.OutputPayload = outputPayload
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, nil, outcome.Reason))
		return outcome, nil
	}

	sequence := 1
	const chunkSize = 4096
	for offset := 0; offset < len(audioBytes); offset += chunkSize {
		end := offset + chunkSize
		if end > len(audioBytes) {
			end = len(audioBytes)
		}
		part := make([]byte, end-offset)
		copy(part, audioBytes[offset:end])
		if err := observer.OnChunk(streamChunkFromRequest(req, contracts.StreamChunkAudio, sequence, part, "")); err != nil {
			return contracts.Outcome{}, err
		}
		sequence++
	}

	final := streamChunkFromRequest(req, contracts.StreamChunkFinal, sequence, nil, "")
	final.Metadata = map[string]string{
		"audio_bytes": strconv.Itoa(len(audioBytes)),
		"mime_type":   mimeTypeForEncoding(a.cfg.AudioFormat),
	}
	if err := observer.OnComplete(final); err != nil {
		return contracts.Outcome{}, err
	}

	outputPayload, outputTruncated := httpadapter.CapturePayload(responseBytes, responseTruncated)
	return contracts.Outcome{
		Class:            contracts.OutcomeSuccess,
		InputPayload:     inputPayload,
		OutputPayload:    outputPayload,
		OutputStatusCode: resp.StatusCode,
		PayloadTruncated: inputTruncated || outputTruncated,
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

func mimeTypeForEncoding(encoding string) string {
	switch strings.ToUpper(encoding) {
	case "LINEAR16":
		return "audio/wav"
	case "OGG_OPUS":
		return "audio/ogg"
	default:
		return "audio/mpeg"
	}
}

func defaultString(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
