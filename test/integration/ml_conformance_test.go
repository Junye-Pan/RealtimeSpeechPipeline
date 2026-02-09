package integration_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/buffering"
)

func TestML002DeterministicMergeCoalesceLineage(t *testing.T) {
	t.Parallel()

	merged, err := buffering.MergeCoalescedEvents(buffering.MergeInput{
		MergeGroupID: "merge-ml-002",
		Sources: []buffering.MergeSource{
			{EventID: "evt-3", RuntimeSequence: 30},
			{EventID: "evt-1", RuntimeSequence: 10},
			{EventID: "evt-2", RuntimeSequence: 20},
		},
	})
	if err != nil {
		t.Fatalf("unexpected merge error: %v", err)
	}
	if merged.MergeGroupID != "merge-ml-002" {
		t.Fatalf("unexpected merge group id: %s", merged.MergeGroupID)
	}
	if len(merged.SourceEventIDs) != 3 || merged.SourceEventIDs[0] != "evt-1" || merged.SourceEventIDs[2] != "evt-3" {
		t.Fatalf("expected deterministic source lineage ordering, got %+v", merged.SourceEventIDs)
	}
	if merged.SourceSpan.Start != 10 || merged.SourceSpan.End != 30 {
		t.Fatalf("expected deterministic source span [10,30], got %+v", merged.SourceSpan)
	}
}

func TestML003ReplayExplainabilityDropVsAbsence(t *testing.T) {
	t.Parallel()

	baseline := []replay.LineageRecord{
		{EventID: "evt-drop", Dropped: true, MergeGroupID: ""},
		{EventID: "evt-merge", Dropped: false, MergeGroupID: "merge-1"},
	}
	replayed := []replay.LineageRecord{
		{EventID: "evt-drop", Dropped: true, MergeGroupID: ""},
		{EventID: "evt-merge", Dropped: false, MergeGroupID: "merge-1"},
	}
	divergences := replay.CompareLineageRecords(baseline, replayed)
	if len(divergences) != 0 {
		t.Fatalf("expected replay to explain dropped/merged outputs, got %+v", divergences)
	}

	missing := []replay.LineageRecord{
		{EventID: "evt-drop", Dropped: true, MergeGroupID: ""},
	}
	divergences = replay.CompareLineageRecords(baseline, missing)
	if len(divergences) != 1 || divergences[0].Class != obs.OutcomeDivergence {
		t.Fatalf("expected outcome divergence for unexplained upstream absence, got %+v", divergences)
	}
}

func TestML004SyncDiscontinuityDeterministicResetPath(t *testing.T) {
	t.Parallel()

	first, err := buffering.HandleSyncLoss(buffering.SyncLossInput{
		SessionID:            "sess-ml-004",
		TurnID:               "turn-ml-004",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-ml-004",
		TransportSequence:    50,
		RuntimeSequence:      60,
		AuthorityEpoch:       8,
		RuntimeTimestampMS:   1000,
		WallClockTimestampMS: 1000,
		SyncDomain:           "av-sync",
		DiscontinuityID:      "disc-ml-004",
	})
	if err != nil {
		t.Fatalf("unexpected discontinuity error: %v", err)
	}
	second, err := buffering.HandleSyncLoss(buffering.SyncLossInput{
		SessionID:            "sess-ml-004",
		TurnID:               "turn-ml-004",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-ml-004",
		TransportSequence:    50,
		RuntimeSequence:      60,
		AuthorityEpoch:       8,
		RuntimeTimestampMS:   1000,
		WallClockTimestampMS: 1000,
		SyncDomain:           "av-sync",
		DiscontinuityID:      "disc-ml-004",
	})
	if err != nil {
		t.Fatalf("unexpected repeated discontinuity error: %v", err)
	}
	if first.Signal != "discontinuity" || first.EmittedBy != "RK-15" || first.SyncDomain != "av-sync" {
		t.Fatalf("expected deterministic discontinuity marker, got %+v", first)
	}
	if second.Signal != first.Signal || second.DiscontinuityID != first.DiscontinuityID || second.TargetLane != eventabi.Lane("") {
		t.Fatalf("expected deterministic downstream reset path markers, got first=%+v second=%+v", first, second)
	}
}
