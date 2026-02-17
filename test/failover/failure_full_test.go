package failover_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/buffering"
	runtimeexternalnode "github.com/tiger/realtime-speech-pipeline/internal/runtime/externalnode"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/guard"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/lanes"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/nodehost"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/planresolver"
	runtimestate "github.com/tiger/realtime-speech-pipeline/internal/runtime/state"
	runtimesync "github.com/tiger/realtime-speech-pipeline/internal/runtime/sync"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestF2NodeTimeoutFailureFull(t *testing.T) {
	t.Parallel()

	nodeResult, err := nodehost.HandleFailure(nodehost.NodeFailureInput{
		SessionID:            "sess-f2-full",
		TurnID:               "turn-f2-full",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f2-full",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected node failure shaping error: %v", err)
	}
	if !nodeResult.Terminal || nodeResult.TerminalReason != "node_timeout_or_failure" {
		t.Fatalf("expected terminal node timeout/failure recommendation, got %+v", nodeResult)
	}

	arbiter := turnarbiter.New()
	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-f2-full",
		TurnID:               "turn-f2-full",
		EventID:              "evt-f2-full-active",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
		AuthorityEpoch:       3,
		NodeTimeoutOrFailure: true,
	})
	if err != nil {
		t.Fatalf("unexpected active node failure error: %v", err)
	}
	if len(active.Events) != 2 || active.Events[0].Name != "abort" || active.Events[0].Reason != "node_timeout_or_failure" || active.Events[1].Name != "close" {
		t.Fatalf("expected abort(node_timeout_or_failure)->close, got %+v", active.Events)
	}
}

func TestF4EdgePressureOverflowFull(t *testing.T) {
	t.Parallel()

	result, err := buffering.HandleEdgePressure(localadmission.Evaluator{}, buffering.PressureInput{
		SessionID:            "sess-f4-full",
		TurnID:               "turn-f4-full",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-a",
		EventID:              "evt-f4-full",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       4,
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       300,
		DropRangeEnd:         330,
		HighWatermark:        true,
		EmitRecovery:         true,
		Shed:                 true,
	})
	if err != nil {
		t.Fatalf("unexpected edge pressure handling error: %v", err)
	}
	if len(result.Signals) < 4 {
		t.Fatalf("expected watermark/flow/drop/recovery signals, got %+v", result.Signals)
	}
	if result.ShedOutcome == nil || result.ShedOutcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected scheduling-point shed outcome, got %+v", result.ShedOutcome)
	}
}

func TestF4LaneQueuePressureControlPreemptionFull(t *testing.T) {
	t.Parallel()

	plan := resolvedPlanForFailoverSchedulerTests(t, "turn-f4-lane-priority-full", 12)
	plan.EdgeBufferPolicies["default"] = controlplane.EdgeBufferPolicy{
		Strategy:                 controlplane.BufferStrategyDrop,
		MaxQueueItems:            1,
		MaxQueueMS:               120,
		MaxQueueBytes:            65536,
		MaxLatencyContributionMS: 60,
		Watermarks: controlplane.EdgeWatermarks{
			QueueItems: &controlplane.WatermarkThreshold{High: 1, Low: 0},
		},
		LaneHandling: controlplane.LaneHandling{
			DataLane:      "drop",
			ControlLane:   "non_blocking_priority",
			TelemetryLane: "best_effort_drop",
		},
		DefaultingSource: "explicit_edge_config",
	}
	plan.FlowControl.Watermarks = controlplane.FlowWatermarks{
		DataLane:      controlplane.WatermarkThreshold{High: 1, Low: 0},
		ControlLane:   controlplane.WatermarkThreshold{High: 1, Low: 0},
		TelemetryLane: controlplane.WatermarkThreshold{High: 1, Low: 0},
	}

	cfg, err := lanes.ConfigFromResolvedTurnPlan("sess-f4-lane-priority-full", "turn-f4-lane-priority-full", plan)
	if err != nil {
		t.Fatalf("config from resolved plan: %v", err)
	}
	scheduler, err := lanes.NewPriorityScheduler(localadmission.Evaluator{}, cfg)
	if err != nil {
		t.Fatalf("new priority scheduler: %v", err)
	}

	firstTelemetry, err := scheduler.Enqueue(lanes.EnqueueInput{
		ItemID:            "telemetry-1",
		Lane:              eventabi.LaneTelemetry,
		EventID:           "evt-f4-telemetry-1",
		TransportSequence: 1,
		RuntimeSequence:   1,
	})
	if err != nil || !firstTelemetry.Accepted {
		t.Fatalf("expected first telemetry enqueue accepted, got result=%+v err=%v", firstTelemetry, err)
	}

	overflowTelemetry, err := scheduler.Enqueue(lanes.EnqueueInput{
		ItemID:            "telemetry-2",
		Lane:              eventabi.LaneTelemetry,
		EventID:           "evt-f4-telemetry-2",
		TransportSequence: 2,
		RuntimeSequence:   2,
	})
	if err != nil {
		t.Fatalf("unexpected telemetry overflow error: %v", err)
	}
	if overflowTelemetry.Accepted || !overflowTelemetry.Dropped || !containsControlSignal(overflowTelemetry.Signals, "drop_notice") {
		t.Fatalf("expected telemetry overflow drop_notice behavior, got %+v", overflowTelemetry)
	}

	controlEnqueue, err := scheduler.Enqueue(lanes.EnqueueInput{
		ItemID:            "control-1",
		Lane:              eventabi.LaneControl,
		EventID:           "evt-f4-control-1",
		TransportSequence: 3,
		RuntimeSequence:   3,
	})
	if err != nil || !controlEnqueue.Accepted {
		t.Fatalf("expected control enqueue accepted under telemetry pressure, got result=%+v err=%v", controlEnqueue, err)
	}

	dispatch, ok, err := scheduler.NextDispatch()
	if err != nil {
		t.Fatalf("next dispatch failed: %v", err)
	}
	if !ok || dispatch.Item.ItemID != "control-1" {
		t.Fatalf("expected control dispatch preemption under telemetry pressure, got %+v ok=%v", dispatch, ok)
	}
}

func TestF5SyncCoupledPartialLossFull(t *testing.T) {
	t.Parallel()

	discontinuity, err := buffering.HandleSyncLoss(buffering.SyncLossInput{
		SessionID:            "sess-f5-full",
		TurnID:               "turn-f5-full",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f5-disc",
		TransportSequence:    20,
		RuntimeSequence:      21,
		AuthorityEpoch:       5,
		RuntimeTimestampMS:   300,
		WallClockTimestampMS: 300,
		SyncDomain:           "av-sync",
		DiscontinuityID:      "disc-42",
	})
	if err != nil {
		t.Fatalf("unexpected discontinuity error: %v", err)
	}
	if discontinuity.Signal != "discontinuity" || discontinuity.SyncDomain != "av-sync" {
		t.Fatalf("expected deterministic discontinuity signal, got %+v", discontinuity)
	}

	atomicDrop, err := buffering.HandleSyncLoss(buffering.SyncLossInput{
		SessionID:            "sess-f5-full",
		TurnID:               "turn-f5-full",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f5-drop",
		TransportSequence:    21,
		RuntimeSequence:      22,
		AuthorityEpoch:       5,
		RuntimeTimestampMS:   301,
		WallClockTimestampMS: 301,
		UseAtomicDrop:        true,
		EdgeID:               "edge-av",
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       400,
		DropRangeEnd:         410,
	})
	if err != nil {
		t.Fatalf("unexpected atomic-drop error: %v", err)
	}
	if atomicDrop.Signal != "drop_notice" {
		t.Fatalf("expected drop_notice for atomic drop policy, got %+v", atomicDrop)
	}

	plan := resolvedPlanForFailoverSchedulerTests(t, "turn-f5-sync-coupled-full", 13)
	plan.EdgeBufferPolicies["default"] = controlplane.EdgeBufferPolicy{
		Strategy:                 controlplane.BufferStrategyDrop,
		MaxQueueItems:            1,
		MaxQueueMS:               120,
		MaxQueueBytes:            65536,
		MaxLatencyContributionMS: 60,
		Watermarks: controlplane.EdgeWatermarks{
			QueueItems: &controlplane.WatermarkThreshold{High: 1, Low: 0},
		},
		LaneHandling: controlplane.LaneHandling{
			DataLane:      "drop",
			ControlLane:   "non_blocking_priority",
			TelemetryLane: "best_effort_drop",
		},
		DefaultingSource: "explicit_edge_config",
	}
	plan.FlowControl.Watermarks = controlplane.FlowWatermarks{
		DataLane:      controlplane.WatermarkThreshold{High: 1, Low: 0},
		ControlLane:   controlplane.WatermarkThreshold{High: 1, Low: 0},
		TelemetryLane: controlplane.WatermarkThreshold{High: 1, Low: 0},
	}

	cfg, err := lanes.ConfigFromResolvedTurnPlan("sess-f5-sync-coupled-full", "turn-f5-sync-coupled-full", plan)
	if err != nil {
		t.Fatalf("config from resolved plan: %v", err)
	}
	scheduler, err := lanes.NewPriorityScheduler(localadmission.Evaluator{}, cfg)
	if err != nil {
		t.Fatalf("new priority scheduler: %v", err)
	}

	if _, err := scheduler.Enqueue(lanes.EnqueueInput{
		ItemID:            "telemetry-pressure-1",
		Lane:              eventabi.LaneTelemetry,
		EventID:           "evt-f5-telemetry-pressure-1",
		TransportSequence: 23,
		RuntimeSequence:   23,
	}); err != nil {
		t.Fatalf("enqueue telemetry pressure item: %v", err)
	}
	if _, err := scheduler.Enqueue(lanes.EnqueueInput{
		ItemID:            "telemetry-pressure-2",
		Lane:              eventabi.LaneTelemetry,
		EventID:           "evt-f5-telemetry-pressure-2",
		TransportSequence: 24,
		RuntimeSequence:   24,
	}); err != nil {
		t.Fatalf("enqueue telemetry overflow item: %v", err)
	}

	discontinuityEnqueue, err := scheduler.Enqueue(lanes.EnqueueInput{
		ItemID:            "sync-discontinuity",
		Lane:              eventabi.LaneControl,
		EventID:           discontinuity.EventID,
		TransportSequence: 25,
		RuntimeSequence:   26,
	})
	if err != nil || !discontinuityEnqueue.Accepted {
		t.Fatalf("expected sync discontinuity control enqueue accepted under telemetry pressure, got result=%+v err=%v", discontinuityEnqueue, err)
	}

	dispatch, ok, err := scheduler.NextDispatch()
	if err != nil {
		t.Fatalf("next dispatch failed: %v", err)
	}
	if !ok || dispatch.Item.ItemID != "sync-discontinuity" {
		t.Fatalf("expected sync control item to preempt telemetry backlog, got %+v ok=%v", dispatch, ok)
	}
}

func TestF6TransportDisconnectStallFull(t *testing.T) {
	t.Parallel()

	disconnected, err := runtimetransport.BuildConnectionSignal(runtimetransport.ConnectionSignalInput{
		SessionID:            "sess-f6-full",
		TurnID:               "turn-f6-full",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f6-disconnect",
		Signal:               "disconnected",
		TransportSequence:    30,
		RuntimeSequence:      31,
		AuthorityEpoch:       6,
		RuntimeTimestampMS:   400,
		WallClockTimestampMS: 400,
		Reason:               "transport_disconnect_or_stall",
	})
	if err != nil {
		t.Fatalf("unexpected disconnected signal error: %v", err)
	}
	if disconnected.Signal != "disconnected" {
		t.Fatalf("expected disconnected signal, got %+v", disconnected)
	}

	arbiter := turnarbiter.New()
	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:                  "sess-f6-full",
		TurnID:                     "turn-f6-full",
		EventID:                    "evt-f6-active",
		PipelineVersion:            "pipeline-v1",
		TransportSequence:          30,
		RuntimeSequence:            31,
		RuntimeTimestampMS:         401,
		WallClockTimestampMS:       401,
		AuthorityEpoch:             6,
		TransportDisconnectOrStall: true,
	})
	if err != nil {
		t.Fatalf("unexpected transport teardown error: %v", err)
	}
	if len(active.Events) < 4 || active.Events[2].Name != "abort" || active.Events[2].Reason != "transport_disconnect_or_stall" || active.Events[3].Name != "close" {
		t.Fatalf("expected transport terminalization abort(transport_disconnect_or_stall)->close, got %+v", active.Events)
	}
}

func TestF8RegionFailoverFull(t *testing.T) {
	t.Parallel()

	signals, err := guard.BuildMigrationSignals(guard.MigrationInput{
		SessionID:             "sess-f8-full",
		TurnID:                "turn-f8-full",
		PipelineVersion:       "pipeline-v1",
		EventIDBase:           "evt-f8",
		TransportSequence:     40,
		RuntimeSequence:       41,
		AuthorityEpoch:        7,
		RuntimeTimestampMS:    500,
		WallClockTimestampMS:  500,
		IncludeSessionHandoff: true,
	})
	if err != nil {
		t.Fatalf("unexpected migration signal generation error: %v", err)
	}
	if len(signals) != 4 {
		t.Fatalf("expected lease/migration/handoff signal pack, got %+v", signals)
	}

	outcome, staleSignal, err := guard.RejectOldPlacementOutput("sess-f8-full", "turn-f8-full", "evt-f8-old", 501, 501, 7)
	if err != nil {
		t.Fatalf("unexpected stale old-writer rejection error: %v", err)
	}
	if outcome.OutcomeKind != controlplane.OutcomeStaleEpochReject || staleSignal.Signal != "stale_epoch_reject" {
		t.Fatalf("expected stale epoch rejection artifacts, got outcome=%+v signal=%+v", outcome, staleSignal)
	}

	arbiter := turnarbiter.New()
	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-f8-full",
		TurnID:               "turn-f8-full",
		EventID:              "evt-f8-revoke",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   502,
		WallClockTimestampMS: 502,
		AuthorityEpoch:       7,
		AuthorityRevoked:     true,
	})
	if err != nil {
		t.Fatalf("unexpected authority-revoke terminalization error: %v", err)
	}
	if len(active.Events) != 3 || active.Events[0].Name != "deauthorized_drain" || active.Events[1].Reason != "authority_loss" || active.Events[2].Name != "close" {
		t.Fatalf("expected deauthorized_drain->abort(authority_loss)->close, got %+v", active.Events)
	}
}

func TestF8StaleAuthorityOutputRejectedByFenceFull(t *testing.T) {
	t.Parallel()

	stateSvc := runtimestate.NewService()
	if err := stateSvc.ValidateAuthority("sess-f8-stale-output-1", 11); err != nil {
		t.Fatalf("seed authority epoch: %v", err)
	}
	fence := runtimetransport.NewOutputFenceWithDependencies(nil, stateSvc)

	staleOutput, err := fence.EvaluateOutput(runtimetransport.OutputAttempt{
		SessionID:            "sess-f8-stale-output-1",
		TurnID:               "turn-f8-stale-output-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f8-stale-output-1",
		TransportSequence:    50,
		RuntimeSequence:      50,
		AuthorityEpoch:       10,
		RuntimeTimestampMS:   600,
		WallClockTimestampMS: 600,
	})
	if err != nil {
		t.Fatalf("unexpected stale output evaluation error: %v", err)
	}
	if staleOutput.Accepted || staleOutput.Signal.Signal != "stale_epoch_reject" {
		t.Fatalf("expected stale output rejection, got %+v", staleOutput)
	}
}

func TestF8SessionHotResetReattachRespectsEpoch(t *testing.T) {
	t.Parallel()

	stateSvc := runtimestate.NewService()
	if err := stateSvc.PutSessionHot("sess-f8-state-1", "route", 21, "runtime-a"); err != nil {
		t.Fatalf("seed session-hot state: %v", err)
	}

	if err := stateSvc.ResetSessionHot("sess-f8-state-1", 20); err == nil {
		t.Fatalf("expected stale reset to be rejected")
	}

	if err := stateSvc.ResetSessionHot("sess-f8-state-1", 22); err != nil {
		t.Fatalf("reset session-hot with rotated epoch: %v", err)
	}
	if _, ok := stateSvc.GetSessionHot("sess-f8-state-1", "route"); ok {
		t.Fatalf("expected reset to clear prior session-hot route state")
	}

	if err := stateSvc.PutSessionHot("sess-f8-state-1", "route", 22, "runtime-b"); err != nil {
		t.Fatalf("reattach session-hot state: %v", err)
	}
	route, ok := stateSvc.GetSessionHot("sess-f8-state-1", "route")
	if !ok || route != "runtime-b" {
		t.Fatalf("expected reattached route state to be present, got route=%v ok=%v", route, ok)
	}
}

func TestF5SyncEngineDropWithDiscontinuityFull(t *testing.T) {
	t.Parallel()

	engine := runtimesync.NewEngine()
	result, err := engine.Evaluate(runtimesync.Input{
		SessionID:            "sess-f5-sync-engine-1",
		TurnID:               "turn-f5-sync-engine-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f5-sync-engine-1",
		TransportSequence:    60,
		RuntimeSequence:      61,
		AuthorityEpoch:       8,
		RuntimeTimestampMS:   700,
		WallClockTimestampMS: 700,
		EdgeID:               "edge-av",
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       100,
		DropRangeEnd:         120,
		SyncDomain:           "av-sync",
		DiscontinuityID:      "disc-sync-engine-1",
		Policy:               runtimesync.PolicyDropWithDiscontinuity,
	})
	if err != nil {
		t.Fatalf("unexpected sync engine error: %v", err)
	}
	if len(result.Signals) != 2 || result.Signals[0].Signal != "drop_notice" || result.Signals[1].Signal != "discontinuity" {
		t.Fatalf("expected drop_notice then discontinuity, got %+v", result.Signals)
	}
}

func TestF2ExternalNodeTimeoutAndCancelFull(t *testing.T) {
	t.Parallel()

	adapter := &failoverExternalNodeAdapter{
		invokeFn: func(ctx context.Context, _ runtimeexternalnode.Request) (runtimeexternalnode.Response, error) {
			<-ctx.Done()
			return runtimeexternalnode.Response{}, ctx.Err()
		},
	}
	runtime, err := runtimeexternalnode.NewRuntime(adapter, runtimeexternalnode.Config{
		Timeout:     20 * time.Millisecond,
		MaxCPUUnits: 2,
		MaxMemoryMB: 512,
	})
	if err != nil {
		t.Fatalf("new external runtime: %v", err)
	}

	_, err = runtime.Invoke(context.Background(), runtimeexternalnode.Request{
		SessionID:      "sess-f2-ext-1",
		TurnID:         "turn-f2-ext-1",
		NodeID:         "external-node-a",
		InvocationID:   "invoke-f2-ext-1",
		AuthorityEpoch: 9,
		CPUUnits:       1,
		MemoryMB:       128,
	})
	if err == nil {
		t.Fatalf("expected external invocation timeout")
	}
	if !errors.Is(err, runtimeexternalnode.ErrExecutionTimeout) {
		t.Fatalf("expected ErrExecutionTimeout, got %v", err)
	}

	adapter.cancelFn = func(_ context.Context, invocationID string, reason string) error {
		if invocationID != "invoke-f2-ext-1" || reason != "cancelled" {
			t.Fatalf("unexpected cancel invocation id/reason: %s %s", invocationID, reason)
		}
		return nil
	}
	if err := runtime.Cancel(context.Background(), "invoke-f2-ext-1", "cancelled"); err != nil {
		t.Fatalf("cancel propagation: %v", err)
	}
}

type failoverExternalNodeAdapter struct {
	invokeFn func(ctx context.Context, req runtimeexternalnode.Request) (runtimeexternalnode.Response, error)
	cancelFn func(ctx context.Context, invocationID string, reason string) error
}

func (a *failoverExternalNodeAdapter) Invoke(ctx context.Context, req runtimeexternalnode.Request) (runtimeexternalnode.Response, error) {
	if a.invokeFn == nil {
		return runtimeexternalnode.Response{InvocationID: req.InvocationID}, nil
	}
	return a.invokeFn(ctx, req)
}

func (a *failoverExternalNodeAdapter) Cancel(ctx context.Context, invocationID string, reason string) error {
	if a.cancelFn == nil {
		return nil
	}
	return a.cancelFn(ctx, invocationID, reason)
}

func containsControlSignal(signals []eventabi.ControlSignal, signal string) bool {
	for _, candidate := range signals {
		if candidate.Signal == signal {
			return true
		}
	}
	return false
}

func resolvedPlanForFailoverSchedulerTests(t *testing.T, turnID string, authorityEpoch int64) controlplane.ResolvedTurnPlan {
	t.Helper()

	plan, err := planresolver.Resolver{}.Resolve(planresolver.Input{
		TurnID:             turnID,
		PipelineVersion:    "pipeline-v1",
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
		AllowedAdaptiveActions: []string{"retry"},
	})
	if err != nil {
		t.Fatalf("resolve plan for failover scheduler tests: %v", err)
	}
	return plan
}
