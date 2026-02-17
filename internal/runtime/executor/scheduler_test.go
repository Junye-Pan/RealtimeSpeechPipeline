package executor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	runtimeexecutionpool "github.com/tiger/realtime-speech-pipeline/internal/runtime/executionpool"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/planresolver"
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

func TestSchedulerResolvedTurnPlanHydratesProviderInvocationDefaults(t *testing.T) {
	t.Parallel()

	var capturedReq contracts.InvocationRequest
	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-plan-bound",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				capturedReq = req
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}
	invoker := invocation.NewController(catalog)
	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invoker)

	plan := testResolvedTurnPlan(t, "turn-plan-hydrate", "pipeline-plan-v1", 17, []string{"retry", "fallback"})
	plan.ProviderBindings["stt"] = "stt-plan-bound"

	decision, err := scheduler.NodeDispatch(SchedulingInput{
		SessionID:            "sess-plan-hydrate-1",
		TurnID:               "turn-plan-hydrate",
		EventID:              "evt-plan-hydrate-1",
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		ProviderInvocation: &ProviderInvocationInput{
			Modality: contracts.ModalitySTT,
		},
		ResolvedTurnPlan: &plan,
	})
	if err != nil {
		t.Fatalf("unexpected node dispatch error: %v", err)
	}
	if !decision.Allowed || decision.Provider == nil {
		t.Fatalf("expected allowed provider decision, got %+v", decision)
	}
	if decision.Provider.SelectedProvider != "stt-plan-bound" {
		t.Fatalf("expected plan-bound provider selection, got %+v", decision.Provider)
	}
	if capturedReq.ProviderID != "stt-plan-bound" {
		t.Fatalf("expected invocation request provider_id from plan binding, got %+v", capturedReq)
	}
	if capturedReq.PipelineVersion != "pipeline-plan-v1" || capturedReq.AuthorityEpoch != 17 {
		t.Fatalf("expected pipeline/epoch from resolved plan, got %+v", capturedReq)
	}
	if len(capturedReq.AllowedAdaptiveActions) != 2 ||
		!containsString(capturedReq.AllowedAdaptiveActions, "retry") ||
		!containsString(capturedReq.AllowedAdaptiveActions, "fallback") {
		t.Fatalf("expected plan adaptive actions in invocation request, got %+v", capturedReq.AllowedAdaptiveActions)
	}
}

func TestSchedulerResolvedTurnPlanMismatchRejected(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	plan := testResolvedTurnPlan(t, "turn-plan-mismatch", "pipeline-plan-v1", 21, []string{"retry"})

	tests := []struct {
		name  string
		input SchedulingInput
	}{
		{
			name: "pipeline mismatch",
			input: SchedulingInput{
				SessionID:            "sess-plan-mismatch-1",
				TurnID:               "turn-plan-mismatch",
				EventID:              "evt-plan-mismatch-pipeline",
				PipelineVersion:      "pipeline-other",
				RuntimeTimestampMS:   10,
				WallClockTimestampMS: 10,
				ResolvedTurnPlan:     &plan,
			},
		},
		{
			name: "authority epoch mismatch",
			input: SchedulingInput{
				SessionID:            "sess-plan-mismatch-2",
				TurnID:               "turn-plan-mismatch",
				EventID:              "evt-plan-mismatch-epoch",
				PipelineVersion:      "pipeline-plan-v1",
				AuthorityEpoch:       22,
				RuntimeTimestampMS:   11,
				WallClockTimestampMS: 11,
				ResolvedTurnPlan:     &plan,
			},
		},
		{
			name: "turn mismatch",
			input: SchedulingInput{
				SessionID:            "sess-plan-mismatch-3",
				TurnID:               "turn-other",
				EventID:              "evt-plan-mismatch-turn",
				PipelineVersion:      "pipeline-plan-v1",
				AuthorityEpoch:       21,
				RuntimeTimestampMS:   12,
				WallClockTimestampMS: 12,
				ResolvedTurnPlan:     &plan,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := scheduler.EdgeEnqueue(tt.input); err == nil {
				t.Fatalf("expected resolved plan mismatch to fail")
			}
		})
	}
}

func TestSchedulerProviderInvocationDisableStreaming(t *testing.T) {
	t.Parallel()

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

	invoker := invocation.NewController(catalog)
	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invoker)

	decision, err := scheduler.NodeDispatch(SchedulingInput{
		SessionID:            "sess-provider-disable-streaming",
		TurnID:               "turn-provider-disable-streaming",
		EventID:              "evt-provider-disable-streaming",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
		ProviderInvocation: &ProviderInvocationInput{
			Modality:                 contracts.ModalityLLM,
			PreferredProvider:        "llm-stream",
			EnableStreaming:          true,
			DisableProviderStreaming: true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected provider invocation error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected provider invocation to succeed")
	}
	if decision.Provider == nil {
		t.Fatalf("expected provider decision details")
	}
	if decision.Provider.StreamingUsed {
		t.Fatalf("expected scheduler to report streaming_used=false when disable flag is set")
	}
	if unaryInvocations != 1 || streamInvocations != 0 {
		t.Fatalf("expected unary path only, got unary=%d stream=%d", unaryInvocations, streamInvocations)
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

func TestExecutePlanPrioritizesDataBeforeTelemetryWhenBothReady(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-lane-priority-1",
			TurnID:               "turn-plan-lane-priority-1",
			EventID:              "evt-plan-lane-priority-1",
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
				{NodeID: "telemetry", NodeType: "metrics", Lane: eventabi.LaneTelemetry},
				{NodeID: "provider", NodeType: "provider", Lane: eventabi.LaneData},
			},
			Edges: []EdgeSpec{
				{From: "admission", To: "telemetry"},
				{From: "admission", To: "provider"},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if len(trace.Nodes) != 3 {
		t.Fatalf("expected 3 node results, got %d", len(trace.Nodes))
	}
	if trace.Nodes[0].NodeID != "admission" || trace.Nodes[1].NodeID != "provider" || trace.Nodes[2].NodeID != "telemetry" {
		t.Fatalf("expected lane-priority order admission->provider->telemetry, got %+v", trace.NodeOrder)
	}
}

func TestExecutePlanWithResolvedTurnPlanTelemetryOverflowRemainsNonBlocking(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	resolvedPlan := testResolvedTurnPlan(t, "turn-plan-telemetry-overflow", "pipeline-v1", 3, []string{"retry"})
	resolvedPlan = withTightPlanQueueLimits(t, resolvedPlan, 1)

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-telemetry-overflow",
			TurnID:               "turn-plan-telemetry-overflow",
			EventID:              "evt-plan-telemetry-overflow",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    10,
			RuntimeSequence:      11,
			AuthorityEpoch:       3,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
			ResolvedTurnPlan:     &resolvedPlan,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "telemetry-a", NodeType: "metrics", Lane: eventabi.LaneTelemetry},
				{NodeID: "telemetry-b", NodeType: "metrics", Lane: eventabi.LaneTelemetry},
				{NodeID: "control", NodeType: "admission", Lane: eventabi.LaneControl},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected telemetry overflow to remain non-blocking, got %+v", trace)
	}
	if trace.TerminalReason != "" {
		t.Fatalf("expected no terminal reason, got %q", trace.TerminalReason)
	}

	controlExecuted := false
	telemetryDropped := false
	for _, node := range trace.Nodes {
		if node.NodeID == "control" && node.Decision.Allowed {
			controlExecuted = true
		}
		if node.NodeID == "telemetry-b" && node.Decision.Allowed {
			telemetryDropped = true
		}
	}
	if !controlExecuted {
		t.Fatalf("expected control node execution to proceed under telemetry overflow, got %+v", trace.Nodes)
	}
	if !telemetryDropped {
		t.Fatalf("expected dropped telemetry node to be recorded as non-blocking, got %+v", trace.Nodes)
	}

	foundDropNotice := false
	for _, signal := range trace.ControlSignals {
		if signal.Signal == "drop_notice" {
			foundDropNotice = true
			break
		}
	}
	if !foundDropNotice {
		t.Fatalf("expected telemetry drop_notice signal, got %+v", trace.ControlSignals)
	}
}

func TestExecutePlanWithResolvedTurnPlanDataOverflowStopsExecution(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	resolvedPlan := testResolvedTurnPlan(t, "turn-plan-data-overflow", "pipeline-v1", 3, []string{"retry"})
	resolvedPlan = withTightPlanQueueLimits(t, resolvedPlan, 1)

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-data-overflow",
			TurnID:               "turn-plan-data-overflow",
			EventID:              "evt-plan-data-overflow",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    20,
			RuntimeSequence:      21,
			AuthorityEpoch:       3,
			RuntimeTimestampMS:   200,
			WallClockTimestampMS: 200,
			ResolvedTurnPlan:     &resolvedPlan,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "data-a", NodeType: "provider", Lane: eventabi.LaneData},
				{NodeID: "data-b", NodeType: "provider", Lane: eventabi.LaneData},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if trace.Completed {
		t.Fatalf("expected data overflow to stop execution, got %+v", trace)
	}
	if trace.TerminalReason != "lane_queue_capacity_exceeded" {
		t.Fatalf("expected queue-capacity terminal reason, got %q", trace.TerminalReason)
	}

	if len(trace.Nodes) != 1 || trace.Nodes[0].NodeID != "data-b" {
		t.Fatalf("expected overflowed node to be recorded first, got %+v", trace.Nodes)
	}
	denied := trace.Nodes[0].Decision
	if denied.Allowed || denied.Outcome == nil || denied.Outcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected shed denial on data overflow, got %+v", denied)
	}

	foundShed := false
	for _, signal := range trace.ControlSignals {
		if signal.Signal == "shed" {
			foundShed = true
			break
		}
	}
	if !foundShed {
		t.Fatalf("expected shed control signal, got %+v", trace.ControlSignals)
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

func TestExecutePlanWithExecutionPoolSaturationStopsControlPath(t *testing.T) {
	t.Parallel()

	pool := runtimeexecutionpool.NewManager(1)
	release := saturateExecutionPoolForTest(t, pool)
	scheduler := NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-pool-saturated-control-1",
			TurnID:               "turn-plan-pool-saturated-control-1",
			EventID:              "evt-plan-pool-saturated-control-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    20,
			RuntimeSequence:      21,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   200,
			WallClockTimestampMS: 200,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "control-node", NodeType: "admission", Lane: eventabi.LaneControl},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected saturated execute plan error: %v", err)
	}
	if trace.Completed {
		t.Fatalf("expected saturated control dispatch to stop execution, got %+v", trace)
	}
	if trace.TerminalReason != "execution_pool_saturated" {
		t.Fatalf("expected execution_pool_saturated terminal reason, got %q", trace.TerminalReason)
	}
	if len(trace.Nodes) != 1 {
		t.Fatalf("expected one node result, got %+v", trace.Nodes)
	}
	denied := trace.Nodes[0].Decision
	if denied.Allowed || denied.Outcome == nil || denied.Outcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected shed denial for saturated control dispatch, got %+v", denied)
	}
	if denied.Outcome.Reason != "execution_pool_saturated" {
		t.Fatalf("expected execution_pool_saturated outcome reason, got %+v", denied.Outcome)
	}
	if denied.ControlSignal == nil || denied.ControlSignal.Signal != "shed" || denied.ControlSignal.Reason != "execution_pool_saturated" {
		t.Fatalf("expected saturated shed control signal, got %+v", denied.ControlSignal)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
	if stats := pool.Stats(); stats.Rejected < 1 {
		t.Fatalf("expected at least one rejected submission under saturation, got %+v", stats)
	}
}

func TestExecutePlanWithExecutionPoolSaturationKeepsTelemetryNonBlocking(t *testing.T) {
	t.Parallel()

	pool := runtimeexecutionpool.NewManager(1)
	release := saturateExecutionPoolForTest(t, pool)
	scheduler := NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-pool-saturated-telemetry-1",
			TurnID:               "turn-plan-pool-saturated-telemetry-1",
			EventID:              "evt-plan-pool-saturated-telemetry-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    30,
			RuntimeSequence:      31,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   300,
			WallClockTimestampMS: 300,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{NodeID: "telemetry-node", NodeType: "metrics", Lane: eventabi.LaneTelemetry},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected saturated telemetry execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected telemetry saturation to remain non-blocking, got %+v", trace)
	}
	if trace.TerminalReason != "" {
		t.Fatalf("expected no terminal reason for telemetry saturation, got %q", trace.TerminalReason)
	}
	if len(trace.Nodes) != 1 {
		t.Fatalf("expected one node result, got %+v", trace.Nodes)
	}
	telemetryDecision := trace.Nodes[0].Decision
	if !telemetryDecision.Allowed {
		t.Fatalf("expected telemetry decision to stay allowed under saturation, got %+v", telemetryDecision)
	}
	if telemetryDecision.Outcome != nil {
		t.Fatalf("expected telemetry saturation to clear terminal outcome, got %+v", telemetryDecision.Outcome)
	}
	if telemetryDecision.ControlSignal == nil || telemetryDecision.ControlSignal.Signal != "shed" || telemetryDecision.ControlSignal.Reason != "execution_pool_saturated" {
		t.Fatalf("expected telemetry saturation shed signal, got %+v", telemetryDecision.ControlSignal)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
}

func TestExecutePlanWithNodeConcurrencyLimitStopsControlPath(t *testing.T) {
	t.Parallel()

	pool := runtimeexecutionpool.NewManager(4)
	release := holdExecutionPoolFairnessSlotForTest(t, pool, "provider-heavy", 1)
	scheduler := NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-node-concurrency-control-1",
			TurnID:               "turn-plan-node-concurrency-control-1",
			EventID:              "evt-plan-node-concurrency-control-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    40,
			RuntimeSequence:      41,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   400,
			WallClockTimestampMS: 400,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:           "control-heavy-node",
					NodeType:         "admission",
					Lane:             eventabi.LaneControl,
					FairnessKey:      "provider-heavy",
					ConcurrencyLimit: 1,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected node-concurrency execute plan error: %v", err)
	}
	if trace.Completed {
		t.Fatalf("expected node concurrency limit to stop control-path execution, got %+v", trace)
	}
	if trace.TerminalReason != "node_concurrency_limited" {
		t.Fatalf("expected terminal reason node_concurrency_limited, got %q", trace.TerminalReason)
	}
	if len(trace.Nodes) != 1 {
		t.Fatalf("expected one node result under node concurrency limit, got %+v", trace.Nodes)
	}
	denied := trace.Nodes[0].Decision
	if denied.Allowed || denied.Outcome == nil || denied.Outcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected shed denial for node concurrency limit, got %+v", denied)
	}
	if denied.Outcome.Reason != "node_concurrency_limited" {
		t.Fatalf("expected node_concurrency_limited outcome reason, got %+v", denied.Outcome)
	}
	if denied.ControlSignal == nil || denied.ControlSignal.Signal != "shed" || denied.ControlSignal.Reason != "node_concurrency_limited" {
		t.Fatalf("expected node concurrency shed control signal, got %+v", denied.ControlSignal)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
	if stats := pool.Stats(); stats.RejectedByConcurrency < 1 {
		t.Fatalf("expected at least one concurrency-limited rejection, got %+v", stats)
	}
}

func TestExecutePlanWithNodeConcurrencyLimitKeepsTelemetryNonBlocking(t *testing.T) {
	t.Parallel()

	pool := runtimeexecutionpool.NewManager(4)
	release := holdExecutionPoolFairnessSlotForTest(t, pool, "provider-heavy", 1)
	scheduler := NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-node-concurrency-telemetry-1",
			TurnID:               "turn-plan-node-concurrency-telemetry-1",
			EventID:              "evt-plan-node-concurrency-telemetry-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    50,
			RuntimeSequence:      51,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   500,
			WallClockTimestampMS: 500,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:           "telemetry-heavy-node",
					NodeType:         "metrics",
					Lane:             eventabi.LaneTelemetry,
					FairnessKey:      "provider-heavy",
					ConcurrencyLimit: 1,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected telemetry node-concurrency execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected telemetry concurrency limiting to remain non-blocking, got %+v", trace)
	}
	if trace.TerminalReason != "" {
		t.Fatalf("expected no terminal reason for telemetry node concurrency limiting, got %q", trace.TerminalReason)
	}
	if len(trace.Nodes) != 1 {
		t.Fatalf("expected one telemetry node result, got %+v", trace.Nodes)
	}
	telemetryDecision := trace.Nodes[0].Decision
	if !telemetryDecision.Allowed {
		t.Fatalf("expected telemetry decision to remain allowed, got %+v", telemetryDecision)
	}
	if telemetryDecision.Outcome != nil {
		t.Fatalf("expected telemetry decision outcome to be cleared, got %+v", telemetryDecision.Outcome)
	}
	if telemetryDecision.ControlSignal == nil || telemetryDecision.ControlSignal.Signal != "shed" || telemetryDecision.ControlSignal.Reason != "node_concurrency_limited" {
		t.Fatalf("expected telemetry node-concurrency shed signal, got %+v", telemetryDecision.ControlSignal)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
	if stats := pool.Stats(); stats.RejectedByConcurrency < 1 {
		t.Fatalf("expected at least one concurrency-limited rejection, got %+v", stats)
	}
}

func TestExecutePlanRejectsNegativeNodeConcurrencyLimit(t *testing.T) {
	t.Parallel()

	scheduler := NewScheduler(localadmission.Evaluator{})
	_, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-negative-node-concurrency-1",
			TurnID:               "turn-plan-negative-node-concurrency-1",
			EventID:              "evt-plan-negative-node-concurrency-1",
			RuntimeTimestampMS:   1,
			WallClockTimestampMS: 1,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:           "invalid-node",
					NodeType:         "admission",
					Lane:             eventabi.LaneControl,
					ConcurrencyLimit: -1,
				},
			},
		},
	)
	if err == nil {
		t.Fatalf("expected negative concurrency limit validation error")
	}
	if !strings.Contains(err.Error(), "concurrency_limit must be >=0") {
		t.Fatalf("expected concurrency limit validation error, got %v", err)
	}
}

func TestExecutePlanUsesResolvedTurnPlanNodeExecutionPoliciesForConcurrencyAndFairness(t *testing.T) {
	t.Parallel()

	pool := runtimeexecutionpool.NewManager(4)
	release := holdExecutionPoolFairnessSlotForTest(t, pool, "provider-heavy", 1)
	scheduler := NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)

	resolvedPlan := testResolvedTurnPlan(t, "turn-plan-node-policy-1", "pipeline-v1", 5, []string{"retry"})
	resolvedPlan.NodeExecutionPolicies = map[string]controlplane.NodeExecutionPolicy{
		"provider-heavy-node": {
			ConcurrencyLimit: 1,
			FairnessKey:      "provider-heavy",
		},
	}

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-node-policy-1",
			TurnID:               "turn-plan-node-policy-1",
			EventID:              "evt-plan-node-policy-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    60,
			RuntimeSequence:      61,
			AuthorityEpoch:       5,
			RuntimeTimestampMS:   600,
			WallClockTimestampMS: 600,
			ResolvedTurnPlan:     &resolvedPlan,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:   "provider-heavy-node",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if trace.Completed {
		t.Fatalf("expected resolved-turn-plan node execution policy to enforce node concurrency limit, got %+v", trace)
	}
	if trace.TerminalReason != "node_concurrency_limited" {
		t.Fatalf("expected node_concurrency_limited terminal reason, got %q", trace.TerminalReason)
	}
	if len(trace.Nodes) != 1 {
		t.Fatalf("expected one node decision, got %+v", trace.Nodes)
	}
	if denied := trace.Nodes[0].Decision; denied.Outcome == nil || denied.Outcome.Reason != "node_concurrency_limited" {
		t.Fatalf("expected node_concurrency_limited outcome from resolved-turn-plan policy, got %+v", denied)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
	if stats := pool.Stats(); stats.RejectedByConcurrency < 1 {
		t.Fatalf("expected concurrency-limited rejection evidence, got %+v", stats)
	}
}

func TestExecutePlanUsesResolvedTurnPlanEdgeFairnessKeyFallback(t *testing.T) {
	t.Parallel()

	pool := runtimeexecutionpool.NewManager(4)
	release := holdExecutionPoolFairnessSlotForTest(t, pool, "edge-fairness-heavy", 1)
	scheduler := NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)

	resolvedPlan := testResolvedTurnPlan(t, "turn-plan-edge-fairness-1", "pipeline-v1", 6, []string{"retry"})
	defaultEdge := resolvedPlan.EdgeBufferPolicies["default"]
	defaultEdge.FairnessKey = "edge-fairness-heavy"
	resolvedPlan.EdgeBufferPolicies["default"] = defaultEdge
	resolvedPlan.NodeExecutionPolicies = map[string]controlplane.NodeExecutionPolicy{
		"provider-heavy-node": {
			ConcurrencyLimit: 1,
		},
	}

	trace, err := scheduler.ExecutePlan(
		SchedulingInput{
			SessionID:            "sess-plan-edge-fairness-1",
			TurnID:               "turn-plan-edge-fairness-1",
			EventID:              "evt-plan-edge-fairness-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    70,
			RuntimeSequence:      71,
			AuthorityEpoch:       6,
			RuntimeTimestampMS:   700,
			WallClockTimestampMS: 700,
			ResolvedTurnPlan:     &resolvedPlan,
		},
		ExecutionPlan{
			Nodes: []NodeSpec{
				{
					NodeID:   "provider-heavy-node",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if trace.Completed {
		t.Fatalf("expected edge fairness key fallback to enforce shared-key limit, got %+v", trace)
	}
	if trace.TerminalReason != "node_concurrency_limited" {
		t.Fatalf("expected node_concurrency_limited terminal reason, got %q", trace.TerminalReason)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
	if stats := pool.Stats(); stats.RejectedByConcurrency < 1 {
		t.Fatalf("expected concurrency-limited rejection evidence, got %+v", stats)
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

func TestSchedulerEmitsTelemetryEvents(t *testing.T) {
	sink := telemetry.NewMemorySink()
	pipeline := telemetry.NewPipeline(sink, telemetry.Config{QueueCapacity: 32})
	previous := telemetry.DefaultEmitter()
	telemetry.SetDefaultEmitter(pipeline)
	t.Cleanup(func() {
		telemetry.SetDefaultEmitter(previous)
		_ = pipeline.Close()
	})

	scheduler := NewScheduler(localadmission.Evaluator{})
	_, err := scheduler.NodeDispatch(SchedulingInput{
		SessionID:            "sess-rk07-telemetry-1",
		TurnID:               "turn-rk07-telemetry-1",
		EventID:              "evt-rk07-telemetry-1",
		PipelineVersion:      "pipeline-v1",
		NodeID:               "node-rk07-1",
		NodeType:             "provider",
		EdgeID:               "edge-rk07-1",
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
		Shed:                 true,
	})
	if err != nil {
		t.Fatalf("unexpected scheduler error: %v", err)
	}
	if err := pipeline.Close(); err != nil {
		t.Fatalf("unexpected pipeline close error: %v", err)
	}

	var shedRateMetric bool
	var nodeSpan bool
	var shedLog bool
	for _, event := range sink.Events() {
		if event.Correlation.SessionID != "sess-rk07-telemetry-1" {
			continue
		}
		if event.Correlation.NodeID != "node-rk07-1" || event.Correlation.EdgeID != "edge-rk07-1" {
			t.Fatalf("expected node/edge correlation IDs, got %+v", event.Correlation)
		}
		if event.Kind == telemetry.EventKindMetric && event.Metric != nil && event.Metric.Name == telemetry.MetricShedRate {
			if event.Metric.Attributes["node_id"] != "node-rk07-1" || event.Metric.Attributes["edge_id"] != "edge-rk07-1" {
				t.Fatalf("expected node/edge metric linkage labels, got %+v", event.Metric.Attributes)
			}
			shedRateMetric = true
		}
		if event.Kind == telemetry.EventKindSpan && event.Span != nil && event.Span.Name == "node_span" {
			if event.Span.Attributes["node_id"] != "node-rk07-1" || event.Span.Attributes["edge_id"] != "edge-rk07-1" {
				t.Fatalf("expected node/edge span linkage labels, got %+v", event.Span.Attributes)
			}
			nodeSpan = true
		}
		if event.Kind == telemetry.EventKindLog && event.Log != nil && event.Log.Name == "scheduling_shed" {
			if event.Log.Attributes["node_id"] != "node-rk07-1" || event.Log.Attributes["edge_id"] != "edge-rk07-1" {
				t.Fatalf("expected node/edge log linkage labels, got %+v", event.Log.Attributes)
			}
			shedLog = true
		}
	}
	if !shedRateMetric || !nodeSpan || !shedLog {
		t.Fatalf("expected scheduler telemetry events, got shed_rate=%v node_span=%v shed_log=%v", shedRateMetric, nodeSpan, shedLog)
	}
}

func testResolvedTurnPlan(t *testing.T, turnID string, pipelineVersion string, authorityEpoch int64, actions []string) controlplane.ResolvedTurnPlan {
	t.Helper()

	resolver := planresolver.Resolver{}
	plan, err := resolver.Resolve(planresolver.Input{
		TurnID:             turnID,
		PipelineVersion:    pipelineVersion,
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     authorityEpoch,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       fmt.Sprintf("routing-view/%s", turnID),
			AdmissionPolicySnapshot:   fmt.Sprintf("admission-policy/%s", turnID),
			ABICompatibilitySnapshot:  fmt.Sprintf("abi-compat/%s", turnID),
			VersionResolutionSnapshot: fmt.Sprintf("version-resolution/%s", turnID),
			PolicyResolutionSnapshot:  fmt.Sprintf("policy-resolution/%s", turnID),
			ProviderHealthSnapshot:    fmt.Sprintf("provider-health/%s", turnID),
		},
		AllowedAdaptiveActions: append([]string(nil), actions...),
	})
	if err != nil {
		t.Fatalf("resolve test plan: %v", err)
	}
	return plan
}

func withTightPlanQueueLimits(t *testing.T, plan controlplane.ResolvedTurnPlan, limit int) controlplane.ResolvedTurnPlan {
	t.Helper()

	policy := plan.EdgeBufferPolicies["default"]
	policy.MaxQueueItems = limit
	plan.EdgeBufferPolicies["default"] = policy
	plan.FlowControl.Watermarks = controlplane.FlowWatermarks{
		DataLane:      controlplane.WatermarkThreshold{High: limit, Low: 0},
		ControlLane:   controlplane.WatermarkThreshold{High: limit, Low: 0},
		TelemetryLane: controlplane.WatermarkThreshold{High: limit, Low: 0},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("tight plan queue limits should remain valid: %v", err)
	}
	return plan
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func saturateExecutionPoolForTest(t *testing.T, pool *runtimeexecutionpool.Manager) chan struct{} {
	t.Helper()

	release := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(runtimeexecutionpool.Task{
		ID: "blocking",
		Run: func() error {
			close(started)
			<-release
			return nil
		},
	}); err != nil {
		t.Fatalf("submit blocking saturation task: %v", err)
	}
	<-started

	if err := pool.Submit(runtimeexecutionpool.Task{
		ID:  "queued",
		Run: func() error { return nil },
	}); err != nil {
		t.Fatalf("submit queued saturation task: %v", err)
	}

	return release
}

func holdExecutionPoolFairnessSlotForTest(
	t *testing.T,
	pool *runtimeexecutionpool.Manager,
	fairnessKey string,
	maxOutstanding int,
) chan struct{} {
	t.Helper()

	release := make(chan struct{})
	started := make(chan struct{})
	if err := pool.Submit(runtimeexecutionpool.Task{
		ID:             fmt.Sprintf("blocking-%s", fairnessKey),
		FairnessKey:    fairnessKey,
		MaxOutstanding: maxOutstanding,
		Run: func() error {
			close(started)
			<-release
			return nil
		},
	}); err != nil {
		t.Fatalf("submit fairness-slot blocking task: %v", err)
	}
	<-started
	return release
}
