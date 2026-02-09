package executor

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
)

func TestSchedulerAllowsWhenNoShed(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	in := SchedulingInput{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		EventID:              "evt-allow",
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
		Shed:                 false,
	}

	checks := []struct {
		name string
		fn   func(SchedulingInput) (SchedulingDecision, error)
	}{
		{name: "edge_enqueue", fn: scheduler.EdgeEnqueue},
		{name: "edge_dequeue", fn: scheduler.EdgeDequeue},
		{name: "node_dispatch", fn: scheduler.NodeDispatch},
	}

	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()
			decision, err := check.fn(in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !decision.Allowed {
				t.Fatalf("expected allow decision")
			}
			if decision.Outcome != nil {
				t.Fatalf("expected no decision outcome for allow path")
			}
			if decision.ControlSignal != nil {
				t.Fatalf("expected no control signal for allow path")
			}
		})
	}
}

func TestSchedulerShedBySchedulingPoint(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	in := SchedulingInput{
		SessionID:            "sess-2",
		TurnID:               "turn-2",
		EventID:              "evt-shed",
		RuntimeTimestampMS:   2,
		WallClockTimestampMS: 2,
		Shed:                 true,
	}

	checks := []struct {
		name          string
		expectedScope controlplane.OutcomeScope
		fn            func(SchedulingInput) (SchedulingDecision, error)
	}{
		{name: "edge_enqueue", expectedScope: controlplane.ScopeEdgeEnqueue, fn: scheduler.EdgeEnqueue},
		{name: "edge_dequeue", expectedScope: controlplane.ScopeEdgeDequeue, fn: scheduler.EdgeDequeue},
		{name: "node_dispatch", expectedScope: controlplane.ScopeNodeDispatch, fn: scheduler.NodeDispatch},
	}

	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()
			decision, err := check.fn(in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision.Allowed {
				t.Fatalf("expected shed decision to be not allowed")
			}
			if decision.Outcome == nil {
				t.Fatalf("expected shed outcome")
			}
			if decision.Outcome.OutcomeKind != controlplane.OutcomeShed {
				t.Fatalf("expected shed, got %s", decision.Outcome.OutcomeKind)
			}
			if decision.Outcome.Phase != controlplane.PhaseScheduling {
				t.Fatalf("expected scheduling_point phase, got %s", decision.Outcome.Phase)
			}
			if decision.Outcome.Scope != check.expectedScope {
				t.Fatalf("expected scope %s, got %s", check.expectedScope, decision.Outcome.Scope)
			}
			if decision.ControlSignal == nil {
				t.Fatalf("expected shed control signal")
			}
			if decision.ControlSignal.Signal != "shed" {
				t.Fatalf("expected control signal shed, got %s", decision.ControlSignal.Signal)
			}
			if decision.ControlSignal.EmittedBy != "RK-25" {
				t.Fatalf("expected RK-25 emitter, got %s", decision.ControlSignal.EmittedBy)
			}
		})
	}
}

func TestSchedulerDeterministicShedReason(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	in := SchedulingInput{
		SessionID:            "sess-3",
		TurnID:               "turn-3",
		EventID:              "evt-shed-default-reason",
		RuntimeTimestampMS:   3,
		WallClockTimestampMS: 3,
		Shed:                 true,
	}

	first, err := scheduler.EdgeEnqueue(in)
	if err != nil {
		t.Fatalf("unexpected error on first decision: %v", err)
	}
	second, err := scheduler.EdgeEnqueue(in)
	if err != nil {
		t.Fatalf("unexpected error on second decision: %v", err)
	}

	if first.Outcome == nil || second.Outcome == nil {
		t.Fatalf("expected shed outcomes on both decisions")
	}
	if first.Outcome.Reason != "scheduling_point_shed" || second.Outcome.Reason != "scheduling_point_shed" {
		t.Fatalf("expected deterministic default shed reason, got %q and %q", first.Outcome.Reason, second.Outcome.Reason)
	}
	if first.ControlSignal == nil || second.ControlSignal == nil {
		t.Fatalf("expected shed control signals on both decisions")
	}
	if first.ControlSignal.Reason != "scheduling_point_shed" || second.ControlSignal.Reason != "scheduling_point_shed" {
		t.Fatalf("expected deterministic shed control-signal reason, got %q and %q", first.ControlSignal.Reason, second.ControlSignal.Reason)
	}
}

func TestSchedulerProviderInvocationSuccess(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}
	invoker := invocation.NewController(catalog)
	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invoker)

	decision, err := scheduler.NodeDispatch(SchedulingInput{
		SessionID:            "sess-provider-1",
		TurnID:               "turn-provider-1",
		EventID:              "evt-provider-1",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		ProviderInvocation: &ProviderInvocationInput{
			Modality:          contracts.ModalitySTT,
			PreferredProvider: "stt-a",
		},
	})
	if err != nil {
		t.Fatalf("unexpected provider invocation error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected provider success to remain allowed")
	}
	if decision.Provider == nil {
		t.Fatalf("expected provider decision details")
	}
	if decision.Provider.SelectedProvider != "stt-a" {
		t.Fatalf("expected selected provider stt-a, got %s", decision.Provider.SelectedProvider)
	}
	if decision.Provider.OutcomeClass != contracts.OutcomeSuccess {
		t.Fatalf("expected success outcome class, got %s", decision.Provider.OutcomeClass)
	}
}

func TestSchedulerProviderInvocationSwitchAfterFailure(t *testing.T) {
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
	invoker := invocation.NewController(catalog)
	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invoker)

	decision, err := scheduler.NodeDispatch(SchedulingInput{
		SessionID:            "sess-provider-2",
		TurnID:               "turn-provider-2",
		EventID:              "evt-provider-2",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    20,
		RuntimeSequence:      21,
		AuthorityEpoch:       8,
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		ProviderInvocation: &ProviderInvocationInput{
			Modality:               contracts.ModalitySTT,
			PreferredProvider:      "stt-a",
			AllowedAdaptiveActions: []string{"provider_switch"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected provider switch error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected switched success to be allowed")
	}
	if decision.Provider == nil {
		t.Fatalf("expected provider decision details")
	}
	if decision.Provider.RetryDecision != "provider_switch" {
		t.Fatalf("expected provider_switch decision, got %s", decision.Provider.RetryDecision)
	}
	if decision.Provider.SelectedProvider != "stt-b" {
		t.Fatalf("expected selected provider stt-b, got %s", decision.Provider.SelectedProvider)
	}
	if len(decision.Provider.Signals) != 3 {
		t.Fatalf("expected provider_error/circuit_event/provider_switch, got %d", len(decision.Provider.Signals))
	}

	evidence := decision.Provider.ToInvocationOutcomeEvidence()
	if evidence.OutcomeClass != "success" || evidence.Modality != "stt" {
		t.Fatalf("expected invocation evidence success/stt, got %+v", evidence)
	}
}
