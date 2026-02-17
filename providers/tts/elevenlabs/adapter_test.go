package elevenlabs

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestConfigFromEnvSecretRefs(t *testing.T) {
	t.Setenv("RSPP_TTS_ELEVENLABS_API_KEY", "literal-key")
	t.Setenv("RSPP_TTS_ELEVENLABS_API_KEY_REF", "env://RSPP_TEST_TTS_ELEVENLABS_API_KEY")
	t.Setenv("RSPP_TEST_TTS_ELEVENLABS_API_KEY", "secret-key")
	t.Setenv("RSPP_TTS_ELEVENLABS_ENDPOINT", "https://literal.example.com")
	t.Setenv("RSPP_TTS_ELEVENLABS_ENDPOINT_REF", "env://RSPP_TEST_TTS_ELEVENLABS_ENDPOINT")
	t.Setenv("RSPP_TEST_TTS_ELEVENLABS_ENDPOINT", "https://secret.example.com")

	cfg := ConfigFromEnv()
	if cfg.APIKey != "secret-key" {
		t.Fatalf("expected API key resolved from secret ref, got %q", cfg.APIKey)
	}
	if cfg.Endpoint != "https://secret.example.com" {
		t.Fatalf("expected endpoint resolved from secret ref, got %q", cfg.Endpoint)
	}
}

func TestInvokeStreamSuccess(t *testing.T) {
	t.Parallel()

	payload := []byte("audio-stream-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/text-to-speech/test-voice/stream" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("xi-api-key"); got != "tts-key" {
			t.Fatalf("expected xi-api-key header, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{
		APIKey:   "tts-key",
		Endpoint: srv.URL + "/v1/text-to-speech/test-voice",
		VoiceID:  "test-voice",
		ModelID:  "eleven_multilingual_v2",
		Text:     "hello",
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	streaming := adapter.(contracts.StreamingAdapter)

	started := 0
	chunks := 0
	completed := 0
	outcome, err := streaming.InvokeStream(testInvocationRequest(), contracts.StreamObserverFuncs{
		OnStartFn: func(chunk contracts.StreamChunk) error {
			started++
			return nil
		},
		OnChunkFn: func(chunk contracts.StreamChunk) error {
			chunks++
			if len(chunk.AudioBytes) == 0 {
				t.Fatalf("expected non-empty audio bytes")
			}
			return nil
		},
		OnCompleteFn: func(chunk contracts.StreamChunk) error {
			completed++
			if chunk.Metadata["audio_bytes"] != strconv.Itoa(len(payload)) {
				t.Fatalf("unexpected final audio_bytes metadata: %+v", chunk.Metadata)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("invoke stream: %v", err)
	}
	if outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected success outcome, got %s", outcome.Class)
	}
	if started != 1 || completed != 1 || chunks < 1 {
		t.Fatalf("unexpected observer counts: start=%d chunks=%d complete=%d", started, chunks, completed)
	}
}

func TestInvokeStreamCancelled(t *testing.T) {
	t.Parallel()

	adapter, err := NewAdapter(Config{Endpoint: "https://example.com", Timeout: time.Second})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	streaming := adapter.(contracts.StreamingAdapter)
	req := testInvocationRequest()
	req.CancelRequested = true

	errorCount := 0
	outcome, err := streaming.InvokeStream(req, contracts.StreamObserverFuncs{
		OnErrorFn: func(chunk contracts.StreamChunk) error {
			errorCount++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("invoke stream cancelled: %v", err)
	}
	if outcome.Class != contracts.OutcomeCancelled {
		t.Fatalf("expected cancelled outcome, got %s", outcome.Class)
	}
	if errorCount != 1 {
		t.Fatalf("expected one error callback, got %d", errorCount)
	}
}

func testInvocationRequest() contracts.InvocationRequest {
	return contracts.InvocationRequest{
		SessionID:            "sess-elevenlabs-1",
		TurnID:               "turn-elevenlabs-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-elevenlabs-1",
		ProviderInvocationID: "pvi/sess-elevenlabs-1/turn-elevenlabs-1/evt-elevenlabs-1/tts",
		ProviderID:           ProviderID,
		Modality:             contracts.ModalityTTS,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	}
}
