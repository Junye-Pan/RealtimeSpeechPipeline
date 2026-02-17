package invocation

import (
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	providerpolicy "github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/policy"
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

func TestInvokeRejectsUnsupportedAdaptiveAction(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewController(catalog)
	_, err = controller.Invoke(InvocationInput{
		SessionID:              "sess-rk11-5",
		TurnID:                 "turn-rk11-5",
		PipelineVersion:        "pipeline-v1",
		EventID:                "evt-rk11-5",
		Modality:               contracts.ModalitySTT,
		PreferredProvider:      "stt-a",
		AllowedAdaptiveActions: []string{"unsupported_action"},
		TransportSequence:      5,
		RuntimeSequence:        5,
		AuthorityEpoch:         5,
		RuntimeTimestampMS:     50,
		WallClockTimestampMS:   50,
	})
	if err == nil {
		t.Fatalf("expected unsupported adaptive action to fail")
	}
	if !strings.Contains(err.Error(), "unsupported adaptive action") {
		t.Fatalf("expected adaptive-action validation error, got %v", err)
	}
}

func TestInvokePolicyEnvelopePassedToAdapter(t *testing.T) {
	t.Parallel()

	received := contracts.InvocationRequest{}
	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				received = req
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewController(catalog)
	_, err = controller.Invoke(InvocationInput{
		SessionID:              "sess-rk11-6",
		TurnID:                 "turn-rk11-6",
		PipelineVersion:        "pipeline-v1",
		EventID:                "evt-rk11-6",
		Modality:               contracts.ModalitySTT,
		PreferredProvider:      "stt-a",
		AllowedAdaptiveActions: []string{"provider_switch", "retry"},
		TransportSequence:      6,
		RuntimeSequence:        6,
		AuthorityEpoch:         6,
		RuntimeTimestampMS:     60,
		WallClockTimestampMS:   60,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if len(received.AllowedAdaptiveActions) != 2 {
		t.Fatalf("expected 2 normalized actions on request, got %+v", received.AllowedAdaptiveActions)
	}
	if received.AllowedAdaptiveActions[0] != "provider_switch" || received.AllowedAdaptiveActions[1] != "retry" {
		t.Fatalf("unexpected normalized action ordering: %+v", received.AllowedAdaptiveActions)
	}
	if received.RetryBudgetRemaining != 1 {
		t.Fatalf("expected retry budget remaining 1 on first attempt, got %d", received.RetryBudgetRemaining)
	}
	if received.CandidateProviderCount != 1 {
		t.Fatalf("expected candidate provider count 1, got %d", received.CandidateProviderCount)
	}
}

func TestInvokeEmitsTelemetryEvents(t *testing.T) {
	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-telemetry",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	sink := telemetry.NewMemorySink()
	pipeline := telemetry.NewPipeline(sink, telemetry.Config{QueueCapacity: 16})
	previous := telemetry.DefaultEmitter()
	telemetry.SetDefaultEmitter(pipeline)
	t.Cleanup(func() {
		telemetry.SetDefaultEmitter(previous)
		_ = pipeline.Close()
	})

	controller := NewController(catalog)
	_, err = controller.Invoke(InvocationInput{
		SessionID:            "sess-rk11-telemetry-1",
		TurnID:               "turn-rk11-telemetry-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-rk11-telemetry-1",
		Modality:             contracts.ModalitySTT,
		PreferredProvider:    "stt-telemetry",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
		NodeID:               "node-rk11-1",
		EdgeID:               "edge-rk11-1",
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if err := pipeline.Close(); err != nil {
		t.Fatalf("unexpected pipeline close error: %v", err)
	}

	var metricFound bool
	var spanFound bool
	var logFound bool
	for _, event := range sink.Events() {
		if event.Correlation.SessionID != "sess-rk11-telemetry-1" {
			continue
		}
		if event.Correlation.NodeID != "node-rk11-1" || event.Correlation.EdgeID != "edge-rk11-1" {
			t.Fatalf("expected node/edge correlation IDs, got %+v", event.Correlation)
		}
		if event.Kind == telemetry.EventKindMetric && event.Metric != nil && event.Metric.Name == telemetry.MetricProviderRTTMS {
			if event.Metric.Attributes["node_id"] != "node-rk11-1" || event.Metric.Attributes["edge_id"] != "edge-rk11-1" {
				t.Fatalf("expected node/edge metric linkage labels, got %+v", event.Metric.Attributes)
			}
			metricFound = true
		}
		if event.Kind == telemetry.EventKindSpan && event.Span != nil && event.Span.Name == "provider_invocation_span" {
			if event.Span.Attributes["node_id"] != "node-rk11-1" || event.Span.Attributes["edge_id"] != "edge-rk11-1" {
				t.Fatalf("expected node/edge span linkage labels, got %+v", event.Span.Attributes)
			}
			spanFound = true
		}
		if event.Kind == telemetry.EventKindLog && event.Log != nil && event.Log.Name == "provider_invocation_attempt" {
			if event.Log.Attributes["node_id"] != "node-rk11-1" || event.Log.Attributes["edge_id"] != "edge-rk11-1" {
				t.Fatalf("expected node/edge log linkage labels, got %+v", event.Log.Attributes)
			}
			logFound = true
		}
	}
	if !metricFound || !spanFound || !logFound {
		t.Fatalf("expected provider invocation telemetry events, got metric=%v span=%v log=%v", metricFound, spanFound, logFound)
	}
}

func TestInvokeUsesResolvedProviderPlanOrderingAndRefs(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:     contracts.OutcomeTimeout,
					Retryable: true,
					Reason:    "provider_timeout",
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

	controller := NewControllerWithConfig(catalog, Config{MaxAttemptsPerProvider: 3})
	result, err := controller.Invoke(InvocationInput{
		SessionID:         "sess-rk11-plan-1",
		TurnID:            "turn-rk11-plan-1",
		PipelineVersion:   "pipeline-v1",
		EventID:           "evt-rk11-plan-1",
		Modality:          contracts.ModalitySTT,
		PreferredProvider: "stt-a",
		ResolvedProviderPlan: &providerpolicy.ResolvedProviderPlan{
			OrderedCandidates:      []string{"stt-b", "stt-a"},
			MaxAttemptsPerProvider: 2,
			AllowedActions:         []string{"retry"},
			PolicySnapshotRef:      "policy-resolution/v2",
			CapabilitySnapshotRef:  "provider-health/v2",
			RoutingReason:          "rule:tenant",
			SignalSource:           "capability_snapshot",
		},
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result.SelectedProvider != "stt-b" {
		t.Fatalf("expected resolved plan ordering to choose stt-b first, got %s", result.SelectedProvider)
	}
	if result.PolicySnapshotRef != "policy-resolution/v2" || result.CapabilitySnapshotRef != "provider-health/v2" {
		t.Fatalf("expected snapshot refs on invocation result, got policy=%q capability=%q", result.PolicySnapshotRef, result.CapabilitySnapshotRef)
	}
	if result.RoutingReason != "rule:tenant" || result.SignalSource != "capability_snapshot" {
		t.Fatalf("expected routing metadata from resolved plan, got reason=%q source=%q", result.RoutingReason, result.SignalSource)
	}
}

func TestInvokeRespectsResolvedPlanAttemptBudget(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-budget",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:     contracts.OutcomeTimeout,
					Retryable: true,
					Reason:    "provider_timeout",
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewControllerWithConfig(catalog, Config{MaxAttemptsPerProvider: 3})
	result, err := controller.Invoke(InvocationInput{
		SessionID:         "sess-rk11-budget-1",
		TurnID:            "turn-rk11-budget-1",
		PipelineVersion:   "pipeline-v1",
		EventID:           "evt-rk11-budget-1",
		Modality:          contracts.ModalitySTT,
		PreferredProvider: "stt-budget",
		ResolvedProviderPlan: &providerpolicy.ResolvedProviderPlan{
			OrderedCandidates:      []string{"stt-budget"},
			MaxAttemptsPerProvider: 3,
			AllowedActions:         []string{"retry"},
			Budget: providerpolicy.Budget{
				MaxTotalAttempts: 1,
			},
		},
		AllowedAdaptiveActions: []string{"unsupported_action_that_should_be_ignored"},
		TransportSequence:      1,
		RuntimeSequence:        1,
		AuthorityEpoch:         1,
		RuntimeTimestampMS:     20,
		WallClockTimestampMS:   20,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("expected exactly one attempt due to max_total_attempts budget, got %d", len(result.Attempts))
	}
	if result.Outcome.Class != contracts.OutcomeTimeout {
		t.Fatalf("expected timeout outcome after single budgeted attempt, got %s", result.Outcome.Class)
	}
}

func TestInvokeUsesStreamingAdapterWhenEnabled(t *testing.T) {
	streamInvocations := 0
	unaryInvocations := 0
	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "llm-stream",
			Mode: contracts.ModalityLLM,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				unaryInvocations++
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
			InvokeStreamFn: func(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
				streamInvocations++
				start := contracts.StreamChunk{
					SessionID:            req.SessionID,
					TurnID:               req.TurnID,
					PipelineVersion:      req.PipelineVersion,
					EventID:              req.EventID,
					ProviderInvocationID: req.ProviderInvocationID,
					ProviderID:           req.ProviderID,
					Modality:             req.Modality,
					Attempt:              req.Attempt,
					Sequence:             0,
					RuntimeTimestampMS:   req.RuntimeTimestampMS,
					WallClockTimestampMS: req.WallClockTimestampMS,
					Kind:                 contracts.StreamChunkStart,
				}
				if err := observer.OnStart(start); err != nil {
					return contracts.Outcome{}, err
				}
				chunk := start
				chunk.Sequence = 1
				chunk.Kind = contracts.StreamChunkDelta
				chunk.TextDelta = "ok"
				if err := observer.OnChunk(chunk); err != nil {
					return contracts.Outcome{}, err
				}
				final := chunk
				final.Sequence = 2
				final.Kind = contracts.StreamChunkFinal
				final.TextFinal = "ok"
				if err := observer.OnComplete(final); err != nil {
					return contracts.Outcome{}, err
				}
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewController(catalog)
	result, err := controller.Invoke(InvocationInput{
		SessionID:            "sess-stream-1",
		TurnID:               "turn-stream-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-stream-1",
		Modality:             contracts.ModalityLLM,
		PreferredProvider:    "llm-stream",
		EnableStreaming:      true,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result.Outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected success outcome, got %s", result.Outcome.Class)
	}
	if streamInvocations != 1 || unaryInvocations != 0 {
		t.Fatalf("expected streaming path only, got stream=%d unary=%d", streamInvocations, unaryInvocations)
	}
	if !result.StreamingUsed || len(result.Attempts) != 1 || !result.Attempts[0].StreamingUsed {
		t.Fatalf("expected streaming attempt evidence, got %+v", result)
	}
	if result.Attempts[0].ChunkCount < 1 {
		t.Fatalf("expected streamed chunk count >=1, got %d", result.Attempts[0].ChunkCount)
	}
}

func TestInvokeDisablesStreamingWhenExplicitlyRequested(t *testing.T) {
	streamInvocations := 0
	unaryInvocations := 0
	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "llm-stream",
			Mode: contracts.ModalityLLM,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				unaryInvocations++
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
			InvokeStreamFn: func(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
				streamInvocations++
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	controller := NewController(catalog)
	result, err := controller.Invoke(InvocationInput{
		SessionID:                "sess-stream-disable-1",
		TurnID:                   "turn-stream-disable-1",
		PipelineVersion:          "pipeline-v1",
		EventID:                  "evt-stream-disable-1",
		Modality:                 contracts.ModalityLLM,
		PreferredProvider:        "llm-stream",
		EnableStreaming:          true,
		DisableProviderStreaming: true,
		TransportSequence:        1,
		RuntimeSequence:          1,
		AuthorityEpoch:           1,
		RuntimeTimestampMS:       1,
		WallClockTimestampMS:     1,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if result.Outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected success outcome, got %s", result.Outcome.Class)
	}
	if unaryInvocations != 1 || streamInvocations != 0 {
		t.Fatalf("expected unary path only when disable flag is set, got unary=%d stream=%d", unaryInvocations, streamInvocations)
	}
	if result.StreamingUsed {
		t.Fatalf("expected streaming_used=false when disable flag is set, got %+v", result)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].StreamingUsed {
		t.Fatalf("expected attempt evidence to show non-streaming invocation, got %+v", result.Attempts)
	}
}
