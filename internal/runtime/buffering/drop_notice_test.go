package buffering

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestML001DropNoticeDeterministicUnderPressure(t *testing.T) {
	t.Parallel()

	first, err := BuildDropNotice(DropNoticeInput{
		SessionID:            "sess-ml-1",
		TurnID:               "turn-ml-1",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-stt->llm",
		EventID:              "evt-drop-1",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   1000,
		WallClockTimestampMS: 1000,
		TargetLane:           eventabi.LaneData,
		RangeStart:           100,
		RangeEnd:             120,
	})
	if err != nil {
		t.Fatalf("unexpected error building first drop notice: %v", err)
	}

	second, err := BuildDropNotice(DropNoticeInput{
		SessionID:            "sess-ml-1",
		TurnID:               "turn-ml-1",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-stt->llm",
		EventID:              "evt-drop-1",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   1000,
		WallClockTimestampMS: 1000,
		TargetLane:           eventabi.LaneData,
		RangeStart:           100,
		RangeEnd:             120,
	})
	if err != nil {
		t.Fatalf("unexpected error building second drop notice: %v", err)
	}

	if first.Signal != "drop_notice" {
		t.Fatalf("expected drop_notice signal, got %s", first.Signal)
	}
	if first.Reason == "" || second.Reason == "" || first.Reason != second.Reason {
		t.Fatalf("expected deterministic non-empty reason, got %q and %q", first.Reason, second.Reason)
	}
	if first.SeqRange == nil || first.SeqRange.Start != 100 || first.SeqRange.End != 120 {
		t.Fatalf("expected seq_range [100,120], got %+v", first.SeqRange)
	}
	if first.SeqRange == nil || second.SeqRange == nil || *first.SeqRange != *second.SeqRange {
		t.Fatalf("expected deterministic seq_range on repeated inputs")
	}
}

func TestDropNoticeRejectsInvalidRange(t *testing.T) {
	t.Parallel()

	_, err := BuildDropNotice(DropNoticeInput{
		SessionID:            "sess-ml-2",
		TurnID:               "turn-ml-2",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-llm->tts",
		EventID:              "evt-drop-2",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       4,
		RuntimeTimestampMS:   2000,
		WallClockTimestampMS: 2000,
		TargetLane:           eventabi.LaneData,
		RangeStart:           50,
		RangeEnd:             40,
	})
	if err == nil {
		t.Fatalf("expected invalid seq_range to fail")
	}
}
