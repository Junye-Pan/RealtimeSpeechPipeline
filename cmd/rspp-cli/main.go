package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	replaycmp "github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/ops"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/regression"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/validation"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "validate-contracts":
		fixtureRoot := filepath.Join("test", "contract", "fixtures")
		if len(os.Args) >= 3 {
			fixtureRoot = os.Args[2]
		}
		summary, err := validation.ValidateContractFixtures(fixtureRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "contract validation failed to execute: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(validation.RenderSummary(summary))
		if summary.Failed > 0 {
			os.Exit(1)
		}
	case "replay-smoke-report":
		outputPath := filepath.Join(".codex", "replay", "smoke-report.json")
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if err := writeReplaySmokeReport(outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write replay smoke report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("replay smoke report written: %s\n", outputPath)
		fmt.Printf("replay smoke summary written: %s\n", summaryPath)
	case "slo-gates-report":
		outputPath := filepath.Join(".codex", "ops", "slo-gates-report.json")
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if err := writeSLOGatesReport(outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write slo gates report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("slo gates report written: %s\n", outputPath)
		fmt.Printf("slo gates summary written: %s\n", summaryPath)
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("rspp-cli usage:")
	fmt.Println("  rspp-cli validate-contracts [fixture_root]")
	fmt.Println("  rspp-cli replay-smoke-report [output_path]")
	fmt.Println("  rspp-cli slo-gates-report [output_path]")
}

type replaySmokeReport struct {
	GeneratedAtUTC     string                 `json:"generated_at_utc"`
	TimingToleranceMS  int64                  `json:"timing_tolerance_ms"`
	TotalDivergences   int                    `json:"total_divergences"`
	ByClass            map[string]int         `json:"by_class"`
	Divergences        []obs.ReplayDivergence `json:"divergences"`
	FailingCount       int                    `json:"failing_count"`
	UnexplainedCount   int                    `json:"unexplained_count"`
	MissingExpected    int                    `json:"missing_expected"`
	FailingDivergences []string               `json:"failing_divergences"`
}

const replaySmokeTimingToleranceMS int64 = 15

func writeReplaySmokeReport(outputPath string) error {
	divergences := buildReplaySmokeDivergences()
	evaluation := regression.EvaluateDivergences(divergences, regression.DivergencePolicy{
		TimingToleranceMS: replaySmokeTimingToleranceMS,
	})

	byClass := map[string]int{
		string(obs.PlanDivergence):      0,
		string(obs.OutcomeDivergence):   0,
		string(obs.OrderingDivergence):  0,
		string(obs.AuthorityDivergence): 0,
		string(obs.TimingDivergence):    0,
	}
	for _, d := range divergences {
		byClass[string(d.Class)]++
	}
	report := replaySmokeReport{
		GeneratedAtUTC:     time.Now().UTC().Format(time.RFC3339),
		TimingToleranceMS:  replaySmokeTimingToleranceMS,
		TotalDivergences:   len(divergences),
		ByClass:            byClass,
		Divergences:        divergences,
		FailingCount:       len(evaluation.Failing),
		UnexplainedCount:   len(evaluation.Unexplained),
		MissingExpected:    len(evaluation.MissingExpected),
		FailingDivergences: uniqueFailingClasses(evaluation.Failing),
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}

	summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
	if err := os.WriteFile(summaryPath, []byte(renderReplaySmokeSummary(report)), 0o644); err != nil {
		return err
	}

	if report.FailingCount > 0 {
		return fmt.Errorf("forbidden replay divergences present: %v", report.FailingDivergences)
	}
	return nil
}

func buildReplaySmokeDivergences() []obs.ReplayDivergence {
	epoch := int64(7)
	baseline := []replaycmp.TraceArtifact{
		{
			PlanHash:              "plan-smoke-a",
			SnapshotProvenanceRef: "snapshot-a",
			OrderingMarker:        "runtime_sequence:100",
			AuthorityEpoch:        7,
			RuntimeTimestampMS:    100,
			Decision: controlplane.DecisionOutcome{
				OutcomeKind:        controlplane.OutcomeAdmit,
				Phase:              controlplane.PhasePreTurn,
				Scope:              controlplane.ScopeSession,
				SessionID:          "sess-rd-smoke",
				EventID:            "evt-rd-smoke-1",
				RuntimeTimestampMS: 100,
				WallClockMS:        100,
				EmittedBy:          controlplane.EmitterRK25,
				Reason:             "admission_capacity_allow",
			},
		},
		{
			PlanHash:              "plan-smoke-a",
			SnapshotProvenanceRef: "snapshot-a",
			OrderingMarker:        "runtime_sequence:110",
			AuthorityEpoch:        7,
			RuntimeTimestampMS:    110,
			Decision: controlplane.DecisionOutcome{
				OutcomeKind:        controlplane.OutcomeStaleEpochReject,
				Phase:              controlplane.PhasePreTurn,
				Scope:              controlplane.ScopeTurn,
				SessionID:          "sess-rd-smoke",
				TurnID:             "turn-rd-smoke-1",
				EventID:            "evt-rd-smoke-2",
				RuntimeTimestampMS: 110,
				WallClockMS:        110,
				EmittedBy:          controlplane.EmitterRK24,
				AuthorityEpoch:     &epoch,
				Reason:             "authority_epoch_mismatch",
			},
		},
	}
	replayed := append([]replaycmp.TraceArtifact(nil), baseline...)
	return replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: replaySmokeTimingToleranceMS})
}

func uniqueFailingClasses(failing []obs.ReplayDivergence) []string {
	if len(failing) == 0 {
		return nil
	}
	unique := make(map[string]struct{})
	for _, item := range failing {
		unique[string(item.Class)] = struct{}{}
	}
	classes := make([]string, 0, len(unique))
	for class := range unique {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	return classes
}

func renderReplaySmokeSummary(report replaySmokeReport) string {
	lines := []string{
		"# Replay Smoke Divergence Report",
		"",
		"Generated at (UTC): " + report.GeneratedAtUTC,
		fmt.Sprintf("Timing tolerance (ms): %d", report.TimingToleranceMS),
		fmt.Sprintf("Total divergences: %d", report.TotalDivergences),
		fmt.Sprintf("Failing divergences: %d", report.FailingCount),
		fmt.Sprintf("Unexplained divergences: %d", report.UnexplainedCount),
		fmt.Sprintf("Missing expected divergences: %d", report.MissingExpected),
		"",
		"## By class",
	}
	for _, cls := range []obs.DivergenceClass{
		obs.PlanDivergence,
		obs.OutcomeDivergence,
		obs.OrderingDivergence,
		obs.AuthorityDivergence,
		obs.TimingDivergence,
	} {
		lines = append(lines, fmt.Sprintf("- %s: %d", cls, report.ByClass[string(cls)]))
	}

	if report.FailingCount == 0 {
		lines = append(lines, "", "Status: PASS")
	} else {
		lines = append(lines, "", "Status: FAIL", "- Forbidden divergences: "+strings.Join(report.FailingDivergences, ", "))
	}
	return strings.Join(lines, "\n") + "\n"
}

type sloGateArtifact struct {
	GeneratedAtUTC string               `json:"generated_at_utc"`
	Thresholds     ops.MVPSLOThresholds `json:"thresholds"`
	Report         ops.MVPSLOGateReport `json:"report"`
}

func writeSLOGatesReport(outputPath string) error {
	thresholds := ops.DefaultMVPSLOThresholds()
	report := ops.EvaluateMVPSLOGates(buildSLOGateSamples(), thresholds)
	artifact := sloGateArtifact{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		Thresholds:     thresholds,
		Report:         report,
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}

	summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
	if err := os.WriteFile(summaryPath, []byte(renderSLOGatesSummary(artifact)), 0o644); err != nil {
		return err
	}

	if !artifact.Report.Passed {
		return fmt.Errorf("mvp slo gate failed: %v", artifact.Report.Violations)
	}
	return nil
}

func buildSLOGateSamples() []ops.TurnMetrics {
	open0 := int64(0)
	open80 := int64(80)
	first420 := int64(420)
	open90 := int64(90)
	first560 := int64(560)
	open70 := int64(70)
	first360 := int64(360)
	cancelAccepted := int64(500)
	cancelFence := int64(620)

	return []ops.TurnMetrics{
		{
			TurnID:               "turn-slo-1",
			Accepted:             true,
			HappyPath:            true,
			TurnOpenProposedAtMS: &open0,
			TurnOpenAtMS:         &open80,
			FirstOutputAtMS:      &first420,
			BaselineComplete:     true,
			TerminalEvents:       []string{"commit", "close"},
		},
		{
			TurnID:               "turn-slo-2",
			Accepted:             true,
			HappyPath:            true,
			TurnOpenProposedAtMS: &open0,
			TurnOpenAtMS:         &open90,
			FirstOutputAtMS:      &first560,
			BaselineComplete:     true,
			TerminalEvents:       []string{"commit", "close"},
		},
		{
			TurnID:                 "turn-slo-3",
			Accepted:               true,
			HappyPath:              true,
			TurnOpenProposedAtMS:   &open0,
			TurnOpenAtMS:           &open70,
			FirstOutputAtMS:        &first360,
			CancelAcceptedAtMS:     &cancelAccepted,
			CancelFenceAppliedAtMS: &cancelFence,
			BaselineComplete:       true,
			TerminalEvents:         []string{"abort", "close"},
		},
	}
}

func renderSLOGatesSummary(artifact sloGateArtifact) string {
	report := artifact.Report
	lines := []string{
		"# MVP SLO Gates Report",
		"",
		"Generated at (UTC): " + artifact.GeneratedAtUTC,
		fmt.Sprintf("Samples: %d", report.Samples),
		fmt.Sprintf("Accepted turns: %d", report.AcceptedTurns),
		fmt.Sprintf("Happy-path turns: %d", report.HappyPathTurns),
		fmt.Sprintf("Cancel-observed turns: %d", report.CancelObservedTurns),
		fmt.Sprintf("OR-02 completeness: %.2f", report.BaselineCompletenessRatio),
		fmt.Sprintf("Stale accepted outputs: %d", report.StaleAcceptedOutputs),
		fmt.Sprintf("Terminal correctness: %.2f", report.TerminalCorrectnessRatio),
	}
	if report.TurnOpenDecisionP95MS != nil {
		lines = append(lines, fmt.Sprintf("Turn-open p95: %d ms", *report.TurnOpenDecisionP95MS))
	}
	if report.FirstOutputP95MS != nil {
		lines = append(lines, fmt.Sprintf("First-output p95: %d ms", *report.FirstOutputP95MS))
	}
	if report.CancelFenceP95MS != nil {
		lines = append(lines, fmt.Sprintf("Cancel-fence p95: %d ms", *report.CancelFenceP95MS))
	}

	if report.Passed {
		lines = append(lines, "", "Status: PASS")
	} else {
		lines = append(lines, "", "Status: FAIL", "## Violations")
		for _, violation := range report.Violations {
			lines = append(lines, "- "+violation)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
