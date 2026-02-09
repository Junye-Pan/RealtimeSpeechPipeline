package ops

import "testing"

func TestEvaluateMVPSLOGatesPass(t *testing.T) {
	t.Parallel()

	samples := []TurnMetrics{
		newAcceptedTurn("turn-1", 0, 90, 500, nil, nil, true, false, []string{"commit", "close"}, true),
		newAcceptedTurn("turn-2", 0, 100, 700, nil, nil, true, false, []string{"abort", "close"}, true),
		newAcceptedTurn("turn-3", 0, 110, 900, int64Ptr(1200), int64Ptr(1290), true, false, []string{"abort", "close"}, false),
	}

	report := EvaluateMVPSLOGates(samples, DefaultMVPSLOThresholds())
	if !report.Passed {
		t.Fatalf("expected report to pass, got violations: %+v", report.Violations)
	}
	if report.BaselineCompletenessRatio != 1.0 {
		t.Fatalf("expected complete OR-02 coverage, got %.2f", report.BaselineCompletenessRatio)
	}
}

func TestEvaluateMVPSLOGatesFail(t *testing.T) {
	t.Parallel()

	samples := []TurnMetrics{
		newAcceptedTurn("turn-bad-1", 0, 250, 2000, int64Ptr(2200), int64Ptr(2500), false, true, []string{"commit", "close"}, true),
		{
			TurnID:               "turn-bad-2",
			Accepted:             true,
			HappyPath:            true,
			TurnOpenProposedAtMS: int64Ptr(10),
			TurnOpenAtMS:         int64Ptr(20),
			BaselineComplete:     true,
			TerminalEvents:       []string{"commit"},
		},
	}

	report := EvaluateMVPSLOGates(samples, DefaultMVPSLOThresholds())
	if report.Passed {
		t.Fatalf("expected report to fail")
	}
	if len(report.Violations) < 4 {
		t.Fatalf("expected multiple violations, got %+v", report.Violations)
	}
}

func newAcceptedTurn(
	turnID string,
	openProposed int64,
	open int64,
	firstOutput int64,
	cancelAccepted *int64,
	cancelFence *int64,
	baselineComplete bool,
	staleAccepted bool,
	terminal []string,
	happyPath bool,
) TurnMetrics {
	return TurnMetrics{
		TurnID:                   turnID,
		Accepted:                 true,
		HappyPath:                happyPath,
		TurnOpenProposedAtMS:     int64Ptr(openProposed),
		TurnOpenAtMS:             int64Ptr(open),
		FirstOutputAtMS:          int64Ptr(firstOutput),
		CancelAcceptedAtMS:       cancelAccepted,
		CancelFenceAppliedAtMS:   cancelFence,
		BaselineComplete:         baselineComplete,
		AcceptedStaleEpochOutput: staleAccepted,
		TerminalEvents:           terminal,
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
