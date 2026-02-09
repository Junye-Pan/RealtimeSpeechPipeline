package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		fmt.Printf("replay smoke report written: %s\n", outputPath)
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
	GeneratedAtUTC   string                 `json:"generated_at_utc"`
	TotalDivergences int                    `json:"total_divergences"`
	ByClass          map[string]int         `json:"by_class"`
	Divergences      []obs.ReplayDivergence `json:"divergences"`
}

func writeReplaySmokeReport(outputPath string) error {
	divergences := buildReplaySmokeDivergences()
	report := replaySmokeReport{
		GeneratedAtUTC:   time.Now().UTC().Format(time.RFC3339),
		TotalDivergences: len(divergences),
		ByClass:          map[string]int{},
		Divergences:      divergences,
	}
	for _, d := range divergences {
		report.ByClass[string(d.Class)]++
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0o644)
}

func buildReplaySmokeDivergences() []obs.ReplayDivergence {
	epoch := int64(7)
	baseline := []controlplane.DecisionOutcome{
		{
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
		{
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
	}
	replayed := append([]controlplane.DecisionOutcome(nil), baseline...)
	return replaycmp.CompareDecisionOutcomes(baseline, replayed)
}
