package ops

import (
	"fmt"
	"math"
	"sort"
)

// TurnMetrics captures per-turn measurements used for MVP SLO gates.
type TurnMetrics struct {
	TurnID                   string
	Accepted                 bool
	HappyPath                bool
	TurnOpenProposedAtMS     *int64
	TurnOpenAtMS             *int64
	FirstOutputAtMS          *int64
	CancelAcceptedAtMS       *int64
	CancelFenceAppliedAtMS   *int64
	BaselineComplete         bool
	AcceptedStaleEpochOutput bool
	TerminalEvents           []string
}

// MVPSLOThresholds define normative MVP limits.
type MVPSLOThresholds struct {
	TurnOpenDecisionP95MS  int64
	FirstOutputP95MS       int64
	CancelFenceP95MS       int64
	RequiredCompleteness   float64
	MaxStaleAcceptedOutput int
}

// DefaultMVPSLOThresholds returns repository baseline SLO thresholds.
func DefaultMVPSLOThresholds() MVPSLOThresholds {
	return MVPSLOThresholds{
		TurnOpenDecisionP95MS:  120,
		FirstOutputP95MS:       1500,
		CancelFenceP95MS:       150,
		RequiredCompleteness:   1.0,
		MaxStaleAcceptedOutput: 0,
	}
}

// MVPSLOGateReport summarizes SLO gate results.
type MVPSLOGateReport struct {
	Samples                   int      `json:"samples"`
	AcceptedTurns             int      `json:"accepted_turns"`
	HappyPathTurns            int      `json:"happy_path_turns"`
	CancelObservedTurns       int      `json:"cancel_observed_turns"`
	TurnOpenDecisionP95MS     *int64   `json:"turn_open_decision_p95_ms,omitempty"`
	FirstOutputP95MS          *int64   `json:"first_output_p95_ms,omitempty"`
	CancelFenceP95MS          *int64   `json:"cancel_fence_p95_ms,omitempty"`
	BaselineCompletenessRatio float64  `json:"baseline_completeness_ratio"`
	StaleAcceptedOutputs      int      `json:"stale_epoch_accepted_outputs"`
	TerminalCorrectnessRatio  float64  `json:"terminal_correctness_ratio"`
	Violations                []string `json:"violations,omitempty"`
	Passed                    bool     `json:"passed"`
}

// EvaluateMVPSLOGates evaluates MVP SLO gates against runtime samples.
func EvaluateMVPSLOGates(samples []TurnMetrics, thresholds MVPSLOThresholds) MVPSLOGateReport {
	report := MVPSLOGateReport{Samples: len(samples)}
	turnOpenLatencies := make([]int64, 0)
	firstOutputLatencies := make([]int64, 0)
	cancelFenceLatencies := make([]int64, 0)

	completeAccepted := 0
	terminalCorrectAccepted := 0

	for _, sample := range samples {
		if sample.Accepted {
			report.AcceptedTurns++
			if sample.BaselineComplete {
				completeAccepted++
			}
			if hasValidTerminalSequence(sample.TerminalEvents) {
				terminalCorrectAccepted++
			}
			if sample.AcceptedStaleEpochOutput {
				report.StaleAcceptedOutputs++
			}
			if sample.TurnOpenProposedAtMS == nil || sample.TurnOpenAtMS == nil {
				report.Violations = append(report.Violations, fmt.Sprintf("turn %s missing open-latency markers", sample.TurnID))
			} else {
				turnOpenLatency := *sample.TurnOpenAtMS - *sample.TurnOpenProposedAtMS
				if turnOpenLatency < 0 {
					report.Violations = append(report.Violations, fmt.Sprintf("turn %s has negative turn-open latency", sample.TurnID))
				} else {
					turnOpenLatencies = append(turnOpenLatencies, turnOpenLatency)
				}
			}
		}

		if sample.HappyPath {
			report.HappyPathTurns++
			if sample.TurnOpenAtMS == nil || sample.FirstOutputAtMS == nil {
				report.Violations = append(report.Violations, fmt.Sprintf("turn %s missing first-output latency markers", sample.TurnID))
			} else {
				latency := *sample.FirstOutputAtMS - *sample.TurnOpenAtMS
				if latency < 0 {
					report.Violations = append(report.Violations, fmt.Sprintf("turn %s has negative first-output latency", sample.TurnID))
				} else {
					firstOutputLatencies = append(firstOutputLatencies, latency)
				}
			}
		}

		if sample.CancelAcceptedAtMS != nil {
			report.CancelObservedTurns++
			if sample.CancelFenceAppliedAtMS == nil {
				report.Violations = append(report.Violations, fmt.Sprintf("turn %s missing cancel fence marker", sample.TurnID))
			} else {
				latency := *sample.CancelFenceAppliedAtMS - *sample.CancelAcceptedAtMS
				if latency < 0 {
					report.Violations = append(report.Violations, fmt.Sprintf("turn %s has negative cancel fence latency", sample.TurnID))
				} else {
					cancelFenceLatencies = append(cancelFenceLatencies, latency)
				}
			}
		}
	}

	if len(turnOpenLatencies) > 0 {
		p95 := percentile95(turnOpenLatencies)
		report.TurnOpenDecisionP95MS = &p95
		if p95 > thresholds.TurnOpenDecisionP95MS {
			report.Violations = append(report.Violations, fmt.Sprintf("turn-open p95=%dms exceeds threshold=%dms", p95, thresholds.TurnOpenDecisionP95MS))
		}
	}
	if len(firstOutputLatencies) > 0 {
		p95 := percentile95(firstOutputLatencies)
		report.FirstOutputP95MS = &p95
		if p95 > thresholds.FirstOutputP95MS {
			report.Violations = append(report.Violations, fmt.Sprintf("first-output p95=%dms exceeds threshold=%dms", p95, thresholds.FirstOutputP95MS))
		}
	}
	if len(cancelFenceLatencies) > 0 {
		p95 := percentile95(cancelFenceLatencies)
		report.CancelFenceP95MS = &p95
		if p95 > thresholds.CancelFenceP95MS {
			report.Violations = append(report.Violations, fmt.Sprintf("cancel-fence p95=%dms exceeds threshold=%dms", p95, thresholds.CancelFenceP95MS))
		}
	}

	if report.AcceptedTurns > 0 {
		report.BaselineCompletenessRatio = float64(completeAccepted) / float64(report.AcceptedTurns)
		report.TerminalCorrectnessRatio = float64(terminalCorrectAccepted) / float64(report.AcceptedTurns)
	}
	if report.BaselineCompletenessRatio < thresholds.RequiredCompleteness {
		report.Violations = append(report.Violations, fmt.Sprintf("OR-02 baseline completeness=%.2f below required=%.2f", report.BaselineCompletenessRatio, thresholds.RequiredCompleteness))
	}
	if report.StaleAcceptedOutputs > thresholds.MaxStaleAcceptedOutput {
		report.Violations = append(report.Violations, fmt.Sprintf("accepted stale-epoch outputs=%d exceeds max=%d", report.StaleAcceptedOutputs, thresholds.MaxStaleAcceptedOutput))
	}
	if report.TerminalCorrectnessRatio < 1.0 {
		report.Violations = append(report.Violations, fmt.Sprintf("terminal lifecycle correctness=%.2f below required=1.00", report.TerminalCorrectnessRatio))
	}
	if report.AcceptedTurns == 0 {
		report.Violations = append(report.Violations, "no accepted turns available for SLO validation")
	}

	report.Passed = len(report.Violations) == 0
	return report
}

func hasValidTerminalSequence(events []string) bool {
	if len(events) != 2 {
		return false
	}
	if events[1] != "close" {
		return false
	}
	return events[0] == "commit" || events[0] == "abort"
}

func percentile95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]int64(nil), values...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	index := int(math.Ceil(0.95*float64(len(copied)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copied) {
		index = len(copied) - 1
	}
	return copied[index]
}
