package regression

import (
	"fmt"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

// ExpectedDivergence declares fixture-approved divergences by class and scope.
type ExpectedDivergence struct {
	Class    obs.DivergenceClass `json:"class"`
	Scope    string              `json:"scope"`
	Approved bool                `json:"approved,omitempty"`
}

// DivergencePolicy defines fail criteria for replay divergences.
type DivergencePolicy struct {
	TimingToleranceMS int64                `json:"timing_tolerance_ms"`
	Expected          []ExpectedDivergence `json:"expected,omitempty"`
}

// DivergenceEvaluation returns policy outcomes for replay divergences.
type DivergenceEvaluation struct {
	Failing         []obs.ReplayDivergence `json:"failing"`
	Unexplained     []obs.ReplayDivergence `json:"unexplained"`
	MissingExpected []ExpectedDivergence   `json:"missing_expected,omitempty"`
}

// EvaluateDivergences enforces CI replay fail conditions.
func EvaluateDivergences(divergences []obs.ReplayDivergence, policy DivergencePolicy) DivergenceEvaluation {
	evaluation := DivergenceEvaluation{}
	if policy.TimingToleranceMS < 0 {
		policy.TimingToleranceMS = 0
	}

	expected := make(map[string]ExpectedDivergence, len(policy.Expected))
	for _, item := range policy.Expected {
		expected[key(item.Class, item.Scope)] = item
	}

	for _, entry := range divergences {
		entryCopy := entry
		expectedMatch, hasExpected := expected[key(entry.Class, entry.Scope)]
		if hasExpected {
			entryCopy.Expected = true
			delete(expected, key(entry.Class, entry.Scope))
		}

		switch entry.Class {
		case obs.PlanDivergence, obs.OutcomeDivergence:
			if !hasExpected {
				evaluation.Unexplained = append(evaluation.Unexplained, entryCopy)
				evaluation.Failing = append(evaluation.Failing, entryCopy)
			}
		case obs.AuthorityDivergence:
			evaluation.Failing = append(evaluation.Failing, entryCopy)
		case obs.OrderingDivergence:
			if !hasExpected || !expectedMatch.Approved {
				evaluation.Failing = append(evaluation.Failing, entryCopy)
				if !hasExpected {
					evaluation.Unexplained = append(evaluation.Unexplained, entryCopy)
				}
			}
		case obs.TimingDivergence:
			if exceedsTimingTolerance(entryCopy, policy.TimingToleranceMS) {
				evaluation.Failing = append(evaluation.Failing, entryCopy)
			}
		default:
			evaluation.Failing = append(evaluation.Failing, entryCopy)
		}
	}

	for _, missing := range expected {
		evaluation.MissingExpected = append(evaluation.MissingExpected, missing)
		evaluation.Failing = append(evaluation.Failing, obs.ReplayDivergence{
			Class:   missing.Class,
			Scope:   missing.Scope,
			Message: fmt.Sprintf("expected divergence annotation missing in observed output: class=%s scope=%s", missing.Class, missing.Scope),
		})
	}

	return evaluation
}

func key(class obs.DivergenceClass, scope string) string {
	return string(class) + "|" + scope
}

func exceedsTimingTolerance(divergence obs.ReplayDivergence, tolerance int64) bool {
	if divergence.DiffMS == nil {
		return true
	}
	return *divergence.DiffMS > tolerance
}
