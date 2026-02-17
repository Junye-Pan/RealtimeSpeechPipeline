package buffering

import (
	"fmt"
	"sort"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	telemetrycontext "github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry/context"
)

// MergeSource represents one source event participating in deterministic coalescing.
type MergeSource struct {
	EventID         string
	RuntimeSequence int64
	SourceRef       string
}

// MergeInput defines deterministic merge/coalesce context.
type MergeInput struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EdgeID               string
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	MergeGroupID         string
	Sources              []MergeSource
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
	if in.SessionID != "" && in.PipelineVersion != "" {
		correlation, err := telemetrycontext.Resolve(telemetrycontext.ResolveInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			EventID:              sources[0].EventID,
			PipelineVersion:      in.PipelineVersion,
			EdgeID:               in.EdgeID,
			AuthorityEpoch:       mergeNonNegative(in.AuthorityEpoch),
			Lane:                 eventabi.LaneTelemetry,
			EmittedBy:            "OR-01",
			RuntimeTimestampMS:   mergeNonNegative(in.RuntimeTimestampMS),
			WallClockTimestampMS: mergeNonNegative(in.WallClockTimestampMS),
		})
		if err == nil {
			telemetry.DefaultEmitter().EmitMetric(
				telemetry.MetricEdgeMergesTotal,
				1,
				"count",
				map[string]string{
					"edge_id":        in.EdgeID,
					"merge_group_id": in.MergeGroupID,
					"source_count":   fmt.Sprintf("%d", len(sources)),
				},
				correlation,
			)
		}
	}

	return MergeResult{
		MergeGroupID:   in.MergeGroupID,
		SourceEventIDs: ids,
		SourceSpan:     span,
	}, nil
}

func mergeNonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
