package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/ops"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/regression"
)

func TestLoadReplayFixturePolicy(t *testing.T) {
	t.Parallel()

	metadataPath := filepath.Join(t.TempDir(), "metadata.json")
	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"fixture-a": {
				Gate:              "full",
				TimingToleranceMS: int64Ptr(27),
				ExpectedDivergences: []regression.ExpectedDivergence{{
					Class:    obs.OrderingDivergence,
					Scope:    "turn:turn-a",
					Approved: true,
				}},
			},
		},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if err := osWriteFile(metadataPath, data); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	policy, toleranceMS, err := loadReplayFixturePolicy(metadataPath, "fixture-a", 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toleranceMS != 27 {
		t.Fatalf("expected tolerance 27, got %d", toleranceMS)
	}
	if policy.TimingToleranceMS != 27 {
		t.Fatalf("expected policy tolerance 27, got %d", policy.TimingToleranceMS)
	}
	if len(policy.Expected) != 1 {
		t.Fatalf("expected one expected divergence, got %+v", policy.Expected)
	}
	if policy.Expected[0].Class != obs.OrderingDivergence || !policy.Expected[0].Approved {
		t.Fatalf("unexpected expected divergence policy: %+v", policy.Expected[0])
	}
}

func TestLoadReplayFixturePolicyMissingFixture(t *testing.T) {
	t.Parallel()

	metadataPath := filepath.Join(t.TempDir(), "metadata.json")
	data := []byte(`{"fixtures":{"fixture-a":{"timing_tolerance_ms":10}}}`)
	if err := osWriteFile(metadataPath, data); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	_, _, err := loadReplayFixturePolicy(metadataPath, "fixture-missing", 15)
	if err == nil {
		t.Fatalf("expected missing fixture error")
	}
}

func TestGenerateRuntimeBaselineArtifactFeedsSLOGates(t *testing.T) {
	t.Parallel()

	artifactPath := filepath.Join(t.TempDir(), "runtime-baseline.json")
	entries, err := generateRuntimeBaselineArtifact(artifactPath)
	if err != nil {
		t.Fatalf("unexpected generation error: %v", err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected at least 3 runtime baseline entries, got %d", len(entries))
	}

	artifact, err := timeline.ReadBaselineArtifact(artifactPath)
	if err != nil {
		t.Fatalf("unexpected artifact read error: %v", err)
	}
	if len(artifact.Entries) != len(entries) {
		t.Fatalf("artifact entries mismatch: got=%d want=%d", len(artifact.Entries), len(entries))
	}

	report := ops.EvaluateMVPSLOGates(toTurnMetrics(entries), ops.DefaultMVPSLOThresholds())
	if !report.Passed {
		t.Fatalf("expected generated runtime baseline metrics to pass SLO gates, got %+v", report.Violations)
	}
}

func TestSelectReplayFixtureIDsByGate(t *testing.T) {
	t.Parallel()

	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"quick-a": {Gate: "quick"},
			"full-a":  {Gate: "full"},
			"both-a":  {Gate: "both"},
		},
	}

	quick, err := selectReplayFixtureIDs(metadata, "quick")
	if err != nil {
		t.Fatalf("unexpected quick gate error: %v", err)
	}
	if len(quick) != 2 || quick[0] != "both-a" || quick[1] != "quick-a" {
		t.Fatalf("unexpected quick fixture selection: %+v", quick)
	}

	full, err := selectReplayFixtureIDs(metadata, "full")
	if err != nil {
		t.Fatalf("unexpected full gate error: %v", err)
	}
	if len(full) != 2 || full[0] != "both-a" || full[1] != "full-a" {
		t.Fatalf("unexpected full fixture selection: %+v", full)
	}
}

func TestWriteReplayRegressionReport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	metadataPath := filepath.Join(tmp, "metadata.json")
	outputPath := filepath.Join(tmp, "regression.json")
	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"rd-ordering-approved-1": {
				Gate:              "full",
				TimingToleranceMS: int64Ptr(15),
				ExpectedDivergences: []regression.ExpectedDivergence{{
					Class:    obs.OrderingDivergence,
					Scope:    "turn:turn-ordering-approved-1",
					Approved: true,
				}},
			},
		},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if err := osWriteFile(metadataPath, data); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	if err := writeReplayRegressionReport(outputPath, metadataPath, "full"); err != nil {
		t.Fatalf("expected replay regression report to pass, got %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("unexpected report read error: %v", err)
	}
	var report replayRegressionReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("unexpected report decode error: %v", err)
	}
	if report.FixtureCount != 1 || report.FailingCount != 0 {
		t.Fatalf("unexpected replay regression report contents: %+v", report)
	}

	fixtureJSONPath := filepath.Join(tmp, replayFixtureReportsDirName, "rd-ordering-approved-1.json")
	fixtureMarkdownPath := filepath.Join(tmp, replayFixtureReportsDirName, "rd-ordering-approved-1.md")

	rawFixture, err := os.ReadFile(fixtureJSONPath)
	if err != nil {
		t.Fatalf("unexpected fixture report read error: %v", err)
	}
	var fixtureReport replayFixtureArtifact
	if err := json.Unmarshal(rawFixture, &fixtureReport); err != nil {
		t.Fatalf("unexpected fixture report decode error: %v", err)
	}
	if fixtureReport.FixtureID != "rd-ordering-approved-1" || fixtureReport.Status != "PASS" {
		t.Fatalf("unexpected fixture report contents: %+v", fixtureReport)
	}

	rawFixtureSummary, err := os.ReadFile(fixtureMarkdownPath)
	if err != nil {
		t.Fatalf("unexpected fixture summary read error: %v", err)
	}
	if !strings.Contains(string(rawFixtureSummary), "Fixture: rd-ordering-approved-1") {
		t.Fatalf("expected fixture summary to include fixture id, got %q", string(rawFixtureSummary))
	}
}

func TestWriteReplayRegressionReportWritesFixtureArtifactsOnFailure(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	metadataPath := filepath.Join(tmp, "metadata.json")
	outputPath := filepath.Join(tmp, "regression.json")
	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"rd-ordering-approved-1": {
				Gate:              "full",
				TimingToleranceMS: int64Ptr(15),
			},
		},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if err := osWriteFile(metadataPath, data); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	if err := writeReplayRegressionReport(outputPath, metadataPath, "full"); err == nil {
		t.Fatalf("expected replay regression report failure when expected divergences are not declared")
	}

	fixtureJSONPath := filepath.Join(tmp, replayFixtureReportsDirName, "rd-ordering-approved-1.json")
	rawFixture, err := os.ReadFile(fixtureJSONPath)
	if err != nil {
		t.Fatalf("unexpected fixture report read error on failure path: %v", err)
	}
	var fixtureReport replayFixtureArtifact
	if err := json.Unmarshal(rawFixture, &fixtureReport); err != nil {
		t.Fatalf("unexpected fixture report decode error on failure path: %v", err)
	}
	if fixtureReport.Status != "FAIL" || fixtureReport.FailingCount == 0 {
		t.Fatalf("unexpected failure fixture report contents: %+v", fixtureReport)
	}
}

func TestToTurnMetricsComputesCompleteness(t *testing.T) {
	t.Parallel()

	open0 := int64(0)
	open100 := int64(100)
	first500 := int64(500)
	entries := []timeline.BaselineEvidence{
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			EnvelopeSnapshot:     "eventabi/v1",
			PayloadTags:          []eventabi.PayloadClass{eventabi.PayloadMetadata},
			PlanHash:             "plan/turn-1",
			SnapshotProvenance:   defaultSnapshotProvenance(),
			DecisionOutcomes:     []controlplane.DecisionOutcome{sloAdmitDecision("sess-1", "turn-1", "evt-1-admit", 100)},
			DeterminismSeed:      1,
			OrderingMarkers:      []string{"runtime_sequence:1"},
			MergeRuleID:          "merge/default",
			MergeRuleVersion:     "v1.0",
			AuthorityEpoch:       1,
			TerminalOutcome:      "commit",
			CloseEmitted:         true,
			TurnOpenProposedAtMS: &open0,
			TurnOpenAtMS:         &open100,
			FirstOutputAtMS:      &first500,
		},
	}

	samples := toTurnMetrics(entries)
	if len(samples) != 1 {
		t.Fatalf("expected one sample, got %d", len(samples))
	}
	if !samples[0].BaselineComplete {
		t.Fatalf("expected baseline to be complete, got %+v", samples[0])
	}
	if !samples[0].HappyPath {
		t.Fatalf("expected happy path sample to be true")
	}
}

func TestWriteSLOGatesReportRequiresBaselineArtifact(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join(t.TempDir(), "slo.json")
	missingArtifactPath := filepath.Join(t.TempDir(), "missing-runtime-baseline.json")
	if err := writeSLOGatesReport(outputPath, missingArtifactPath); err == nil {
		t.Fatalf("expected missing baseline artifact to fail slo-gates-report")
	}
}

func TestWriteSLOGatesReportFromRuntimeArtifact(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	artifactPath := filepath.Join(tmp, "runtime-baseline.json")
	outputPath := filepath.Join(tmp, "slo.json")

	if err := writeRuntimeBaselineArtifact(artifactPath); err != nil {
		t.Fatalf("unexpected runtime baseline generation error: %v", err)
	}
	if err := writeSLOGatesReport(outputPath, artifactPath); err != nil {
		t.Fatalf("expected slo report generation from runtime artifact to pass, got %v", err)
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
