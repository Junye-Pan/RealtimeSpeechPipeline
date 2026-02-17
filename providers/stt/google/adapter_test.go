package google

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestConfigFromEnvSecretRefs(t *testing.T) {
	t.Setenv("RSPP_STT_GOOGLE_API_KEY", "literal-key")
	t.Setenv("RSPP_STT_GOOGLE_API_KEY_REF", "env://RSPP_TEST_STT_GOOGLE_API_KEY")
	t.Setenv("RSPP_TEST_STT_GOOGLE_API_KEY", "secret-key")
	t.Setenv("RSPP_STT_GOOGLE_ENDPOINT", "https://literal.example.com")
	t.Setenv("RSPP_STT_GOOGLE_ENDPOINT_REF", "env://RSPP_TEST_STT_GOOGLE_ENDPOINT")
	t.Setenv("RSPP_TEST_STT_GOOGLE_ENDPOINT", "https://secret.example.com")

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
		if got := r.URL.Query().Get("key"); got != "google-key" {
			t.Fatalf("expected API key query param, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"results":[{"alternatives":[{"transcript":"hello google stt"}]}]}`)
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{
		APIKey:      "google-key",
		Endpoint:    srv.URL,
		Language:    "en-US",
		Model:       "latest_long",
		AudioURI:    "gs://bucket/audio.raw",
		Timeout:     2 * time.Second,
		EnablePunc:  true,
		SampleRate:  16000,
		ChannelMode: 1,
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
			if chunk.TextFinal != "hello google stt" {
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
		t.Fatalf("expected one error callback for cancellation, got %d", errorCount)
	}
}

func testInvocationRequest() contracts.InvocationRequest {
	return contracts.InvocationRequest{
		SessionID:            "sess-stt-google-1",
		TurnID:               "turn-stt-google-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-stt-google-1",
		ProviderInvocationID: "pvi/sess-stt-google-1/turn-stt-google-1/evt-stt-google-1/stt",
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
