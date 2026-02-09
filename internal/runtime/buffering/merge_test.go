package buffering

import "testing"
import "reflect"

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
