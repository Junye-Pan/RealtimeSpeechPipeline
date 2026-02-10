package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	runtimeexecutionpool "github.com/tiger/realtime-speech-pipeline/internal/runtime/executionpool"
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
	if evidence.FinalAttemptLatencyMS != 0 || evidence.TotalInvocationLatencyMS != 0 {
		t.Fatalf("expected default zero latency fields for direct provider evidence projection, got %+v", evidence)
	}
}

func TestExecutePlanDeterministicOrderAndRoutes(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-1",
			TurnID:               "turn-plan-1",
			EventID:              "evt-plan-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    10,
			RuntimeSequence:      11,
			AuthorityEpoch:       3,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "admission", NodeType: "admission", Lane: eventabi.LaneControl},
				{NodeID: "provider", NodeType: "provider", Lane: eventabi.LaneData},
				{NodeID: "telemetry", NodeType: "metrics", Lane: eventabi.LaneTelemetry},
			},
			Edges: []EdgeSpec{
				{From: "admission", To: "provider"},
				{From: "admission", To: "telemetry"},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected completed execution trace")
	}
	if len(trace.Nodes) != 3 {
		t.Fatalf("expected 3 node results, got %d", len(trace.Nodes))
	}
	if trace.Nodes[0].NodeID != "admission" || trace.Nodes[1].NodeID != "provider" || trace.Nodes[2].NodeID != "telemetry" {
		t.Fatalf("unexpected node execution order: %+v", trace.NodeOrder)
	}
	if trace.Nodes[0].DispatchTarget.QueueKey != "runtime/control/admission" {
		t.Fatalf("unexpected dispatch route for admission node: %s", trace.Nodes[0].DispatchTarget.QueueKey)
	}
	if trace.Nodes[1].DispatchTarget.QueueKey != "runtime/data/provider" {
		t.Fatalf("unexpected dispatch route for provider node: %s", trace.Nodes[1].DispatchTarget.QueueKey)
	}
	if trace.Nodes[2].DispatchTarget.QueueKey != "runtime/telemetry/metrics" {
		t.Fatalf("unexpected dispatch route for telemetry node: %s", trace.Nodes[2].DispatchTarget.QueueKey)
	}
	if len(trace.ControlSignals) != 0 {
		t.Fatalf("expected no control signals on allow-only path, got %d", len(trace.ControlSignals))
	}
}

func TestExecutePlanStopsOnDeniedNode(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-2",
			TurnID:               "turn-plan-2",
			EventID:              "evt-plan-2",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    20,
			RuntimeSequence:      21,
			AuthorityEpoch:       4,
			RuntimeTimestampMS:   200,
			WallClockTimestampMS: 200,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "node-a", NodeType: "admission", Lane: eventabi.LaneControl},
				{NodeID: "node-b", NodeType: "admission", Lane: eventabi.LaneControl, Shed: true},
				{NodeID: "node-c", NodeType: "provider", Lane: eventabi.LaneData},
			},
			Edges: []EdgeSpec{
				{From: "node-a", To: "node-b"},
				{From: "node-b", To: "node-c"},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if trace.Completed {
		t.Fatalf("expected trace to stop on denied node")
	}
	if len(trace.Nodes) != 2 {
		t.Fatalf("expected execution to stop at second node, got %d nodes", len(trace.Nodes))
	}
	if trace.Nodes[1].NodeID != "node-b" || trace.Nodes[1].Decision.Allowed {
		t.Fatalf("expected node-b denied decision, got %+v", trace.Nodes[1].Decision)
	}
	if len(trace.ControlSignals) != 1 || trace.ControlSignals[0].Signal != "shed" {
		t.Fatalf("expected one normalized shed signal, got %+v", trace.ControlSignals)
	}
}

func TestExecutePlanProviderAttemptsPersisted(t *testing.T) {
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

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 4, DetailCapacity: 4, AttemptCapacity: 8})
	scheduler := NewSchedulerWithProviderInvokerAndAttemptAppender(
		localadmission.Evaluator{},
		invocation.NewController(catalog),
		&recorder,
	)

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-provider-1",
			TurnID:               "turn-plan-provider-1",
			EventID:              "evt-plan-provider-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    50,
			RuntimeSequence:      51,
			AuthorityEpoch:       7,
			RuntimeTimestampMS:   500,
			WallClockTimestampMS: 500,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:   "provider-node",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
					Provider: &ProviderInvocationInput{
						Modality:               contracts.ModalitySTT,
						PreferredProvider:      "stt-a",
						AllowedAdaptiveActions: []string{"provider_switch"},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected completed execution trace")
	}
	if len(trace.Nodes) != 1 || trace.Nodes[0].Decision.Provider == nil {
		t.Fatalf("expected provider node decision details, got %+v", trace.Nodes)
	}
	attempts := recorder.ProviderAttemptEntries()
	if len(attempts) != 2 {
		t.Fatalf("expected 2 persisted provider attempts, got %d", len(attempts))
	}
	if attempts[0].ProviderID != "stt-a" || attempts[1].ProviderID != "stt-b" {
		t.Fatalf("unexpected persisted attempt providers: %+v", attempts)
	}
	if attempts[0].AttemptLatencyMS != 0 || attempts[1].AttemptLatencyMS != 1 {
		t.Fatalf("unexpected persisted attempt latencies: %+v", attempts)
	}
}

func TestExecutePlanInvocationSnapshotDisabledByDefault(t *testing.T) {
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

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 4, DetailCapacity: 4, AttemptCapacity: 8})
	scheduler := NewSchedulerWithProviderInvokerAndAttemptAppender(
		localadmission.Evaluator{},
		invocation.NewController(catalog),
		&recorder,
	)

	_, err = scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-snapshot-disabled-1",
			TurnID:               "turn-plan-snapshot-disabled-1",
			EventID:              "evt-plan-snapshot-disabled-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    10,
			RuntimeSequence:      11,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:   "provider-node",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
					Provider: &ProviderInvocationInput{
						Modality:          contracts.ModalitySTT,
						PreferredProvider: "stt-a",
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}

	if snapshots := recorder.InvocationSnapshotEntries(); len(snapshots) != 0 {
		t.Fatalf("expected no invocation snapshots when disabled, got %+v", snapshots)
	}
}

func TestExecutePlanInvocationSnapshotEnabled(t *testing.T) {
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

	recorder := timeline.NewRecorder(timeline.StageAConfig{
		BaselineCapacity:         4,
		DetailCapacity:           4,
		AttemptCapacity:          8,
		InvocationSnapshotCap:    4,
		EnableInvocationSnapshot: true,
	})
	scheduler := NewSchedulerWithProviderInvokerAndAttemptAppender(
		localadmission.Evaluator{},
		invocation.NewController(catalog),
		&recorder,
	)

	_, err = scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-snapshot-enabled-1",
			TurnID:               "turn-plan-snapshot-enabled-1",
			EventID:              "evt-plan-snapshot-enabled-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    10,
			RuntimeSequence:      11,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:   "provider-node",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
					Provider: &ProviderInvocationInput{
						Modality:               contracts.ModalitySTT,
						PreferredProvider:      "stt-a",
						AllowedAdaptiveActions: []string{"provider_switch"},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}

	snapshots := recorder.InvocationSnapshotEntries()
	if len(snapshots) != 1 {
		t.Fatalf("expected one invocation snapshot entry, got %+v", snapshots)
	}
	snapshot := snapshots[0]
	if snapshot.ProviderID != "stt-b" || snapshot.OutcomeClass != "success" || snapshot.AttemptCount != 2 {
		t.Fatalf("unexpected invocation snapshot payload: %+v", snapshot)
	}
	if snapshot.FinalAttemptLatencyMS != 1 || snapshot.TotalInvocationLatencyMS != 1 {
		t.Fatalf("unexpected invocation snapshot latencies: %+v", snapshot)
	}
}

func TestExecutePlanCycleValidation(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	_, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-cycle-1",
			TurnID:               "turn-plan-cycle-1",
			EventID:              "evt-plan-cycle-1",
			PipelineVersion:      "pipeline-v1",
			RuntimeTimestampMS:   1,
			WallClockTimestampMS: 1,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "a", NodeType: "provider", Lane: eventabi.LaneData},
				{NodeID: "b", NodeType: "provider", Lane: eventabi.LaneData},
			},
			Edges: []EdgeSpec{
				{From: "a", To: "b"},
				{From: "b", To: "a"},
			},
		},
	)
	if err == nil {
		t.Fatalf("expected cycle validation error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestExecutePlanProviderFailureDegradeContinues(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "llm-a",
			Mode: contracts.ModalityLLM,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:     contracts.OutcomeTimeout,
					Retryable: false,
					Reason:    "provider_timeout",
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invocation.NewController(catalog))
	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-failure-degrade-1",
			TurnID:               "turn-plan-failure-degrade-1",
			EventID:              "evt-plan-failure-degrade-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   10,
			WallClockTimestampMS: 10,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:       "llm-node",
					NodeType:     "provider",
					Lane:         eventabi.LaneData,
					AllowDegrade: true,
					Provider: &ProviderInvocationInput{
						Modality:          contracts.ModalityLLM,
						PreferredProvider: "llm-a",
					},
				},
				{
					NodeID:   "follow-up",
					NodeType: "admission",
					Lane:     eventabi.LaneControl,
				},
			},
			Edges: []EdgeSpec{{From: "llm-node", To: "follow-up"}},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected degraded execution to continue, got %+v", trace)
	}
	if len(trace.Nodes) != 2 {
		t.Fatalf("expected follow-up node execution, got %d nodes", len(trace.Nodes))
	}
	if trace.Nodes[0].Failure == nil || trace.Nodes[0].Failure.Terminal {
		t.Fatalf("expected non-terminal failure shaping on first node, got %+v", trace.Nodes[0].Failure)
	}

	foundDegrade := false
	for _, signal := range trace.ControlSignals {
		if signal.Signal == "degrade" {
			foundDegrade = true
			break
		}
	}
	if !foundDegrade {
		t.Fatalf("expected degrade signal in trace controls, got %+v", trace.ControlSignals)
	}
}

func TestExecutePlanProviderFailureTerminalStopsWithReason(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "tts-a",
			Mode: contracts.ModalityTTS,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:     contracts.OutcomeInfrastructureFailure,
					Retryable: false,
					Reason:    "provider_unreachable",
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invocation.NewController(catalog))
	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-failure-terminal-1",
			TurnID:               "turn-plan-failure-terminal-1",
			EventID:              "evt-plan-failure-terminal-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    5,
			RuntimeSequence:      5,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   50,
			WallClockTimestampMS: 50,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:   "tts-node",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
					Provider: &ProviderInvocationInput{
						Modality:          contracts.ModalityTTS,
						PreferredProvider: "tts-a",
					},
				},
				{
					NodeID:   "should-not-run",
					NodeType: "admission",
					Lane:     eventabi.LaneControl,
				},
			},
			Edges: []EdgeSpec{{From: "tts-node", To: "should-not-run"}},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if trace.Completed {
		t.Fatalf("expected terminal failure to stop execution")
	}
	if trace.TerminalReason != "node_timeout_or_failure" {
		t.Fatalf("expected terminal reason node_timeout_or_failure, got %q", trace.TerminalReason)
	}
	if len(trace.Nodes) != 1 {
		t.Fatalf("expected execution to stop after first node, got %d nodes", len(trace.Nodes))
	}
	if trace.Nodes[0].Failure == nil || !trace.Nodes[0].Failure.Terminal {
		t.Fatalf("expected terminal node failure shape, got %+v", trace.Nodes[0].Failure)
	}
}

func TestExecutePlanWithExecutionPool(t *testing.T) {
	t.Parallel()

	pool := runtimeexecutionpool.NewManager(4)
	scheduler := NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)
	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-pool-1",
			TurnID:               "turn-plan-pool-1",
			EventID:              "evt-plan-pool-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    10,
			RuntimeSequence:      11,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "a", NodeType: "admission", Lane: eventabi.LaneControl},
				{NodeID: "b", NodeType: "admission", Lane: eventabi.LaneControl},
			},
			Edges: []EdgeSpec{{From: "a", To: "b"}},
		},
	)
	if err != nil {
		t.Fatalf("unexpected pooled execute plan error: %v", err)
	}
	if !trace.Completed || len(trace.Nodes) != 2 {
		t.Fatalf("expected pooled execution completion, got %+v", trace)
	}
	if err := pool.Drain(context.Background()); err != nil {
		t.Fatalf("unexpected pool drain error: %v", err)
	}
	stats := pool.Stats()
	if stats.Submitted < 2 || stats.Completed < 2 {
		t.Fatalf("expected pool submissions/completions, got %+v", stats)
	}
}

func TestSchedulerGeneratesEventIDFromIdentity(t *testing.T) {
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
	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invocation.NewController(catalog))
	decision, err := scheduler.NodeDispatch(SchedulingInput{
		SessionID:            "sess-identity-scheduler-1",
		TurnID:               "turn-identity-scheduler-1",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		ProviderInvocation: &ProviderInvocationInput{
			Modality:          contracts.ModalitySTT,
			PreferredProvider: "stt-a",
		},
	})
	if err != nil {
		t.Fatalf("unexpected scheduler invoke error: %v", err)
	}
	if decision.Provider == nil || decision.Provider.ProviderInvocationID == "" {
		t.Fatalf("expected generated provider invocation id, got %+v", decision.Provider)
	}
}
