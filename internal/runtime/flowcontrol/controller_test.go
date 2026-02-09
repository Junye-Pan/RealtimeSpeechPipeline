package flowcontrol

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestEvaluateSignalMode(t *testing.T) {
	t.Parallel()

	controller := NewController()
	signals, err := controller.Evaluate(Input{
		SessionID:            "sess-flow-1",
		TurnID:               "turn-flow-1",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-a",
		EventID:              "evt-flow-1",
		TargetLane:           eventabi.LaneData,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
		HighWatermark:        true,
		EmitRecovery:         true,
		Mode:                 ModeSignal,
	})
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if len(signals) != 2 {
		t.Fatalf("expected xoff + xon, got %+v", signals)
	}
	if signals[0].Signal != "flow_xoff" || signals[1].Signal != "flow_xon" {
		t.Fatalf("expected flow_xoff/flow_xon ordering, got %+v", signals)
	}
}

func TestEvaluateCreditMode(t *testing.T) {
	t.Parallel()

	controller := NewController()
	signals, err := controller.Evaluate(Input{
		SessionID:            "sess-flow-2",
		TurnID:               "turn-flow-2",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-b",
		EventID:              "evt-flow-2",
		TargetLane:           eventabi.LaneData,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
		EmitRecovery:         true,
		Mode:                 ModeCredit,
		CreditAmount:         3,
	})
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected credit_grant, got %+v", signals)
	}
	if signals[0].Signal != "credit_grant" {
		t.Fatalf("expected credit_grant signal, got %+v", signals[0])
	}
	if signals[0].Amount == nil || *signals[0].Amount != 3 {
		t.Fatalf("expected credit amount 3, got %+v", signals[0].Amount)
	}
}
