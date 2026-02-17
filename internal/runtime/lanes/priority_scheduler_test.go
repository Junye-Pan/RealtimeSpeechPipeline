package lanes

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/planresolver"
)

func TestPrioritySchedulerDispatchesByLanePriority(t *testing.T) {
	t.Parallel()

	scheduler := mustNewPriorityScheduler(t, DispatchSchedulerConfig{
		SessionID:       "sess-lane-priority-1",
		TurnID:          "turn-lane-priority-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  3,
		QueueLimits:     QueueLimits{Control: 4, Data: 4, Telemetry: 4},
		Watermarks: QueueWatermarks{
			Control:   controlplane.WatermarkThreshold{High: 3, Low: 1},
			Data:      controlplane.WatermarkThreshold{High: 3, Low: 1},
			Telemetry: controlplane.WatermarkThreshold{High: 3, Low: 1},
		},
	})

	enqueue := []EnqueueInput{
		{ItemID: "data-1", Lane: eventabi.LaneData, EventID: "evt-data-1", TransportSequence: 1, RuntimeSequence: 1},
		{ItemID: "telemetry-1", Lane: eventabi.LaneTelemetry, EventID: "evt-telemetry-1", TransportSequence: 2, RuntimeSequence: 2},
		{ItemID: "control-1", Lane: eventabi.LaneControl, EventID: "evt-control-1", TransportSequence: 3, RuntimeSequence: 3},
		{ItemID: "data-2", Lane: eventabi.LaneData, EventID: "evt-data-2", TransportSequence: 4, RuntimeSequence: 4},
	}
	for _, in := range enqueue {
		result, err := scheduler.Enqueue(in)
		if err != nil {
			t.Fatalf("enqueue %s failed: %v", in.ItemID, err)
		}
		if !result.Accepted {
			t.Fatalf("expected enqueue accepted for %s, got %+v", in.ItemID, result)
		}
	}

	expected := []string{"control-1", "data-1", "data-2", "telemetry-1"}
	got := make([]string, 0, len(expected))
	for {
		next, ok, err := scheduler.NextDispatch()
		if err != nil {
			t.Fatalf("next dispatch failed: %v", err)
		}
		if !ok {
			break
		}
		got = append(got, next.Item.ItemID)
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d dispatches, got %d (%v)", len(expected), len(got), got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("priority order mismatch at %d: expected %s got %s", i, expected[i], got[i])
		}
	}
}

func TestPrioritySchedulerTelemetryOverflowDoesNotBlockControl(t *testing.T) {
	t.Parallel()

	scheduler := mustNewPriorityScheduler(t, DispatchSchedulerConfig{
		SessionID:       "sess-lane-telemetry-overflow-1",
		TurnID:          "turn-lane-telemetry-overflow-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  4,
		QueueLimits:     QueueLimits{Control: 2, Data: 2, Telemetry: 1},
		Watermarks: QueueWatermarks{
			Control:   controlplane.WatermarkThreshold{High: 1, Low: 0},
			Data:      controlplane.WatermarkThreshold{High: 1, Low: 0},
			Telemetry: controlplane.WatermarkThreshold{High: 1, Low: 0},
		},
	})

	firstTelemetry, err := scheduler.Enqueue(EnqueueInput{
		ItemID:            "telemetry-1",
		Lane:              eventabi.LaneTelemetry,
		EventID:           "evt-telemetry-1",
		TransportSequence: 1,
		RuntimeSequence:   1,
	})
	if err != nil || !firstTelemetry.Accepted {
		t.Fatalf("expected first telemetry enqueue to succeed, got result=%+v err=%v", firstTelemetry, err)
	}

	overflowTelemetry, err := scheduler.Enqueue(EnqueueInput{
		ItemID:            "telemetry-2",
		Lane:              eventabi.LaneTelemetry,
		EventID:           "evt-telemetry-2",
		TransportSequence: 2,
		RuntimeSequence:   2,
	})
	if err != nil {
		t.Fatalf("unexpected telemetry overflow error: %v", err)
	}
	if overflowTelemetry.Accepted || !overflowTelemetry.Dropped {
		t.Fatalf("expected telemetry overflow to drop best effort, got %+v", overflowTelemetry)
	}
	if !containsLaneSignal(overflowTelemetry.Signals, "drop_notice") {
		t.Fatalf("expected drop_notice on telemetry overflow, got %+v", overflowTelemetry.Signals)
	}

	controlResult, err := scheduler.Enqueue(EnqueueInput{
		ItemID:            "control-1",
		Lane:              eventabi.LaneControl,
		EventID:           "evt-control-1",
		TransportSequence: 3,
		RuntimeSequence:   3,
	})
	if err != nil || !controlResult.Accepted {
		t.Fatalf("expected control enqueue after telemetry overflow to succeed, got result=%+v err=%v", controlResult, err)
	}

	first, ok, err := scheduler.NextDispatch()
	if err != nil {
		t.Fatalf("unexpected next dispatch error: %v", err)
	}
	if !ok || first.Item.ItemID != "control-1" {
		t.Fatalf("expected control dispatch to preempt telemetry, got %+v ok=%v", first, ok)
	}
}

func TestPrioritySchedulerDataOverflowReturnsShedOutcome(t *testing.T) {
	t.Parallel()

	scheduler := mustNewPriorityScheduler(t, DispatchSchedulerConfig{
		SessionID:       "sess-lane-data-overflow-1",
		TurnID:          "turn-lane-data-overflow-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  5,
		QueueLimits:     QueueLimits{Control: 2, Data: 1, Telemetry: 2},
		Watermarks: QueueWatermarks{
			Control:   controlplane.WatermarkThreshold{High: 1, Low: 0},
			Data:      controlplane.WatermarkThreshold{High: 1, Low: 0},
			Telemetry: controlplane.WatermarkThreshold{High: 1, Low: 0},
		},
	})

	first, err := scheduler.Enqueue(EnqueueInput{
		ItemID:            "data-1",
		Lane:              eventabi.LaneData,
		EventID:           "evt-data-1",
		TransportSequence: 1,
		RuntimeSequence:   1,
	})
	if err != nil || !first.Accepted {
		t.Fatalf("expected first data enqueue success, got result=%+v err=%v", first, err)
	}

	overflow, err := scheduler.Enqueue(EnqueueInput{
		ItemID:            "data-2",
		Lane:              eventabi.LaneData,
		EventID:           "evt-data-2",
		TransportSequence: 2,
		RuntimeSequence:   2,
	})
	if err != nil {
		t.Fatalf("unexpected overflow enqueue error: %v", err)
	}
	if overflow.Accepted || !overflow.Dropped {
		t.Fatalf("expected data overflow to be rejected/dropped, got %+v", overflow)
	}
	if overflow.ShedOutcome == nil || overflow.ShedOutcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected scheduling shed outcome on data overflow, got %+v", overflow.ShedOutcome)
	}
	if !containsLaneSignal(overflow.Signals, "shed") {
		t.Fatalf("expected shed control signal on data overflow, got %+v", overflow.Signals)
	}
}

func TestPrioritySchedulerEmitsRecoverySignalAfterDrainBelowLowWatermark(t *testing.T) {
	t.Parallel()

	scheduler := mustNewPriorityScheduler(t, DispatchSchedulerConfig{
		SessionID:       "sess-lane-recovery-1",
		TurnID:          "turn-lane-recovery-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  6,
		QueueLimits:     QueueLimits{Control: 2, Data: 2, Telemetry: 2},
		Watermarks: QueueWatermarks{
			Control:   controlplane.WatermarkThreshold{High: 1, Low: 0},
			Data:      controlplane.WatermarkThreshold{High: 1, Low: 0},
			Telemetry: controlplane.WatermarkThreshold{High: 1, Low: 0},
		},
	})

	enqueue, err := scheduler.Enqueue(EnqueueInput{
		ItemID:               "data-1",
		Lane:                 eventabi.LaneData,
		EventID:              "evt-data-1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	if !containsLaneSignal(enqueue.Signals, "flow_xoff") {
		t.Fatalf("expected high-watermark flow_xoff signal, got %+v", enqueue.Signals)
	}

	dispatch, ok, err := scheduler.NextDispatch()
	if err != nil {
		t.Fatalf("unexpected next dispatch error: %v", err)
	}
	if !ok || dispatch.Item.ItemID != "data-1" {
		t.Fatalf("expected data dispatch, got %+v ok=%v", dispatch, ok)
	}
	if !containsLaneSignal(dispatch.Signals, "flow_xon") {
		t.Fatalf("expected recovery flow_xon signal, got %+v", dispatch.Signals)
	}
}

func TestPrioritySchedulerEmitsEdgeMetricsWithStableLinkageLabels(t *testing.T) {
	sink := telemetry.NewMemorySink()
	pipeline := telemetry.NewPipeline(sink, telemetry.Config{QueueCapacity: 64})
	previous := telemetry.DefaultEmitter()
	telemetry.SetDefaultEmitter(pipeline)
	t.Cleanup(func() {
		telemetry.SetDefaultEmitter(previous)
		_ = pipeline.Close()
	})

	scheduler := mustNewPriorityScheduler(t, DispatchSchedulerConfig{
		SessionID:       "sess-lane-telemetry-linkage-1",
		TurnID:          "turn-lane-telemetry-linkage-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  7,
		QueueLimits:     QueueLimits{Control: 2, Data: 2, Telemetry: 1},
		Watermarks: QueueWatermarks{
			Control:   controlplane.WatermarkThreshold{High: 1, Low: 0},
			Data:      controlplane.WatermarkThreshold{High: 1, Low: 0},
			Telemetry: controlplane.WatermarkThreshold{High: 1, Low: 0},
		},
	})

	if _, err := scheduler.Enqueue(EnqueueInput{
		ItemID:               "telemetry-1",
		Lane:                 eventabi.LaneTelemetry,
		EventID:              "evt-telemetry-linkage-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	}); err != nil {
		t.Fatalf("enqueue telemetry item 1: %v", err)
	}
	if _, err := scheduler.Enqueue(EnqueueInput{
		ItemID:               "telemetry-2",
		Lane:                 eventabi.LaneTelemetry,
		EventID:              "evt-telemetry-linkage-2",
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
	}); err != nil {
		t.Fatalf("enqueue telemetry item 2 overflow: %v", err)
	}
	if _, _, err := scheduler.NextDispatch(); err != nil {
		t.Fatalf("next dispatch: %v", err)
	}

	if err := pipeline.Close(); err != nil {
		t.Fatalf("close telemetry pipeline: %v", err)
	}

	var queueAccepted bool
	var queueDequeue bool
	var dropsTotal bool
	for _, event := range sink.Events() {
		if event.Correlation.SessionID != "sess-lane-telemetry-linkage-1" {
			continue
		}
		if event.Correlation.EdgeID == "" {
			t.Fatalf("expected edge_id correlation, got %+v", event.Correlation)
		}
		if event.Kind != telemetry.EventKindMetric || event.Metric == nil {
			continue
		}
		edgeIDAttr := event.Metric.Attributes["edge_id"]
		if edgeIDAttr == "" || edgeIDAttr != event.Correlation.EdgeID {
			t.Fatalf("expected stable edge_id metric linkage labels, got attrs=%+v correlation=%+v", event.Metric.Attributes, event.Correlation)
		}
		if event.Metric.Name == telemetry.MetricEdgeQueueDepth && event.Metric.Attributes["status"] == "accepted" {
			queueAccepted = true
		}
		if event.Metric.Name == telemetry.MetricEdgeQueueDepth && event.Metric.Attributes["status"] == "dequeue" {
			queueDequeue = true
		}
		if event.Metric.Name == telemetry.MetricEdgeDropsTotal && event.Metric.Attributes["strategy"] == "best_effort_drop" {
			dropsTotal = true
		}
	}

	if !queueAccepted || !queueDequeue || !dropsTotal {
		t.Fatalf("expected edge queue/drop metric coverage, got queueAccepted=%v queueDequeue=%v dropsTotal=%v", queueAccepted, queueDequeue, dropsTotal)
	}
}

func TestConfigFromResolvedTurnPlanDerivesQueueAndWatermarkSettings(t *testing.T) {
	t.Parallel()

	plan := testPlanForLaneScheduler(t, "turn-config-plan-1", "pipeline-v1", 11)
	plan.FlowControl.Watermarks = controlplane.FlowWatermarks{
		DataLane:      controlplane.WatermarkThreshold{High: 70, Low: 30},
		ControlLane:   controlplane.WatermarkThreshold{High: 16, Low: 8},
		TelemetryLane: controlplane.WatermarkThreshold{High: 90, Low: 40},
	}
	plan.EdgeBufferPolicies["default"] = controlplane.EdgeBufferPolicy{
		Strategy:                 controlplane.BufferStrategyDrop,
		MaxQueueItems:            80,
		MaxQueueMS:               300,
		MaxQueueBytes:            262144,
		MaxLatencyContributionMS: 100,
		Watermarks: controlplane.EdgeWatermarks{
			QueueItems: &controlplane.WatermarkThreshold{High: 60, Low: 20},
		},
		LaneHandling: controlplane.LaneHandling{
			DataLane:      "drop",
			ControlLane:   "non_blocking_priority",
			TelemetryLane: "best_effort_drop",
		},
		DefaultingSource: "execution_profile_default",
	}

	cfg, err := ConfigFromResolvedTurnPlan("sess-config-plan-1", "turn-config-plan-1", plan)
	if err != nil {
		t.Fatalf("config from resolved plan: %v", err)
	}
	if cfg.PipelineVersion != "pipeline-v1" || cfg.AuthorityEpoch != 11 {
		t.Fatalf("expected pipeline/epoch from plan, got %+v", cfg)
	}
	if cfg.QueueLimits.Control != 80 || cfg.QueueLimits.Data != 80 || cfg.QueueLimits.Telemetry != 90 {
		t.Fatalf("unexpected queue limits derived from plan: %+v", cfg.QueueLimits)
	}
	if cfg.Watermarks.Control.High != 16 || cfg.Watermarks.Data.High != 70 || cfg.Watermarks.Telemetry.High != 90 {
		t.Fatalf("unexpected watermarks derived from plan: %+v", cfg.Watermarks)
	}
}

func TestConfigFromResolvedTurnPlanRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	if _, err := ConfigFromResolvedTurnPlan("", "turn", controlplane.ResolvedTurnPlan{}); err == nil {
		t.Fatalf("expected missing session_id to fail")
	}

	plan := testPlanForLaneScheduler(t, "turn-invalid-plan", "pipeline-v1", 4)
	plan.FlowControl.Watermarks.ControlLane = controlplane.WatermarkThreshold{High: 0, Low: 0}
	if _, err := ConfigFromResolvedTurnPlan("sess-invalid-plan", "turn-invalid-plan", plan); err == nil {
		t.Fatalf("expected invalid plan watermark to fail")
	}
}

func mustNewPriorityScheduler(t *testing.T, cfg DispatchSchedulerConfig) *PriorityScheduler {
	t.Helper()
	scheduler, err := NewPriorityScheduler(localadmission.Evaluator{}, cfg)
	if err != nil {
		t.Fatalf("new priority scheduler: %v", err)
	}
	return scheduler
}

func containsLaneSignal(signals []eventabi.ControlSignal, signal string) bool {
	for _, candidate := range signals {
		if candidate.Signal == signal {
			return true
		}
	}
	return false
}

func testPlanForLaneScheduler(t *testing.T, turnID string, pipelineVersion string, authorityEpoch int64) controlplane.ResolvedTurnPlan {
	t.Helper()

	plan, err := planresolver.Resolver{}.Resolve(planresolver.Input{
		TurnID:             turnID,
		PipelineVersion:    pipelineVersion,
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     authorityEpoch,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/test",
			AdmissionPolicySnapshot:   "admission-policy/test",
			ABICompatibilitySnapshot:  "abi-compat/test",
			VersionResolutionSnapshot: "version-resolution/test",
			PolicyResolutionSnapshot:  "policy-resolution/test",
			ProviderHealthSnapshot:    "provider-health/test",
		},
		AllowedAdaptiveActions: []string{"retry"},
	})
	if err != nil {
		t.Fatalf("resolve plan for lane scheduler test: %v", err)
	}
	return plan
}
