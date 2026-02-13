package httpadapter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestInvokeMapsHTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		expected  contracts.OutcomeClass
		retryable bool
	}{
		{name: "success", status: http.StatusOK, expected: contracts.OutcomeSuccess, retryable: false},
		{name: "timeout", status: http.StatusRequestTimeout, expected: contracts.OutcomeTimeout, retryable: true},
		{name: "overload", status: http.StatusTooManyRequests, expected: contracts.OutcomeOverload, retryable: true},
		{name: "blocked", status: http.StatusUnauthorized, expected: contracts.OutcomeBlocked, retryable: false},
		{name: "infra", status: http.StatusBadGateway, expected: contracts.OutcomeInfrastructureFailure, retryable: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer ts.Close()

			adapter, err := New(Config{
				ProviderID: "provider-a",
				Modality:   contracts.ModalityLLM,
				Endpoint:   ts.URL,
				BuildBody: func(req contracts.InvocationRequest) any {
					return map[string]any{"event_id": req.EventID}
				},
			})
			if err != nil {
				t.Fatalf("unexpected adapter error: %v", err)
			}
			outcome, err := adapter.Invoke(contracts.InvocationRequest{
				SessionID:            "sess-1",
				TurnID:               "turn-1",
				PipelineVersion:      "pipeline-v1",
				EventID:              "evt-1",
				ProviderInvocationID: "pvi-1",
				ProviderID:           "provider-a",
				Modality:             contracts.ModalityLLM,
				Attempt:              1,
				TransportSequence:    1,
				RuntimeSequence:      1,
				AuthorityEpoch:       1,
				RuntimeTimestampMS:   1,
				WallClockTimestampMS: 1,
			})
			if err != nil {
				t.Fatalf("unexpected invoke error: %v", err)
			}
			if outcome.Class != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, outcome.Class)
			}
			if outcome.Retryable != tc.retryable {
				t.Fatalf("expected retryable=%v, got %v", tc.retryable, outcome.Retryable)
			}
		})
	}
}

func TestInvokeCancelledShortCircuit(t *testing.T) {
	t.Parallel()

	adapter, err := New(Config{ProviderID: "provider-a", Modality: contracts.ModalityTTS, Endpoint: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected adapter error: %v", err)
	}

	outcome, err := adapter.Invoke(contracts.InvocationRequest{
		SessionID:            "sess-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           "provider-a",
		Modality:             contracts.ModalityTTS,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
		CancelRequested:      true,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if outcome.Class != contracts.OutcomeCancelled {
		t.Fatalf("expected cancelled outcome, got %s", outcome.Class)
	}
}

func TestInvokeCapturesInputAndOutputPayloads(t *testing.T) {
	t.Setenv("RSPP_PROVIDER_IO_CAPTURE_MODE", "full")
	t.Setenv("RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES", "4096")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer ts.Close()

	adapter, err := New(Config{
		ProviderID: "provider-a",
		Modality:   contracts.ModalityLLM,
		Endpoint:   ts.URL,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{"prompt": "hello"}
		},
	})
	if err != nil {
		t.Fatalf("unexpected adapter error: %v", err)
	}

	outcome, err := adapter.Invoke(contracts.InvocationRequest{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           "provider-a",
		Modality:             contracts.ModalityLLM,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected success outcome, got %s", outcome.Class)
	}
	if !strings.Contains(outcome.InputPayload, `"prompt":"hello"`) {
		t.Fatalf("expected input payload capture, got %q", outcome.InputPayload)
	}
	if !strings.Contains(outcome.OutputPayload, `"result":"ok"`) {
		t.Fatalf("expected output payload capture, got %q", outcome.OutputPayload)
	}
	if outcome.OutputStatusCode != http.StatusOK {
		t.Fatalf("expected output status code %d, got %d", http.StatusOK, outcome.OutputStatusCode)
	}
}

func TestInvokePayloadCaptureTruncates(t *testing.T) {
	t.Setenv("RSPP_PROVIDER_IO_CAPTURE_MODE", "full")
	t.Setenv("RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES", "256")

	largeOutput := strings.Repeat("x", 1024)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeOutput))
	}))
	defer ts.Close()

	adapter, err := New(Config{
		ProviderID: "provider-a",
		Modality:   contracts.ModalityLLM,
		Endpoint:   ts.URL,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{"prompt": strings.Repeat("y", 1024)}
		},
	})
	if err != nil {
		t.Fatalf("unexpected adapter error: %v", err)
	}

	outcome, err := adapter.Invoke(contracts.InvocationRequest{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           "provider-a",
		Modality:             contracts.ModalityLLM,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if !outcome.PayloadTruncated {
		t.Fatalf("expected payload truncation marker, got false")
	}
	if len(outcome.InputPayload) == 0 || len(outcome.OutputPayload) == 0 {
		t.Fatalf("expected non-empty captured payloads")
	}
}

func TestInvokeStreamEmitsLifecycle(t *testing.T) {
	t.Setenv("RSPP_PROVIDER_IO_CAPTURE_MODE", "full")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer ts.Close()

	adapter, err := New(Config{
		ProviderID: "provider-a",
		Modality:   contracts.ModalityLLM,
		Endpoint:   ts.URL,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{"prompt": "hello"}
		},
	})
	if err != nil {
		t.Fatalf("unexpected adapter error: %v", err)
	}

	started := 0
	completed := 0
	outcome, err := adapter.InvokeStream(contracts.InvocationRequest{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           "provider-a",
		Modality:             contracts.ModalityLLM,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	}, contracts.StreamObserverFuncs{
		OnStartFn: func(chunk contracts.StreamChunk) error {
			started++
			return nil
		},
		OnCompleteFn: func(chunk contracts.StreamChunk) error {
			completed++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected stream invoke error: %v", err)
	}
	if outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected stream success outcome, got %s", outcome.Class)
	}
	if started != 1 || completed != 1 {
		t.Fatalf("expected one start and complete chunk, got start=%d complete=%d", started, completed)
	}
}
