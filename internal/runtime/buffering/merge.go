package buffering

import (
	"fmt"
	"sort"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// MergeSource represents one source event participating in deterministic coalescing.
type MergeSource struct {
	EventID         string
	RuntimeSequence int64
	SourceRef       string
}

// MergeInput defines deterministic merge/coalesce context.
type MergeInput struct {
	MergeGroupID string
	Sources      []MergeSource
}

// MergeResult carries merge lineage metadata required for replay explainability.
type MergeResult struct {
	MergeGroupID   string
	SourceEventIDs []string
	SourceSpan     eventabi.SeqRange
}

// MergeCoalescedEvents deterministically merges events and emits lineage metadata.
func MergeCoalescedEvents(in MergeInput) (MergeResult, error) {
	if in.MergeGroupID == "" {
		return MergeResult{}, fmt.Errorf("merge_group_id is required")
	}
	if len(in.Sources) < 2 {
		return MergeResult{}, fmt.Errorf("at least two source events are required")
	}

	sources := append([]MergeSource(nil), in.Sources...)
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].RuntimeSequence == sources[j].RuntimeSequence {
			return sources[i].EventID < sources[j].EventID
		}
		return sources[i].RuntimeSequence < sources[j].RuntimeSequence
	})

	ids := make([]string, 0, len(sources))
	for _, src := range sources {
		if src.EventID == "" {
			return MergeResult{}, fmt.Errorf("source event id is required")
		}
		if src.RuntimeSequence < 0 {
			return MergeResult{}, fmt.Errorf("source runtime sequence must be >= 0")
		}
		ids = append(ids, src.EventID)
	}

	span := eventabi.SeqRange{Start: sources[0].RuntimeSequence, End: sources[len(sources)-1].RuntimeSequence}
	if err := span.Validate(); err != nil {
		return MergeResult{}, err
	}

	return MergeResult{
		MergeGroupID:   in.MergeGroupID,
		SourceEventIDs: ids,
		SourceSpan:     span,
	}, nil
}
