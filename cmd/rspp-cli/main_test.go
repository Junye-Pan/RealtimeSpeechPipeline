package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/ops"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/regression"
	toolingrelease "github.com/tiger/realtime-speech-pipeline/internal/tooling/release"
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

func TestWriteReplayRegressionReportInvocationLatencyThresholdPass(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	metadataPath := filepath.Join(tmp, "metadata.json")
	outputPath := filepath.Join(tmp, "regression.json")
	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"rd-003-baseline-completeness": {
				Gate:                              "full",
				TimingToleranceMS:                 int64Ptr(15),
				FinalAttemptLatencyThresholdMS:    int64Ptr(20),
				TotalInvocationLatencyThresholdMS: int64Ptr(40),
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
		t.Fatalf("expected invocation latency thresholds within limits to pass, got %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(tmp, replayFixtureReportsDirName, "rd-003-baseline-completeness.json"))
	if err != nil {
		t.Fatalf("unexpected fixture artifact read error: %v", err)
	}
	var fixtureReport replayFixtureArtifact
	if err := json.Unmarshal(raw, &fixtureReport); err != nil {
		t.Fatalf("unexpected fixture artifact decode error: %v", err)
	}
	if fixtureReport.InvocationLatencyBreaches != 0 || fixtureReport.FailingCount != 0 {
		t.Fatalf("unexpected pass fixture report: %+v", fixtureReport)
	}
}

func TestWriteReplayRegressionReportInvocationLatencyThresholdPassWithMetadataScope(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	metadataPath := filepath.Join(tmp, "metadata.json")
	outputPath := filepath.Join(tmp, "regression.json")
	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"rd-ordering-approved-1": {
				Gate:                              "full",
				TimingToleranceMS:                 int64Ptr(15),
				FinalAttemptLatencyThresholdMS:    int64Ptr(20),
				TotalInvocationLatencyThresholdMS: int64Ptr(40),
				InvocationLatencyScopes:           []string{"turn:turn-ordering-approved-1"},
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
		t.Fatalf("expected metadata-scoped invocation latency thresholds within limits to pass, got %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(tmp, replayFixtureReportsDirName, "rd-ordering-approved-1.json"))
	if err != nil {
		t.Fatalf("unexpected fixture artifact read error: %v", err)
	}
	var fixtureReport replayFixtureArtifact
	if err := json.Unmarshal(raw, &fixtureReport); err != nil {
		t.Fatalf("unexpected fixture artifact decode error: %v", err)
	}
	if fixtureReport.InvocationLatencyBreaches != 0 || fixtureReport.FailingCount != 0 {
		t.Fatalf("unexpected pass fixture report: %+v", fixtureReport)
	}
}

func TestWriteReplayRegressionReportInvocationLatencyThresholdFail(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	metadataPath := filepath.Join(tmp, "metadata.json")
	outputPath := filepath.Join(tmp, "regression.json")
	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"rd-003-baseline-completeness": {
				Gate:                              "full",
				TimingToleranceMS:                 int64Ptr(15),
				FinalAttemptLatencyThresholdMS:    int64Ptr(5),
				TotalInvocationLatencyThresholdMS: int64Ptr(10),
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
		t.Fatalf("expected invocation latency threshold breach to fail replay regression report")
	}

	raw, err := os.ReadFile(filepath.Join(tmp, replayFixtureReportsDirName, "rd-003-baseline-completeness.json"))
	if err != nil {
		t.Fatalf("unexpected fixture artifact read error: %v", err)
	}
	var fixtureReport replayFixtureArtifact
	if err := json.Unmarshal(raw, &fixtureReport); err != nil {
		t.Fatalf("unexpected fixture artifact decode error: %v", err)
	}
	if fixtureReport.InvocationLatencyBreaches == 0 || fixtureReport.FailingCount == 0 || fixtureReport.ByClass[string(obs.TimingDivergence)] == 0 {
		t.Fatalf("expected timing divergence failures from latency threshold breach, got %+v", fixtureReport)
	}
}

func TestWriteReplayRegressionReportInvocationLatencyThresholdMissingEvidenceFail(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	metadataPath := filepath.Join(tmp, "metadata.json")
	outputPath := filepath.Join(tmp, "regression.json")
	metadata := replayFixtureMetadata{
		Fixtures: map[string]replayFixturePolicy{
			"rd-003-baseline-completeness": {
				Gate:                              "full",
				TimingToleranceMS:                 int64Ptr(15),
				FinalAttemptLatencyThresholdMS:    int64Ptr(20),
				TotalInvocationLatencyThresholdMS: int64Ptr(40),
				InvocationLatencyScopes:           []string{"turn:turn-missing"},
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
		t.Fatalf("expected missing invocation latency evidence to fail replay regression report")
	}

	raw, err := os.ReadFile(filepath.Join(tmp, replayFixtureReportsDirName, "rd-003-baseline-completeness.json"))
	if err != nil {
		t.Fatalf("unexpected fixture artifact read error: %v", err)
	}
	var fixtureReport replayFixtureArtifact
	if err := json.Unmarshal(raw, &fixtureReport); err != nil {
		t.Fatalf("unexpected fixture artifact decode error: %v", err)
	}
	if fixtureReport.InvocationLatencyBreaches == 0 || fixtureReport.FailingCount == 0 {
		t.Fatalf("expected missing evidence breaches to be recorded, got %+v", fixtureReport)
	}
}

func TestInvocationLatencyScopesForFixturePrefersMetadataScopes(t *testing.T) {
	t.Parallel()

	scopes := invocationLatencyScopesForFixture("rd-ordering-approved-1", replayFixturePolicy{
		InvocationLatencyScopes: []string{
			"turn:turn-z",
			"turn:turn-a",
			"turn:turn-z",
			" ",
		},
	})
	want := []string{"turn:turn-a", "turn:turn-z"}
	if len(scopes) != len(want) {
		t.Fatalf("expected scopes %+v, got %+v", want, scopes)
	}
	for i := range want {
		if scopes[i] != want[i] {
			t.Fatalf("expected scopes %+v, got %+v", want, scopes)
		}
	}
}

func TestInvocationLatencyScopesForFixtureFallsBackToDerivedScope(t *testing.T) {
	t.Parallel()

	scopes := invocationLatencyScopesForFixture("cf-001-cancel-fence", replayFixturePolicy{})
	if len(scopes) != 1 || scopes[0] != "turn:turn-cf-001" {
		t.Fatalf("expected derived fixture scope, got %+v", scopes)
	}
}

func TestBuildInvocationLatencyThresholdDivergencesDeterministicAcrossScopes(t *testing.T) {
	t.Parallel()

	policy := replayFixturePolicy{
		FinalAttemptLatencyThresholdMS:    int64Ptr(11),
		TotalInvocationLatencyThresholdMS: int64Ptr(30),
		InvocationLatencyScopes:           []string{"turn:turn-z", "turn:turn-a", "turn:turn-z"},
	}
	samplesByScope := map[string]invocationLatencySample{
		"turn:turn-a": {
			Scope:                    "turn:turn-a",
			FinalAttemptLatencyMS:    12,
			TotalInvocationLatencyMS: 31,
		},
	}

	divergences := buildInvocationLatencyThresholdDivergences("rd-ordering-approved-1", policy, samplesByScope, nil)
	wantScopes := []string{
		"invocation_latency_final:turn:turn-a",
		"invocation_latency_total:turn:turn-a",
		"invocation_latency_final:turn:turn-z",
		"invocation_latency_total:turn:turn-z",
	}
	if len(divergences) != len(wantScopes) {
		t.Fatalf("expected %d divergences, got %+v", len(wantScopes), divergences)
	}
	for i, wantScope := range wantScopes {
		if divergences[i].Class != obs.TimingDivergence || divergences[i].Scope != wantScope {
			t.Fatalf("unexpected divergence ordering/content at index %d: %+v", i, divergences)
		}
	}
}

func TestInvocationLatencySamplesFromBaselineEntriesAggregatesByTurn(t *testing.T) {
	t.Parallel()

	entries := []timeline.BaselineEvidence{
		{
			TurnID: "turn-rd-003",
			InvocationOutcomes: []timeline.InvocationOutcomeEvidence{
				{
					ProviderInvocationID:     "inv-1",
					Modality:                 "llm",
					ProviderID:               "llm-a",
					OutcomeClass:             "success",
					RetryDecision:            "none",
					AttemptCount:             1,
					FinalAttemptLatencyMS:    8,
					TotalInvocationLatencyMS: 21,
				},
				{
					ProviderInvocationID:     "inv-2",
					Modality:                 "llm",
					ProviderID:               "llm-b",
					OutcomeClass:             "success",
					RetryDecision:            "none",
					AttemptCount:             1,
					FinalAttemptLatencyMS:    12,
					TotalInvocationLatencyMS: 27,
				},
			},
		},
	}

	samples := invocationLatencySamplesFromBaselineEntries(entries)
	sample, ok := samples["turn:turn-rd-003"]
	if !ok {
		t.Fatalf("expected turn sample to exist, got %+v", samples)
	}
	if sample.FinalAttemptLatencyMS != 12 || sample.TotalInvocationLatencyMS != 27 {
		t.Fatalf("expected max latencies per turn sample, got %+v", sample)
	}
}

func TestToTurnMetricsComputesCompleteness(t *testing.T) {
	t.Parallel()

	open0 := int64(0)
	open100 := int64(100)
	terminal900 := int64(900)
	first500 := int64(500)
	entries := []timeline.BaselineEvidence{
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			EnvelopeSnapshot:     "eventabi/v1",
			PayloadTags:          []eventabi.PayloadClass{eventabi.PayloadMetadata},
			RedactionDecisions:   []eventabi.RedactionDecision{{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow}},
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
			TurnTerminalAtMS:     &terminal900,
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

func TestEvaluateLiveEndToEndGateWaivesAfterMultipleAttempts(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	chainPath := filepath.Join(tmp, "live-provider-chain-report.json")
	for _, name := range []string{
		"live-provider-chain-report.json",
		"live-provider-chain-report.streaming.json",
		"live-provider-chain-report.nonstreaming.json",
	} {
		if err := osWriteFile(filepath.Join(tmp, name), []byte(`{}`)); err != nil {
			t.Fatalf("write attempt artifact: %v", err)
		}
	}

	report := liveProviderChainArtifact{
		Status: "pass",
		Combinations: []liveProviderChainCombinationEntry{
			{Status: "pass", Latency: liveProviderChainLatency{TurnCompletionE2EMS: 2600}},
			{Status: "pass", Latency: liveProviderChainLatency{TurnCompletionE2EMS: 3100}},
		},
	}

	out := evaluateLiveEndToEndGate(report, ops.DefaultMVPSLOThresholds(), chainPath)
	if !out.Waived {
		t.Fatalf("expected waiver after multiple attempts, got %+v", out)
	}
	if !out.Passed {
		t.Fatalf("expected waived gate to pass, got %+v", out)
	}
	if len(out.Violations) != 0 {
		t.Fatalf("expected violations to be cleared after waiver, got %+v", out.Violations)
	}
	if len(out.AttemptFiles) < mvpE2EWaiveAfterAttempts {
		t.Fatalf("expected attempt files >= %d, got %+v", mvpE2EWaiveAfterAttempts, out.AttemptFiles)
	}
}

func TestEvaluateLiveEndToEndGateFailsWithoutWaiver(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	chainPath := filepath.Join(tmp, "live-provider-chain-report.json")
	if err := osWriteFile(chainPath, []byte(`{}`)); err != nil {
		t.Fatalf("write chain artifact: %v", err)
	}

	report := liveProviderChainArtifact{
		Status: "pass",
		Combinations: []liveProviderChainCombinationEntry{
			{Status: "pass", Latency: liveProviderChainLatency{TurnCompletionE2EMS: 2500}},
		},
	}

	out := evaluateLiveEndToEndGate(report, ops.DefaultMVPSLOThresholds(), chainPath)
	if out.Waived {
		t.Fatalf("expected no waiver with a single attempt, got %+v", out)
	}
	if out.Passed {
		t.Fatalf("expected gate failure without waiver, got %+v", out)
	}
	if len(out.Violations) == 0 {
		t.Fatalf("expected threshold violations, got none")
	}
}

func TestEvaluateMVPFixedDecisionGatesRequiresParityValidity(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	livekitPath := filepath.Join(tmp, "livekit-smoke-report.json")
	if err := osWriteFile(livekitPath, []byte(`{"probe":{"status":"passed"}}`)); err != nil {
		t.Fatalf("write livekit report: %v", err)
	}

	report := liveProviderChainArtifact{
		Status:                "pass",
		ExecutionMode:         "streaming",
		SemanticParity:        boolPtr(false),
		ParityComparisonValid: boolPtr(false),
		ParityInvalidReason:   "comparison identity mismatch",
		EnabledProviders: liveProviderChainEnabledProviders{
			STT: []string{"stt-deepgram"},
			LLM: []string{"llm-anthropic"},
			TTS: []string{"tts-elevenlabs"},
		},
		SelectedCombinations: []liveProviderChainComboSelection{
			{
				STTProviderID: "stt-deepgram",
				LLMProviderID: "llm-anthropic",
				TTSProviderID: "tts-elevenlabs",
			},
		},
	}

	out := evaluateMVPFixedDecisionGates(report, livekitPath)
	if !out.ParityMarkersPresent {
		t.Fatalf("expected parity markers to be detected, got %+v", out)
	}
	if out.ParityComparisonValid {
		t.Fatalf("expected parity comparison validity to be false, got %+v", out)
	}
	if !hasViolationContaining(out.Violations, "parity comparison invalid") {
		t.Fatalf("expected parity invalid violation, got %+v", out.Violations)
	}
}

func TestWriteLiveLatencyCompareReport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	streamingPath := filepath.Join(tmp, "streaming.json")
	nonStreamingPath := filepath.Join(tmp, "nonstreaming.json")
	outputPath := filepath.Join(tmp, "live-latency-compare.json")

	streaming := liveProviderChainArtifact{
		ExecutionMode:      "streaming",
		ComparisonIdentity: "cmp-a",
		Status:             "pass",
		Combinations: []liveProviderChainCombinationEntry{
			{
				Status:        "pass",
				STTProviderID: "stt-a",
				LLMProviderID: "llm-a",
				TTSProviderID: "tts-a",
				Handoffs:      []liveProviderChainHandoffEntry{{HandoffID: "h-1"}},
				Latency: liveProviderChainLatency{
					FirstAssistantAudioE2EMS: 1000,
					TurnCompletionE2EMS:      2200,
				},
			},
			{
				Status:        "pass",
				STTProviderID: "stt-b",
				LLMProviderID: "llm-b",
				TTSProviderID: "tts-b",
				Handoffs:      []liveProviderChainHandoffEntry{{HandoffID: "h-2"}},
				Latency: liveProviderChainLatency{
					FirstAssistantAudioE2EMS: 900,
					TurnCompletionE2EMS:      1800,
				},
			},
		},
	}
	nonStreaming := liveProviderChainArtifact{
		ExecutionMode:      "non_streaming",
		ComparisonIdentity: "cmp-a",
		Status:             "pass",
		Combinations: []liveProviderChainCombinationEntry{
			{
				Status:        "pass",
				STTProviderID: "stt-a",
				LLMProviderID: "llm-a",
				TTSProviderID: "tts-a",
				Steps: []liveProviderChainStepEntry{
					{Output: liveProviderChainStepOutput{Attempts: []liveProviderChainStepAttempt{{StreamingUsed: false}}}},
				},
				Latency: liveProviderChainLatency{
					FirstAssistantAudioE2EMS: 1400,
					TurnCompletionE2EMS:      2100,
				},
			},
			{
				Status:        "pass",
				STTProviderID: "stt-b",
				LLMProviderID: "llm-b",
				TTSProviderID: "tts-b",
				Steps: []liveProviderChainStepEntry{
					{Output: liveProviderChainStepOutput{Attempts: []liveProviderChainStepAttempt{{StreamingUsed: false}}}},
				},
				Latency: liveProviderChainLatency{
					FirstAssistantAudioE2EMS: 1100,
					TurnCompletionE2EMS:      1700,
				},
			},
		},
	}

	streamingRaw, err := json.Marshal(streaming)
	if err != nil {
		t.Fatalf("marshal streaming report: %v", err)
	}
	nonStreamingRaw, err := json.Marshal(nonStreaming)
	if err != nil {
		t.Fatalf("marshal non-streaming report: %v", err)
	}
	if err := osWriteFile(streamingPath, streamingRaw); err != nil {
		t.Fatalf("write streaming report: %v", err)
	}
	if err := osWriteFile(nonStreamingPath, nonStreamingRaw); err != nil {
		t.Fatalf("write non-streaming report: %v", err)
	}

	if err := writeLiveLatencyCompareReport(outputPath, streamingPath, nonStreamingPath); err != nil {
		t.Fatalf("expected compare report generation to pass, got %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read compare report: %v", err)
	}
	var artifact liveLatencyCompareArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode compare report: %v", err)
	}
	if artifact.PairCount != 2 {
		t.Fatalf("expected pair count 2, got %d", artifact.PairCount)
	}
	if len(artifact.Combos) != 2 {
		t.Fatalf("expected two combo rows, got %d", len(artifact.Combos))
	}
	if artifact.Aggregate.FirstAudio.Winner != "streaming" {
		t.Fatalf("expected first-audio winner streaming, got %q", artifact.Aggregate.FirstAudio.Winner)
	}
	if artifact.Aggregate.Completion.Winner != "non_streaming" {
		t.Fatalf("expected completion winner non_streaming, got %q", artifact.Aggregate.Completion.Winner)
	}

	summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
	if _, err := os.Stat(summaryPath); err != nil {
		t.Fatalf("expected markdown compare artifact, got %v", err)
	}
}

func TestWriteLiveLatencyCompareReportRejectsStreamingWithoutOverlapEvidence(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	streamingPath := filepath.Join(tmp, "streaming.json")
	nonStreamingPath := filepath.Join(tmp, "nonstreaming.json")
	outputPath := filepath.Join(tmp, "live-latency-compare.json")

	streaming := liveProviderChainArtifact{
		ExecutionMode:      "streaming",
		ComparisonIdentity: "cmp-b",
		Status:             "pass",
		Combinations: []liveProviderChainCombinationEntry{
			{
				Status:        "pass",
				STTProviderID: "stt-a",
				LLMProviderID: "llm-a",
				TTSProviderID: "tts-a",
				Latency:       liveProviderChainLatency{TurnCompletionE2EMS: 2000},
			},
		},
	}
	nonStreaming := liveProviderChainArtifact{
		ExecutionMode:      "non_streaming",
		ComparisonIdentity: "cmp-b",
		Status:             "pass",
		Combinations: []liveProviderChainCombinationEntry{
			{
				Status:        "pass",
				STTProviderID: "stt-a",
				LLMProviderID: "llm-a",
				TTSProviderID: "tts-a",
				Steps: []liveProviderChainStepEntry{
					{Output: liveProviderChainStepOutput{Attempts: []liveProviderChainStepAttempt{{StreamingUsed: false}}}},
				},
				Latency: liveProviderChainLatency{TurnCompletionE2EMS: 1900},
			},
		},
	}

	streamingRaw, _ := json.Marshal(streaming)
	nonStreamingRaw, _ := json.Marshal(nonStreaming)
	if err := osWriteFile(streamingPath, streamingRaw); err != nil {
		t.Fatalf("write streaming report: %v", err)
	}
	if err := osWriteFile(nonStreamingPath, nonStreamingRaw); err != nil {
		t.Fatalf("write non-streaming report: %v", err)
	}

	err := writeLiveLatencyCompareReport(outputPath, streamingPath, nonStreamingPath)
	if err == nil {
		t.Fatalf("expected compare report generation to fail when streaming overlap evidence is missing")
	}
	if !strings.Contains(err.Error(), "missing streaming overlap evidence") {
		t.Fatalf("expected overlap-evidence failure, got %v", err)
	}
}

func TestValidateLiveProviderModeEvidenceRejectsStreamingAttemptsInNonStreamingMode(t *testing.T) {
	t.Parallel()

	report := liveProviderChainArtifact{
		ExecutionMode: "non_streaming",
		Combinations: []liveProviderChainCombinationEntry{
			{
				Status:        "pass",
				STTProviderID: "stt-a",
				LLMProviderID: "llm-a",
				TTSProviderID: "tts-a",
				Steps: []liveProviderChainStepEntry{
					{Output: liveProviderChainStepOutput{Attempts: []liveProviderChainStepAttempt{{StreamingUsed: true}}}},
				},
			},
		},
	}

	violations := validateLiveProviderModeEvidence(report)
	if len(violations) == 0 {
		t.Fatalf("expected non-streaming mode evidence violation")
	}
	if !hasViolationContaining(violations, "streaming_used=true") {
		t.Fatalf("expected streaming_used violation, got %+v", violations)
	}
}

func TestValidateSimpleModeOnly(t *testing.T) {
	t.Parallel()

	if err := validateSimpleModeOnly(); err != nil {
		t.Fatalf("expected simple-mode validation to pass, got %v", err)
	}
}

func TestWriteContractsReport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	outputPath := filepath.Join(tmp, "contracts-report.json")
	fixtureRoot := filepath.Join("test", "contract", "fixtures")

	if err := writeContractsReport(outputPath, fixtureRoot); err != nil {
		t.Fatalf("expected contracts report generation to pass, got %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("unexpected contracts report read error: %v", err)
	}
	var report contractsReportArtifact
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("unexpected contracts report decode error: %v", err)
	}
	if !report.Passed || report.Summary.Failed != 0 || report.Summary.Total == 0 {
		t.Fatalf("unexpected contracts report content: %+v", report)
	}

	summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
	if _, err := os.Stat(summaryPath); err != nil {
		t.Fatalf("expected contracts summary markdown artifact, got %v", err)
	}
}

func TestWriteReleaseManifest(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	now := time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
	outputPath := filepath.Join(tmp, "release-manifest.json")
	rolloutCfgPath := filepath.Join(tmp, "rollout.json")
	contractsPath := filepath.Join(tmp, "contracts.json")
	replayPath := filepath.Join(tmp, "replay.json")
	sloPath := filepath.Join(tmp, "slo.json")

	if err := osWriteFile(rolloutCfgPath, []byte(`{
  "pipeline_version": "pipeline-v2",
  "strategy": "canary",
  "rollback_posture": {
    "mode": "automatic",
    "trigger": "replay_or_slo_failure"
  }
}`)); err != nil {
		t.Fatalf("unexpected rollout config write error: %v", err)
	}
	if err := osWriteFile(contractsPath, []byte(`{"generated_at_utc":"2026-02-11T11:00:00Z","passed":true}`)); err != nil {
		t.Fatalf("unexpected contracts artifact write error: %v", err)
	}
	if err := osWriteFile(replayPath, []byte(`{"generated_at_utc":"2026-02-11T11:10:00Z","failing_count":0}`)); err != nil {
		t.Fatalf("unexpected replay artifact write error: %v", err)
	}
	if err := osWriteFile(sloPath, []byte(`{"generated_at_utc":"2026-02-11T11:20:00Z","report":{"passed":true}}`)); err != nil {
		t.Fatalf("unexpected slo artifact write error: %v", err)
	}

	manifest, err := writeReleaseManifest(
		outputPath,
		"specs/pipeline-v2.json",
		rolloutCfgPath,
		contractsPath,
		replayPath,
		sloPath,
		now,
	)
	if err != nil {
		t.Fatalf("expected release manifest generation to pass, got %v", err)
	}
	if !manifest.Readiness.Passed {
		t.Fatalf("expected readiness pass in release manifest, got %+v", manifest.Readiness)
	}
	if manifest.RolloutConfig.PipelineVersion != "pipeline-v2" {
		t.Fatalf("unexpected release manifest rollout config: %+v", manifest.RolloutConfig)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("unexpected release manifest read error: %v", err)
	}
	var artifact toolingrelease.ReleaseManifest
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("unexpected release manifest decode error: %v", err)
	}
	if artifact.ReleaseID == "" || len(artifact.SourceArtifacts) != 4 {
		t.Fatalf("unexpected release manifest artifact: %+v", artifact)
	}
}

func TestWriteReleaseManifestFailsWhenReadinessFails(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	now := time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
	outputPath := filepath.Join(tmp, "release-manifest.json")
	rolloutCfgPath := filepath.Join(tmp, "rollout.json")
	contractsPath := filepath.Join(tmp, "contracts.json")
	replayPath := filepath.Join(tmp, "replay.json")
	sloPath := filepath.Join(tmp, "slo.json")

	if err := osWriteFile(rolloutCfgPath, []byte(`{
  "pipeline_version": "pipeline-v2",
  "strategy": "canary",
  "rollback_posture": {
    "mode": "automatic",
    "trigger": "replay_or_slo_failure"
  }
}`)); err != nil {
		t.Fatalf("unexpected rollout config write error: %v", err)
	}
	if err := osWriteFile(contractsPath, []byte(`{"generated_at_utc":"2026-02-11T11:00:00Z","passed":true}`)); err != nil {
		t.Fatalf("unexpected contracts artifact write error: %v", err)
	}
	if err := osWriteFile(replayPath, []byte(`{"generated_at_utc":"2026-02-11T11:10:00Z","failing_count":2}`)); err != nil {
		t.Fatalf("unexpected replay artifact write error: %v", err)
	}
	if err := osWriteFile(sloPath, []byte(`{"generated_at_utc":"2026-02-11T11:20:00Z","report":{"passed":true}}`)); err != nil {
		t.Fatalf("unexpected slo artifact write error: %v", err)
	}

	if _, err := writeReleaseManifest(
		outputPath,
		"specs/pipeline-v2.json",
		rolloutCfgPath,
		contractsPath,
		replayPath,
		sloPath,
		now,
	); err == nil {
		t.Fatalf("expected release manifest generation to fail when readiness fails")
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func hasViolationContaining(violations []string, needle string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, needle) {
			return true
		}
	}
	return false
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
