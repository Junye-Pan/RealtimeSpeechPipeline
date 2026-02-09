package replay

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/observability"
)

// TraceArtifact captures replay-comparable evidence dimensions.
type TraceArtifact struct {
	PlanHash           string
	Decision           controlplane.DecisionOutcome
	OrderingMarker     string
	AuthorityEpoch     int64
	RuntimeTimestampMS int64
}

// CompareConfig allows deterministic tolerance configuration.
type CompareConfig struct {
	TimingToleranceMS int64
}

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

// CompareTraceArtifacts compares plan, outcome, ordering, authority, and timing dimensions.
func CompareTraceArtifacts(baseline, replay []TraceArtifact, cfg CompareConfig) []observability.ReplayDivergence {
	divergences := make([]observability.ReplayDivergence, 0)

	if len(baseline) != len(replay) {
		divergences = append(divergences, observability.ReplayDivergence{
			Class:   observability.OutcomeDivergence,
			Scope:   "trace",
			Message: fmt.Sprintf("trace length mismatch: baseline=%d replay=%d", len(baseline), len(replay)),
		})
	}

	limit := len(baseline)
	if len(replay) < limit {
		limit = len(replay)
	}

	for i := 0; i < limit; i++ {
		scope := divergenceScope(baseline[i].Decision)

		if baseline[i].PlanHash != replay[i].PlanHash {
			divergences = append(divergences, observability.ReplayDivergence{
				Class:   observability.PlanDivergence,
				Scope:   scope,
				Message: fmt.Sprintf("plan hash mismatch at index=%d baseline=%s replay=%s", i, baseline[i].PlanHash, replay[i].PlanHash),
			})
		}

		if !equivalentDecisionOutcome(baseline[i].Decision, replay[i].Decision) {
			divergences = append(divergences, observability.ReplayDivergence{
				Class:   observability.OutcomeDivergence,
				Scope:   scope,
				Message: fmt.Sprintf("decision_outcome mismatch at index=%d baseline_event=%s replay_event=%s", i, baseline[i].Decision.EventID, replay[i].Decision.EventID),
			})
		}

		if baseline[i].OrderingMarker != replay[i].OrderingMarker {
			divergences = append(divergences, observability.ReplayDivergence{
				Class:   observability.OrderingDivergence,
				Scope:   scope,
				Message: fmt.Sprintf("ordering marker mismatch at index=%d baseline=%s replay=%s", i, baseline[i].OrderingMarker, replay[i].OrderingMarker),
			})
		}

		if baseline[i].AuthorityEpoch != replay[i].AuthorityEpoch {
			divergences = append(divergences, observability.ReplayDivergence{
				Class:   observability.AuthorityDivergence,
				Scope:   scope,
				Message: fmt.Sprintf("authority epoch mismatch at index=%d baseline=%d replay=%d", i, baseline[i].AuthorityEpoch, replay[i].AuthorityEpoch),
			})
		}

		tolerance := cfg.TimingToleranceMS
		if tolerance < 0 {
			tolerance = 0
		}
		if absDiff(baseline[i].RuntimeTimestampMS, replay[i].RuntimeTimestampMS) > tolerance {
			divergences = append(divergences, observability.ReplayDivergence{
				Class:   observability.TimingDivergence,
				Scope:   scope,
				Message: fmt.Sprintf("timing mismatch at index=%d baseline=%d replay=%d tolerance=%d", i, baseline[i].RuntimeTimestampMS, replay[i].RuntimeTimestampMS, tolerance),
			})
		}
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

func divergenceScope(out controlplane.DecisionOutcome) string {
	if out.TurnID != "" {
		return "turn:" + out.TurnID
	}
	return "session:" + out.SessionID
}

func absDiff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}
