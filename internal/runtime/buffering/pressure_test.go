package buffering

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
)

func TestHandleEdgePressureF4SignalsAndShed(t *testing.T) {
	t.Parallel()

	result, err := HandleEdgePressure(localadmission.Evaluator{}, PressureInput{
		SessionID:            "sess-f4-1",
		TurnID:               "turn-f4-1",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-a",
		EventID:              "evt-f4-1",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       50,
		DropRangeEnd:         60,
		HighWatermark:        true,
		Shed:                 true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Signals) < 3 {
		t.Fatalf("expected watermark, flow_xoff, and drop_notice; got %+v", result.Signals)
	}
	if result.ShedOutcome == nil || result.ShedOutcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected scheduling-point shed outcome, got %+v", result.ShedOutcome)
	}
}

func TestHandleSyncLossF5Discontinuity(t *testing.T) {
	t.Parallel()

	sig, err := HandleSyncLoss(SyncLossInput{
		SessionID:            "sess-f5-1",
		TurnID:               "turn-f5-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f5-1",
		TransportSequence:    20,
		RuntimeSequence:      21,
		AuthorityEpoch:       4,
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		SyncDomain:           "av-sync",
		DiscontinuityID:      "disc-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig.Signal != "discontinuity" || sig.EmittedBy != "RK-15" {
		t.Fatalf("expected RK-15 discontinuity signal, got %+v", sig)
	}
}

func TestHandleSyncLossF5AtomicDrop(t *testing.T) {
	t.Parallel()

	sig, err := HandleSyncLoss(SyncLossInput{
		SessionID:            "sess-f5-2",
		TurnID:               "turn-f5-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f5-2",
		TransportSequence:    20,
		RuntimeSequence:      21,
		AuthorityEpoch:       4,
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		UseAtomicDrop:        true,
		EdgeID:               "edge-av",
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       70,
		DropRangeEnd:         80,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig.Signal != "drop_notice" {
		t.Fatalf("expected drop_notice for atomic-drop policy, got %+v", sig)
	}
}
