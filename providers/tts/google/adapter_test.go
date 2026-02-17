package google

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestConfigFromEnvSecretRefs(t *testing.T) {
	t.Setenv("RSPP_TTS_GOOGLE_API_KEY", "literal-key")
	t.Setenv("RSPP_TTS_GOOGLE_API_KEY_REF", "env://RSPP_TEST_TTS_GOOGLE_API_KEY")
	t.Setenv("RSPP_TEST_TTS_GOOGLE_API_KEY", "secret-key")
	t.Setenv("RSPP_TTS_GOOGLE_ENDPOINT", "https://literal.example.com")
	t.Setenv("RSPP_TTS_GOOGLE_ENDPOINT_REF", "env://RSPP_TEST_TTS_GOOGLE_ENDPOINT")
	t.Setenv("RSPP_TEST_TTS_GOOGLE_ENDPOINT", "https://secret.example.com")

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

	audio := []byte("google-tts-audio-bytes")
	encoded := base64.StdEncoding.EncodeToString(audio)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "tts-google-key" {
			t.Fatalf("expected API key query param, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"audioContent":%q}`, encoded)
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{
		APIKey:      "tts-google-key",
		Endpoint:    srv.URL,
		VoiceName:   "en-US-Chirp3-HD-Achernar",
		Language:    "en-US",
		SampleText:  "hello",
		AudioFormat: "MP3",
		Timeout:     2 * time.Second,
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
			if chunk.Metadata["audio_bytes"] != strconv.Itoa(len(audio)) {
				t.Fatalf("unexpected final metadata: %+v", chunk.Metadata)
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

func TestInvokeStreamInvalidAudioBase64(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"audioContent":"%%%"}`))
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{Endpoint: srv.URL, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	streaming := adapter.(contracts.StreamingAdapter)
	errorsObserved := 0
	outcome, err := streaming.InvokeStream(testInvocationRequest(), contracts.StreamObserverFuncs{
		OnErrorFn: func(chunk contracts.StreamChunk) error {
			errorsObserved++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("invoke stream: %v", err)
	}
	if outcome.Class != contracts.OutcomeInfrastructureFailure {
		t.Fatalf("expected infrastructure_failure outcome, got %s", outcome.Class)
	}
	if errorsObserved != 1 {
		t.Fatalf("expected one error callback, got %d", errorsObserved)
	}
}

func testInvocationRequest() contracts.InvocationRequest {
	return contracts.InvocationRequest{
		SessionID:            "sess-tts-google-1",
		TurnID:               "turn-tts-google-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-tts-google-1",
		ProviderInvocationID: "pvi/sess-tts-google-1/turn-tts-google-1/evt-tts-google-1/tts",
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
