package buffering

import (
	"reflect"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
)

func TestMergeCoalescedEventsML002DeterministicLineage(t *testing.T) {
	t.Parallel()

	input := MergeInput{
		MergeGroupID: "merge-group-1",
		Sources: []MergeSource{
			{EventID: "evt-c", RuntimeSequence: 30},
			{EventID: "evt-a", RuntimeSequence: 10},
			{EventID: "evt-b", RuntimeSequence: 20},
		},
	}
	first, err := MergeCoalescedEvents(input)
	if err != nil {
		t.Fatalf("unexpected merge error: %v", err)
	}
	second, err := MergeCoalescedEvents(input)
	if err != nil {
		t.Fatalf("unexpected repeated merge error: %v", err)
	}

	if first.MergeGroupID != "merge-group-1" {
		t.Fatalf("unexpected merge group id: %s", first.MergeGroupID)
	}
	if len(first.SourceEventIDs) != 3 || first.SourceEventIDs[0] != "evt-a" || first.SourceEventIDs[1] != "evt-b" || first.SourceEventIDs[2] != "evt-c" {
		t.Fatalf("expected deterministic sorted source IDs, got %+v", first.SourceEventIDs)
	}
	if first.SourceSpan.Start != 10 || first.SourceSpan.End != 30 {
		t.Fatalf("expected source span [10,30], got %+v", first.SourceSpan)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic merge result, got first=%+v second=%+v", first, second)
	}
}

func TestMergeCoalescedEventsEmitsEdgeMergeMetric(t *testing.T) {
	sink := telemetry.NewMemorySink()
	pipeline := telemetry.NewPipeline(sink, telemetry.Config{QueueCapacity: 32})
	previous := telemetry.DefaultEmitter()
	telemetry.SetDefaultEmitter(pipeline)
	t.Cleanup(func() {
		telemetry.SetDefaultEmitter(previous)
		_ = pipeline.Close()
	})

	_, err := MergeCoalescedEvents(MergeInput{
		SessionID:            "sess-merge-telemetry-1",
		TurnID:               "turn-merge-telemetry-1",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-merge-1",
		AuthorityEpoch:       4,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		MergeGroupID:         "merge-group-telemetry-1",
		Sources: []MergeSource{
			{EventID: "evt-b", RuntimeSequence: 20},
			{EventID: "evt-a", RuntimeSequence: 10},
		},
	})
	if err != nil {
		t.Fatalf("merge for telemetry metric: %v", err)
	}
	if err := pipeline.Close(); err != nil {
		t.Fatalf("close telemetry pipeline: %v", err)
	}

	metricFound := false
	for _, event := range sink.Events() {
		if event.Correlation.SessionID != "sess-merge-telemetry-1" {
			continue
		}
		if event.Correlation.EdgeID != "edge-merge-1" {
			t.Fatalf("expected edge correlation linkage, got %+v", event.Correlation)
		}
		if event.Kind != telemetry.EventKindMetric || event.Metric == nil {
			continue
		}
		if event.Metric.Name == telemetry.MetricEdgeMergesTotal {
			if event.Metric.Attributes["edge_id"] != "edge-merge-1" {
				t.Fatalf("expected edge_id metric linkage, got %+v", event.Metric.Attributes)
			}
			metricFound = true
		}
	}
	if !metricFound {
		t.Fatalf("expected edge merge telemetry metric")
	}
}
