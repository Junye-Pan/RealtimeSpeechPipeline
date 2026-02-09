package replay

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/observability"
)

// CompareDecisionOutcomes performs deterministic replay/outcome comparison.
func CompareDecisionOutcomes(baseline, replay []controlplane.DecisionOutcome) []observability.ReplayDivergence {
	divergences := make([]observability.ReplayDivergence, 0)

	if len(baseline) != len(replay) {
		divergences = append(divergences, observability.ReplayDivergence{
			Class:   observability.OutcomeDivergence,
			Scope:   "trace",
			Message: fmt.Sprintf("decision_outcome length mismatch: baseline=%d replay=%d", len(baseline), len(replay)),
		})
	}

	limit := len(baseline)
	if len(replay) < limit {
		limit = len(replay)
	}

	for i := 0; i < limit; i++ {
		if equivalentDecisionOutcome(baseline[i], replay[i]) {
			continue
		}
		scope := "session:" + baseline[i].SessionID
		if baseline[i].TurnID != "" {
			scope = "turn:" + baseline[i].TurnID
		}
		divergences = append(divergences, observability.ReplayDivergence{
			Class:   observability.OutcomeDivergence,
			Scope:   scope,
			Message: fmt.Sprintf("decision_outcome mismatch at index=%d baseline_event=%s replay_event=%s", i, baseline[i].EventID, replay[i].EventID),
		})
	}

	return divergences
}

func equivalentDecisionOutcome(a, b controlplane.DecisionOutcome) bool {
	if a.OutcomeKind != b.OutcomeKind ||
		a.Phase != b.Phase ||
		a.Scope != b.Scope ||
		a.SessionID != b.SessionID ||
		a.TurnID != b.TurnID ||
		a.EventID != b.EventID ||
		a.RuntimeTimestampMS != b.RuntimeTimestampMS ||
		a.WallClockMS != b.WallClockMS ||
		a.EmittedBy != b.EmittedBy ||
		a.Reason != b.Reason {
		return false
	}

	switch {
	case a.AuthorityEpoch == nil && b.AuthorityEpoch == nil:
		return true
	case a.AuthorityEpoch == nil || b.AuthorityEpoch == nil:
		return false
	default:
		return *a.AuthorityEpoch == *b.AuthorityEpoch
	}
}
