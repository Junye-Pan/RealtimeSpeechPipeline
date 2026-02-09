package failover_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/buffering"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/guard"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/nodehost"
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
