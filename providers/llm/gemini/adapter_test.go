package gemini

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestConfigFromEnvSecretRefs(t *testing.T) {
	t.Setenv("RSPP_LLM_GEMINI_API_KEY", "literal-key")
	t.Setenv("RSPP_LLM_GEMINI_API_KEY_REF", "env://RSPP_TEST_GEMINI_API_KEY")
	t.Setenv("RSPP_TEST_GEMINI_API_KEY", "secret-key")
	t.Setenv("RSPP_LLM_GEMINI_ENDPOINT", "https://literal.example.com")
	t.Setenv("RSPP_LLM_GEMINI_ENDPOINT_REF", "env://RSPP_TEST_GEMINI_ENDPOINT")
	t.Setenv("RSPP_TEST_GEMINI_ENDPOINT", "https://secret.example.com")

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
		if r.URL.Path != "/v1beta/models/gemini-1.5-flash:streamGenerateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "sse" {
			t.Fatalf("expected alt=sse query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello \"}]}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"world\"}]}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{
		APIKey:   "key",
		Endpoint: srv.URL + "/v1beta/models/gemini-1.5-flash:generateContent",
		Prompt:   "hello",
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	streaming, ok := adapter.(contracts.StreamingAdapter)
	if !ok {
		t.Fatalf("expected streaming adapter")
	}

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
			if chunk.TextFinal != "hello world" {
				t.Fatalf("expected final text hello world, got %q", chunk.TextFinal)
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
	if started != 1 || completed != 1 || chunks != 2 {
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
		t.Fatalf("expected one error callback for cancelled invoke, got %d", errorCount)
	}
}

func testInvocationRequest() contracts.InvocationRequest {
	return contracts.InvocationRequest{
		SessionID:            "sess-gemini-1",
		TurnID:               "turn-gemini-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-gemini-1",
		ProviderInvocationID: "pvi/sess-gemini-1/turn-gemini-1/evt-gemini-1/llm",
		ProviderID:           ProviderID,
		Modality:             contracts.ModalityLLM,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	}
}
