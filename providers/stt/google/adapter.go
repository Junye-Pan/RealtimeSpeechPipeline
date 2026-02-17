package google

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
)

const ProviderID = "stt-google"

type Config struct {
	APIKey      string
	Endpoint    string
	Language    string
	Model       string
	AudioURI    string
	Timeout     time.Duration
	EnablePunc  bool
	SampleRate  int
	ChannelMode int
}

type Adapter struct {
	cfg   Config
	unary *httpadapter.Adapter
	http  *http.Client
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:      providerconfig.ResolveEnvValue("RSPP_STT_GOOGLE_API_KEY", "RSPP_STT_GOOGLE_API_KEY_REF", ""),
		Endpoint:    providerconfig.ResolveEnvValue("RSPP_STT_GOOGLE_ENDPOINT", "RSPP_STT_GOOGLE_ENDPOINT_REF", "https://speech.googleapis.com/v1/speech:recognize"),
		Language:    defaultString(os.Getenv("RSPP_STT_GOOGLE_LANGUAGE"), "en-US"),
		Model:       defaultString(os.Getenv("RSPP_STT_GOOGLE_MODEL"), "latest_long"),
		AudioURI:    defaultString(os.Getenv("RSPP_STT_GOOGLE_AUDIO_URI"), "gs://cloud-samples-data/speech/brooklyn_bridge.raw"),
		Timeout:     10 * time.Second,
		EnablePunc:  true,
		SampleRate:  16000,
		ChannelMode: 1,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	unary, err := httpadapter.New(httpadapter.Config{
		ProviderID:       ProviderID,
		Modality:         contracts.ModalitySTT,
		Endpoint:         cfg.Endpoint,
		APIKey:           cfg.APIKey,
		QueryAPIKeyParam: "key",
		Timeout:          cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"config": map[string]any{
					"languageCode":               cfg.Language,
					"model":                      cfg.Model,
					"enableAutomaticPunctuation": cfg.EnablePunc,
					"audioChannelCount":          cfg.ChannelMode,
					"sampleRateHertz":            cfg.SampleRate,
				},
				"audio": map[string]any{
					"uri": cfg.AudioURI,
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

	bodyObj := map[string]any{
		"config": map[string]any{
			"languageCode":               a.cfg.Language,
			"model":                      a.cfg.Model,
			"enableAutomaticPunctuation": a.cfg.EnablePunc,
			"audioChannelCount":          a.cfg.ChannelMode,
			"sampleRateHertz":            a.cfg.SampleRate,
		},
		"audio": map[string]any{
			"uri": a.cfg.AudioURI,
		},
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

	if err := observer.OnStart(streamChunkFromRequest(req, contracts.StreamChunkStart, 0, "", "")); err != nil {
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
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
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
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
		return outcome, nil
	}

	transcript, err := googleTranscript(responseBytes)
	if err != nil {
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
		_ = observer.OnError(streamChunkFromRequest(req, contracts.StreamChunkError, 1, "", outcome.Reason))
		return outcome, nil
	}

	sequence := 1
	words := strings.Fields(transcript)
	for _, word := range words {
		delta := word + " "
		if err := observer.OnChunk(streamChunkFromRequest(req, contracts.StreamChunkDelta, sequence, delta, "")); err != nil {
			return contracts.Outcome{}, err
		}
		sequence++
	}
	final := streamChunkFromRequest(req, contracts.StreamChunkFinal, sequence, "", "")
	final.TextFinal = strings.TrimSpace(transcript)
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

func googleTranscript(raw []byte) (string, error) {
	var payload struct {
		Results []struct {
			Alternatives []struct {
				Transcript string `json:"transcript"`
			} `json:"alternatives"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if len(payload.Results) == 0 || len(payload.Results[0].Alternatives) == 0 {
		return "", nil
	}
	return payload.Results[0].Alternatives[0].Transcript, nil
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
