package invocation

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
)

func TestInvokeRetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	attempts := 0
	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				attempts++
				if attempts == 1 {
					return contracts.Outcome{
						Class:     contracts.OutcomeTimeout,
						Retryable: true,
						Reason:    "provider_timeout",
					}, nil
				}
				return contracts.Outcome{
					Class: contracts.OutcomeSuccess,
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewControllerWithConfig(catalog, Config{
		MaxAttemptsPerProvider: 2,
		MaxCandidateProviders:  5,
	})

	result, err := controller.Invoke(InvocationInput{
		SessionID:              "sess-rk11-1",
		TurnID:                 "turn-rk11-1",
		PipelineVersion:        "pipeline-v1",
		EventID:                "evt-rk11-1",
		Modality:               contracts.ModalitySTT,
		PreferredProvider:      "stt-a",
		AllowedAdaptiveActions: []string{"retry"},
		TransportSequence:      1,
		RuntimeSequence:        1,
		AuthorityEpoch:         1,
		RuntimeTimestampMS:     10,
		WallClockTimestampMS:   10,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result.Outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected success outcome, got %s", result.Outcome.Class)
	}
	if result.RetryDecision != "retry" {
		t.Fatalf("expected retry decision, got %s", result.RetryDecision)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.Attempts))
	}
	if len(result.Signals) != 1 || result.Signals[0].Signal != "provider_error" {
		t.Fatalf("expected one provider_error signal, got %+v", result.Signals)
	}
}

func TestInvokeSwitchesProviderAfterFailure(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:       contracts.OutcomeOverload,
					Retryable:   false,
					CircuitOpen: true,
					Reason:      "provider_overload",
					BackoffMS:   50,
				}, nil
			},
		},
		contracts.StaticAdapter{
			ID:   "stt-b",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewController(catalog)
	result, err := controller.Invoke(InvocationInput{
		SessionID:              "sess-rk11-2",
		TurnID:                 "turn-rk11-2",
		PipelineVersion:        "pipeline-v1",
		EventID:                "evt-rk11-2",
		Modality:               contracts.ModalitySTT,
		PreferredProvider:      "stt-a",
		AllowedAdaptiveActions: []string{"provider_switch"},
		TransportSequence:      2,
		RuntimeSequence:        2,
		AuthorityEpoch:         2,
		RuntimeTimestampMS:     20,
		WallClockTimestampMS:   20,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result.Outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected switched success outcome, got %s", result.Outcome.Class)
	}
	if result.SelectedProvider != "stt-b" {
		t.Fatalf("expected selected provider stt-b, got %s", result.SelectedProvider)
	}
	if result.RetryDecision != "provider_switch" {
		t.Fatalf("expected provider_switch decision, got %s", result.RetryDecision)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.Attempts))
	}
	if len(result.Signals) != 3 {
		t.Fatalf("expected provider_error + circuit_event + provider_switch, got %d signals", len(result.Signals))
	}
	if result.Signals[0].Signal != "provider_error" || result.Signals[1].Signal != "circuit_event" || result.Signals[2].Signal != "provider_switch" {
		t.Fatalf("unexpected signal sequence: %+v", result.Signals)
	}
}

func TestInvokeTerminalFailureWithoutSwitch(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:     contracts.OutcomeBlocked,
					Retryable: false,
					Reason:    "safety_block",
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewController(catalog)
	result, err := controller.Invoke(InvocationInput{
		SessionID:            "sess-rk11-3",
		TurnID:               "turn-rk11-3",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-rk11-3",
		Modality:             contracts.ModalitySTT,
		PreferredProvider:    "stt-a",
		TransportSequence:    3,
		RuntimeSequence:      3,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   30,
		WallClockTimestampMS: 30,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result.Outcome.Class != contracts.OutcomeBlocked {
		t.Fatalf("expected blocked outcome, got %s", result.Outcome.Class)
	}
	if result.RetryDecision != "none" {
		t.Fatalf("expected retry decision none, got %s", result.RetryDecision)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(result.Attempts))
	}
	if len(result.Signals) != 1 || result.Signals[0].Signal != "provider_error" {
		t.Fatalf("expected one provider_error signal, got %+v", result.Signals)
	}
}

func TestInvokeCancelledBeforeAttempt(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewController(catalog)
	result, err := controller.Invoke(InvocationInput{
		SessionID:            "sess-rk11-4",
		TurnID:               "turn-rk11-4",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-rk11-4",
		Modality:             contracts.ModalitySTT,
		PreferredProvider:    "stt-a",
		CancelRequested:      true,
		TransportSequence:    4,
		RuntimeSequence:      4,
		AuthorityEpoch:       4,
		RuntimeTimestampMS:   40,
		WallClockTimestampMS: 40,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result.Outcome.Class != contracts.OutcomeCancelled {
		t.Fatalf("expected cancelled outcome, got %s", result.Outcome.Class)
	}
	if len(result.Attempts) != 0 {
		t.Fatalf("expected no attempts on pre-cancel, got %d", len(result.Attempts))
	}
	if len(result.Signals) != 0 {
		t.Fatalf("expected no signals on pre-cancel, got %+v", result.Signals)
	}
}
