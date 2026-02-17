package sync

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestEngineAtomicDropPolicy(t *testing.T) {
	t.Parallel()

	engine := NewEngine()
	result, err := engine.Evaluate(Input{
		SessionID:            "sess-sync-1",
		TurnID:               "turn-sync-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-sync-1",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		EdgeID:               "edge-av",
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       20,
		DropRangeEnd:         30,
		Policy:               PolicyAtomicDrop,
	})
	if err != nil {
		t.Fatalf("evaluate atomic drop policy: %v", err)
	}
	if len(result.Signals) != 1 || result.Signals[0].Signal != "drop_notice" {
		t.Fatalf("expected one drop_notice signal, got %+v", result.Signals)
	}
}

func TestEngineDropWithDiscontinuityPolicy(t *testing.T) {
	t.Parallel()

	engine := NewEngine()
	result, err := engine.Evaluate(Input{
		SessionID:            "sess-sync-2",
		TurnID:               "turn-sync-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-sync-2",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		EdgeID:               "edge-av",
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       20,
		DropRangeEnd:         30,
		SyncDomain:           "av-sync",
		DiscontinuityID:      "disc-1",
		Policy:               PolicyDropWithDiscontinuity,
	})
	if err != nil {
		t.Fatalf("evaluate drop_with_discontinuity policy: %v", err)
	}
	if len(result.Signals) != 2 {
		t.Fatalf("expected drop_notice + discontinuity signals, got %+v", result.Signals)
	}
	if result.Signals[0].Signal != "drop_notice" || result.Signals[1].Signal != "discontinuity" {
		t.Fatalf("unexpected signal order/content: %+v", result.Signals)
	}
}

func TestEngineRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	engine := NewEngine()
	_, err := engine.Evaluate(Input{
		SessionID:       "sess-sync-3",
		PipelineVersion: "pipeline-v1",
		EventID:         "evt-sync-3",
		Policy:          PolicyDropWithDiscontinuity,
	})
	if err == nil {
		t.Fatalf("expected missing required fields to fail")
	}
}
