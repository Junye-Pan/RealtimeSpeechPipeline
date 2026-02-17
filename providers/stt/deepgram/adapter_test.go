package deepgram

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestConfigFromEnvSecretRefs(t *testing.T) {
	t.Setenv("RSPP_STT_DEEPGRAM_API_KEY", "literal-key")
	t.Setenv("RSPP_STT_DEEPGRAM_API_KEY_REF", "env://RSPP_TEST_DEEPGRAM_API_KEY")
	t.Setenv("RSPP_TEST_DEEPGRAM_API_KEY", "secret-key")
	t.Setenv("RSPP_STT_DEEPGRAM_ENDPOINT", "https://literal.example.com")
	t.Setenv("RSPP_STT_DEEPGRAM_ENDPOINT_REF", "env://RSPP_TEST_DEEPGRAM_ENDPOINT")
	t.Setenv("RSPP_TEST_DEEPGRAM_ENDPOINT", "https://secret.example.com")

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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Token deepgram-key" {
			t.Fatalf("expected authorization header with token, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"results":{"channels":[{"alternatives":[{"transcript":"hello deepgram"}]}]}}`)
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{
		APIKey:   "deepgram-key",
		Endpoint: srv.URL,
		Model:    "nova-2",
		AudioURL: "https://example.com/audio.wav",
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
			return nil
		},
		OnCompleteFn: func(chunk contracts.StreamChunk) error {
			completed++
			if chunk.TextFinal != "hello deepgram" {
				t.Fatalf("unexpected final transcript %q", chunk.TextFinal)
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
	if started != 1 || completed != 1 || chunks < 2 {
		t.Fatalf("unexpected observer counts: start=%d chunks=%d complete=%d", started, chunks, completed)
	}
}

func TestInvokeStreamHTTPErrorMapsToOverload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limit"}`))
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
	if outcome.Class != contracts.OutcomeOverload {
		t.Fatalf("expected overload outcome, got %s", outcome.Class)
	}
	if errorsObserved != 1 {
		t.Fatalf("expected one error callback, got %d", errorsObserved)
	}
}

func testInvocationRequest() contracts.InvocationRequest {
	return contracts.InvocationRequest{
		SessionID:            "sess-deepgram-1",
		TurnID:               "turn-deepgram-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-deepgram-1",
		ProviderInvocationID: "pvi/sess-deepgram-1/turn-deepgram-1/evt-deepgram-1/stt",
		ProviderID:           ProviderID,
		Modality:             contracts.ModalitySTT,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	}
}
