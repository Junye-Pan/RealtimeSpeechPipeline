package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	replaycmp "github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
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
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("rspp-cli usage:")
	fmt.Println("  rspp-cli validate-contracts [fixture_root]")
	fmt.Println("  rspp-cli replay-smoke-report [output_path]")
}

type replaySmokeReport struct {
	GeneratedAtUTC     string                 `json:"generated_at_utc"`
	TimingToleranceMS  int64                  `json:"timing_tolerance_ms"`
	TotalDivergences   int                    `json:"total_divergences"`
	ByClass            map[string]int         `json:"by_class"`
	Divergences        []obs.ReplayDivergence `json:"divergences"`
	FailingDivergences []string               `json:"failing_divergences"`
}

const replaySmokeTimingToleranceMS int64 = 15

func writeReplaySmokeReport(outputPath string) error {
	divergences := buildReplaySmokeDivergences()
	byClass := map[string]int{
		string(obs.PlanDivergence):      0,
		string(obs.OutcomeDivergence):   0,
		string(obs.OrderingDivergence):  0,
		string(obs.AuthorityDivergence): 0,
		string(obs.TimingDivergence):    0,
	}
	report := replaySmokeReport{
		GeneratedAtUTC:    time.Now().UTC().Format(time.RFC3339),
		TimingToleranceMS: replaySmokeTimingToleranceMS,
		TotalDivergences:  len(divergences),
		ByClass:           byClass,
		Divergences:       divergences,
	}
	for _, d := range divergences {
		report.ByClass[string(d.Class)]++
	}
	report.FailingDivergences = failingDivergences(report.ByClass)

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

	if len(report.FailingDivergences) > 0 {
		return fmt.Errorf("forbidden replay divergences present: %v", report.FailingDivergences)
	}
	return nil
}

func buildReplaySmokeDivergences() []obs.ReplayDivergence {
	epoch := int64(7)
	baseline := []replaycmp.TraceArtifact{
		{
			PlanHash:           "plan-smoke-a",
			OrderingMarker:     "runtime_sequence:100",
			AuthorityEpoch:     7,
			RuntimeTimestampMS: 100,
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
			PlanHash:           "plan-smoke-a",
			OrderingMarker:     "runtime_sequence:110",
			AuthorityEpoch:     7,
			RuntimeTimestampMS: 110,
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

func failingDivergences(byClass map[string]int) []string {
	out := make([]string, 0, 5)
	for _, cls := range []obs.DivergenceClass{
		obs.PlanDivergence,
		obs.OutcomeDivergence,
		obs.AuthorityDivergence,
	} {
		if byClass[string(cls)] > 0 {
			out = append(out, string(cls))
		}
	}
	return out
}

func renderReplaySmokeSummary(report replaySmokeReport) string {
	lines := []string{
		"# Replay Smoke Divergence Report",
		"",
		"Generated at (UTC): " + report.GeneratedAtUTC,
		fmt.Sprintf("Timing tolerance (ms): %d", report.TimingToleranceMS),
		fmt.Sprintf("Total divergences: %d", report.TotalDivergences),
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

	if len(report.FailingDivergences) == 0 {
		lines = append(lines, "", "Status: PASS")
	} else {
		lines = append(lines, "", "Status: FAIL", "- Forbidden divergences: "+strings.Join(report.FailingDivergences, ", "))
	}
	return strings.Join(lines, "\n") + "\n"
}
