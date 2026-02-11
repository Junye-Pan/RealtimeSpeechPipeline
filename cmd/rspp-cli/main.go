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
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	replaycmp "github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/ops"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/regression"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/validation"
)

const (
	replaySmokeTimingToleranceMS       int64 = 15
	replaySmokeFixtureID                     = "rd-001-smoke"
	replayRegressionDefaultGate              = "full"
	defaultReplayMetadataPath                = "test/replay/fixtures/metadata.json"
	defaultReplayRegressionReportPath        = ".codex/replay/regression-report.json"
	replayFixtureReportsDirName              = "fixtures"
	defaultRuntimeBaselineArtifactPath       = ".codex/replay/runtime-baseline.json"
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
		metadataPath := defaultReplayMetadataPath
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if len(os.Args) >= 4 {
			metadataPath = os.Args[3]
		}
		if err := writeReplaySmokeReport(outputPath, metadataPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write replay smoke report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("replay smoke report written: %s\n", outputPath)
		fmt.Printf("replay smoke summary written: %s\n", summaryPath)
	case "replay-regression-report":
		outputPath := defaultReplayRegressionReportPath
		metadataPath := defaultReplayMetadataPath
		gate := replayRegressionDefaultGate
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if len(os.Args) >= 4 {
			metadataPath = os.Args[3]
		}
		if len(os.Args) >= 5 {
			gate = os.Args[4]
		}
		if err := writeReplayRegressionReport(outputPath, metadataPath, gate); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write replay regression report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("replay regression report written: %s\n", outputPath)
		fmt.Printf("replay regression summary written: %s\n", summaryPath)
	case "generate-runtime-baseline":
		outputPath := defaultRuntimeBaselineArtifactPath
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if err := writeRuntimeBaselineArtifact(outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate runtime baseline artifact: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("runtime baseline artifact written: %s\n", outputPath)
	case "slo-gates-report":
		outputPath := filepath.Join(".codex", "ops", "slo-gates-report.json")
		baselineArtifactPath := defaultRuntimeBaselineArtifactPath
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if len(os.Args) >= 4 {
			baselineArtifactPath = os.Args[3]
		}
		if err := writeSLOGatesReport(outputPath, baselineArtifactPath); err != nil {
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
	fmt.Println("  rspp-cli replay-smoke-report [output_path] [metadata_path]")
	fmt.Println("  rspp-cli replay-regression-report [output_path] [metadata_path] [gate]")
	fmt.Println("  rspp-cli generate-runtime-baseline [output_path]")
	fmt.Println("  rspp-cli slo-gates-report [output_path] [baseline_artifact_path]")
}

type replaySmokeReport struct {
	GeneratedAtUTC     string                 `json:"generated_at_utc"`
	FixtureID          string                 `json:"fixture_id"`
	MetadataPath       string                 `json:"metadata_path"`
	TimingToleranceMS  int64                  `json:"timing_tolerance_ms"`
	TotalDivergences   int                    `json:"total_divergences"`
	ByClass            map[string]int         `json:"by_class"`
	Divergences        []obs.ReplayDivergence `json:"divergences"`
	FailingCount       int                    `json:"failing_count"`
	UnexplainedCount   int                    `json:"unexplained_count"`
	MissingExpected    int                    `json:"missing_expected"`
	ExpectedConfigured int                    `json:"expected_configured"`
	FailingDivergences []string               `json:"failing_divergences"`
}

func writeReplaySmokeReport(outputPath string, metadataPath string) error {
	policy, effectiveTimingToleranceMS, err := loadReplayFixturePolicy(metadataPath, replaySmokeFixtureID, replaySmokeTimingToleranceMS)
	if err != nil {
		return err
	}
	divergences := buildReplaySmokeDivergences(effectiveTimingToleranceMS)
	evaluation := regression.EvaluateDivergences(divergences, policy)

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
		FixtureID:          replaySmokeFixtureID,
		MetadataPath:       metadataPath,
		TimingToleranceMS:  effectiveTimingToleranceMS,
		TotalDivergences:   len(divergences),
		ByClass:            byClass,
		Divergences:        divergences,
		FailingCount:       len(evaluation.Failing),
		UnexplainedCount:   len(evaluation.Unexplained),
		MissingExpected:    len(evaluation.MissingExpected),
		ExpectedConfigured: len(policy.Expected),
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

type replayFixtureMetadata struct {
	Fixtures map[string]replayFixturePolicy `json:"fixtures"`
}

type replayFixturePolicy struct {
	Gate                              string                          `json:"gate,omitempty"`
	TimingToleranceMS                 *int64                          `json:"timing_tolerance_ms,omitempty"`
	FinalAttemptLatencyThresholdMS    *int64                          `json:"final_attempt_latency_threshold_ms,omitempty"`
	TotalInvocationLatencyThresholdMS *int64                          `json:"total_invocation_latency_threshold_ms,omitempty"`
	InvocationLatencyScopes           []string                        `json:"invocation_latency_scopes,omitempty"`
	ExpectedDivergences               []regression.ExpectedDivergence `json:"expected_divergences,omitempty"`
}

func loadReplayFixturePolicy(metadataPath string, fixtureID string, defaultTimingToleranceMS int64) (regression.DivergencePolicy, int64, error) {
	metadata, err := loadReplayFixtureMetadata(metadataPath)
	if err != nil {
		return regression.DivergencePolicy{}, 0, err
	}

	fixturePolicy, ok := metadata.Fixtures[fixtureID]
	if !ok {
		return regression.DivergencePolicy{}, 0, fmt.Errorf("fixture %s not found in metadata %s", fixtureID, metadataPath)
	}

	timingToleranceMS := fixtureTimingTolerance(fixturePolicy, defaultTimingToleranceMS)

	policy := regression.DivergencePolicy{
		TimingToleranceMS: timingToleranceMS,
		Expected:          fixturePolicy.ExpectedDivergences,
	}
	return policy, timingToleranceMS, nil
}

func loadReplayFixtureMetadata(metadataPath string) (replayFixtureMetadata, error) {
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		return replayFixtureMetadata{}, fmt.Errorf("read replay fixture metadata %s: %w", metadataPath, err)
	}
	var metadata replayFixtureMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return replayFixtureMetadata{}, fmt.Errorf("decode replay fixture metadata %s: %w", metadataPath, err)
	}
	if len(metadata.Fixtures) == 0 {
		return replayFixtureMetadata{}, fmt.Errorf("replay fixture metadata %s contains no fixtures", metadataPath)
	}
	return metadata, nil
}

func fixtureTimingTolerance(policy replayFixturePolicy, defaultTimingToleranceMS int64) int64 {
	timingToleranceMS := defaultTimingToleranceMS
	if policy.TimingToleranceMS != nil {
		timingToleranceMS = *policy.TimingToleranceMS
	}
	if timingToleranceMS < 0 {
		timingToleranceMS = 0
	}
	return timingToleranceMS
}

type replayFixtureExecutionReport struct {
	FixtureID                         string         `json:"fixture_id"`
	Gate                              string         `json:"gate"`
	TimingToleranceMS                 int64          `json:"timing_tolerance_ms"`
	FinalAttemptLatencyThresholdMS    *int64         `json:"final_attempt_latency_threshold_ms,omitempty"`
	TotalInvocationLatencyThresholdMS *int64         `json:"total_invocation_latency_threshold_ms,omitempty"`
	InvocationLatencyBreaches         int            `json:"invocation_latency_breaches,omitempty"`
	TotalDivergences                  int            `json:"total_divergences"`
	FailingCount                      int            `json:"failing_count"`
	UnexplainedCount                  int            `json:"unexplained_count"`
	MissingExpected                   int            `json:"missing_expected"`
	ExpectedConfigured                int            `json:"expected_configured"`
	ByClass                           map[string]int `json:"by_class"`
	FailingClasses                    []string       `json:"failing_classes,omitempty"`
}

type replayFixtureArtifact struct {
	GeneratedAtUTC string `json:"generated_at_utc"`
	MetadataPath   string `json:"metadata_path"`
	replayFixtureExecutionReport
	Status string `json:"status"`
}

type replayRegressionReport struct {
	GeneratedAtUTC     string                         `json:"generated_at_utc"`
	Gate               string                         `json:"gate"`
	MetadataPath       string                         `json:"metadata_path"`
	FixtureCount       int                            `json:"fixture_count"`
	TotalDivergences   int                            `json:"total_divergences"`
	FailingCount       int                            `json:"failing_count"`
	UnexplainedCount   int                            `json:"unexplained_count"`
	MissingExpected    int                            `json:"missing_expected"`
	ByClass            map[string]int                 `json:"by_class"`
	FailingDivergences []string                       `json:"failing_divergences"`
	Fixtures           []replayFixtureExecutionReport `json:"fixtures"`
}

type replayFixtureBuilder func(timingToleranceMS int64) []obs.ReplayDivergence

type invocationLatencySample struct {
	Scope                    string
	FinalAttemptLatencyMS    int64
	TotalInvocationLatencyMS int64
}

func writeReplayRegressionReport(outputPath string, metadataPath string, gate string) error {
	normalizedGate := strings.ToLower(strings.TrimSpace(gate))
	if normalizedGate == "" {
		normalizedGate = replayRegressionDefaultGate
	}
	if normalizedGate != "quick" && normalizedGate != "full" {
		return fmt.Errorf("unsupported replay regression gate %q (expected quick|full)", gate)
	}

	metadata, err := loadReplayFixtureMetadata(metadataPath)
	if err != nil {
		return err
	}
	fixtureIDs, err := selectReplayFixtureIDs(metadata, normalizedGate)
	if err != nil {
		return err
	}
	builders := replayFixtureBuilders()

	fixtureReports := make([]replayFixtureExecutionReport, 0, len(fixtureIDs))
	totalByClass := map[string]int{
		string(obs.PlanDivergence):      0,
		string(obs.OutcomeDivergence):   0,
		string(obs.OrderingDivergence):  0,
		string(obs.AuthorityDivergence): 0,
		string(obs.TimingDivergence):    0,
	}
	latencySamplesByScope, latencySamplesErr := runtimeBaselineInvocationLatencySamplesForReplay()

	failingEntries := make([]obs.ReplayDivergence, 0)
	totalDivergences := 0
	totalFailing := 0
	totalUnexplained := 0
	totalMissingExpected := 0

	for _, fixtureID := range fixtureIDs {
		policy := metadata.Fixtures[fixtureID]
		builder, ok := builders[fixtureID]
		if !ok {
			return fmt.Errorf("no replay fixture builder registered for %s", fixtureID)
		}

		timingToleranceMS := fixtureTimingTolerance(policy, replaySmokeTimingToleranceMS)
		divergences := builder(timingToleranceMS)
		latencyThresholdDivergences := buildInvocationLatencyThresholdDivergences(fixtureID, policy, latencySamplesByScope, latencySamplesErr)
		divergences = append(divergences, latencyThresholdDivergences...)
		evaluation := regression.EvaluateDivergences(divergences, regression.DivergencePolicy{
			TimingToleranceMS: timingToleranceMS,
			Expected:          policy.ExpectedDivergences,
		})

		byClass := map[string]int{
			string(obs.PlanDivergence):      0,
			string(obs.OutcomeDivergence):   0,
			string(obs.OrderingDivergence):  0,
			string(obs.AuthorityDivergence): 0,
			string(obs.TimingDivergence):    0,
		}
		for _, entry := range divergences {
			byClass[string(entry.Class)]++
			totalByClass[string(entry.Class)]++
		}

		report := replayFixtureExecutionReport{
			FixtureID:                         fixtureID,
			Gate:                              normalizedGate,
			TimingToleranceMS:                 timingToleranceMS,
			FinalAttemptLatencyThresholdMS:    normalizeNonNegativeThreshold(policy.FinalAttemptLatencyThresholdMS),
			TotalInvocationLatencyThresholdMS: normalizeNonNegativeThreshold(policy.TotalInvocationLatencyThresholdMS),
			InvocationLatencyBreaches:         len(latencyThresholdDivergences),
			TotalDivergences:                  len(divergences),
			FailingCount:                      len(evaluation.Failing),
			UnexplainedCount:                  len(evaluation.Unexplained),
			MissingExpected:                   len(evaluation.MissingExpected),
			ExpectedConfigured:                len(policy.ExpectedDivergences),
			ByClass:                           byClass,
			FailingClasses:                    uniqueFailingClasses(evaluation.Failing),
		}
		fixtureReports = append(fixtureReports, report)

		totalDivergences += len(divergences)
		totalFailing += len(evaluation.Failing)
		totalUnexplained += len(evaluation.Unexplained)
		totalMissingExpected += len(evaluation.MissingExpected)
		failingEntries = append(failingEntries, evaluation.Failing...)
	}

	summary := replayRegressionReport{
		GeneratedAtUTC:     time.Now().UTC().Format(time.RFC3339),
		Gate:               normalizedGate,
		MetadataPath:       metadataPath,
		FixtureCount:       len(fixtureReports),
		TotalDivergences:   totalDivergences,
		FailingCount:       totalFailing,
		UnexplainedCount:   totalUnexplained,
		MissingExpected:    totalMissingExpected,
		ByClass:            totalByClass,
		FailingDivergences: uniqueFailingClasses(failingEntries),
		Fixtures:           fixtureReports,
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}
	summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
	if err := os.WriteFile(summaryPath, []byte(renderReplayRegressionSummary(summary)), 0o644); err != nil {
		return err
	}

	fixtureOutputDir := filepath.Join(filepath.Dir(outputPath), replayFixtureReportsDirName)
	if err := writeReplayFixtureArtifacts(fixtureOutputDir, summary.GeneratedAtUTC, metadataPath, fixtureReports); err != nil {
		return err
	}

	if summary.FailingCount > 0 {
		return fmt.Errorf("replay regression gate failed: %v", summary.FailingDivergences)
	}
	return nil
}

func writeReplayFixtureArtifacts(outputDir string, generatedAtUTC string, metadataPath string, reports []replayFixtureExecutionReport) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create replay fixture artifact directory %s: %w", outputDir, err)
	}

	for _, report := range reports {
		artifact := replayFixtureArtifact{
			GeneratedAtUTC:               generatedAtUTC,
			MetadataPath:                 metadataPath,
			replayFixtureExecutionReport: report,
			Status:                       replayFixtureStatus(report),
		}
		filename := sanitizeFixtureFilename(report.FixtureID)
		jsonPath := filepath.Join(outputDir, filename+".json")
		markdownPath := filepath.Join(outputDir, filename+".md")

		data, err := json.MarshalIndent(artifact, "", "  ")
		if err != nil {
			return fmt.Errorf("encode replay fixture artifact %s: %w", report.FixtureID, err)
		}
		if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
			return fmt.Errorf("write replay fixture artifact %s: %w", jsonPath, err)
		}
		if err := os.WriteFile(markdownPath, []byte(renderReplayFixtureSummary(artifact)), 0o644); err != nil {
			return fmt.Errorf("write replay fixture summary %s: %w", markdownPath, err)
		}
	}
	return nil
}

func replayFixtureStatus(report replayFixtureExecutionReport) string {
	if report.FailingCount == 0 {
		return "PASS"
	}
	return "FAIL"
}

func sanitizeFixtureFilename(fixtureID string) string {
	trimmed := strings.TrimSpace(fixtureID)
	if trimmed == "" {
		return "fixture"
	}
	return strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		" ", "_",
	).Replace(trimmed)
}

func selectReplayFixtureIDs(metadata replayFixtureMetadata, gate string) ([]string, error) {
	ids := make([]string, 0, len(metadata.Fixtures))
	for fixtureID, policy := range metadata.Fixtures {
		if !isFixtureEnabledForGate(policy, gate) {
			continue
		}
		ids = append(ids, fixtureID)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no replay fixtures configured for gate=%s", gate)
	}
	sort.Strings(ids)
	return ids, nil
}

func isFixtureEnabledForGate(policy replayFixturePolicy, gate string) bool {
	declared := strings.ToLower(strings.TrimSpace(policy.Gate))
	if declared == "" {
		declared = "full"
	}
	if declared == "both" {
		return gate == "quick" || gate == "full"
	}
	return declared == gate
}

func replayFixtureBuilders() map[string]replayFixtureBuilder {
	return map[string]replayFixtureBuilder{
		"ae-001-preturn-stale-epoch":           buildReplayNoDivergence,
		"ae-002-inturn-authority-revoke":       buildReplayNoDivergence,
		"ae-003-old-placement-stale-output":    buildReplayNoDivergence,
		"ae-004-ingress-authority-enrichment":  buildReplayNoDivergence,
		"ae-005-preturn-deauthorization":       buildReplayNoDivergence,
		"cf-001-cancel-fence":                  buildReplayNoDivergence,
		"cf-002-provider-late-output":          buildReplayNoDivergence,
		"cf-003-cancel-terminalization":        buildReplayNoDivergence,
		"cf-004-cancel-observability":          buildReplayNoDivergence,
		"f1-admission-overload":                buildReplayNoDivergence,
		"f2-node-timeout-failure":              buildReplayNoDivergence,
		"f3-provider-failure":                  buildReplayNoDivergence,
		"f4-edge-pressure-overflow":            buildReplayNoDivergence,
		"f5-sync-coupled-loss":                 buildReplayNoDivergence,
		"f6-transport-disconnect-stall":        buildReplayNoDivergence,
		"f7-authority-conflict":                buildReplayNoDivergence,
		"f8-region-failover":                   buildReplayNoDivergence,
		"ml-001-drop-under-pressure":           buildReplayNoDivergence,
		"ml-002-deterministic-merge":           buildReplayNoDivergence,
		"ml-003-replay-absence-classification": buildReplayML003OutcomeDivergence,
		"ml-004-sync-discontinuity":            buildReplayNoDivergence,
		"rd-001-smoke":                         buildReplaySmokeDivergences,
		"rd-002-recompute-within-tolerance":    buildReplayTimingDivergenceWithinTolerance,
		"rd-003-baseline-completeness":         buildReplayNoDivergence,
		"rd-004-snapshot-provenance-plan":      buildReplayPlanDivergence,
		"rd-ordering-approved-1":               buildReplayOrderingDivergence,
	}
}

func buildInvocationLatencyThresholdDivergences(
	fixtureID string,
	policy replayFixturePolicy,
	samplesByScope map[string]invocationLatencySample,
	samplesErr error,
) []obs.ReplayDivergence {
	finalThreshold := normalizeNonNegativeThreshold(policy.FinalAttemptLatencyThresholdMS)
	totalThreshold := normalizeNonNegativeThreshold(policy.TotalInvocationLatencyThresholdMS)
	if finalThreshold == nil && totalThreshold == nil {
		return nil
	}

	if samplesErr != nil {
		return appendMissingInvocationLatencyEvidenceDivergences(nil, fixtureIDScope(fixtureID), finalThreshold, totalThreshold, fmt.Sprintf("runtime baseline latency extraction failed: %v", samplesErr))
	}

	scopes := invocationLatencyScopesForFixture(fixtureID, policy)
	if len(scopes) == 0 {
		return appendMissingInvocationLatencyEvidenceDivergences(nil, fixtureIDScope(fixtureID), finalThreshold, totalThreshold, "latency evidence scope could not be derived from fixture id")
	}

	divergences := make([]obs.ReplayDivergence, 0)
	for _, scope := range scopes {
		sample, ok := samplesByScope[scope]
		if !ok {
			divergences = appendMissingInvocationLatencyEvidenceDivergences(divergences, scope, finalThreshold, totalThreshold, "runtime baseline artifact lacks invocation latency evidence for scope")
			continue
		}
		if finalThreshold != nil && sample.FinalAttemptLatencyMS > *finalThreshold {
			diff := sample.FinalAttemptLatencyMS - *finalThreshold
			divergences = append(divergences, obs.ReplayDivergence{
				Class:   obs.TimingDivergence,
				Scope:   "invocation_latency_final:" + scope,
				Message: fmt.Sprintf("final attempt latency threshold exceeded: observed=%d threshold=%d", sample.FinalAttemptLatencyMS, *finalThreshold),
				DiffMS:  &diff,
			})
		}
		if totalThreshold != nil && sample.TotalInvocationLatencyMS > *totalThreshold {
			diff := sample.TotalInvocationLatencyMS - *totalThreshold
			divergences = append(divergences, obs.ReplayDivergence{
				Class:   obs.TimingDivergence,
				Scope:   "invocation_latency_total:" + scope,
				Message: fmt.Sprintf("total invocation latency threshold exceeded: observed=%d threshold=%d", sample.TotalInvocationLatencyMS, *totalThreshold),
				DiffMS:  &diff,
			})
		}
	}
	return divergences
}

func runtimeBaselineInvocationLatencySamplesForReplay() (map[string]invocationLatencySample, error) {
	tempDir, err := os.MkdirTemp("", "rspp-replay-runtime-baseline-*")
	if err != nil {
		return nil, fmt.Errorf("create runtime baseline temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	path := filepath.Join(tempDir, "runtime-baseline.json")
	entries, err := generateRuntimeBaselineArtifact(path)
	if err != nil {
		return nil, fmt.Errorf("generate runtime baseline artifact: %w", err)
	}
	return invocationLatencySamplesFromBaselineEntries(entries), nil
}

func invocationLatencySamplesFromBaselineEntries(entries []timeline.BaselineEvidence) map[string]invocationLatencySample {
	samples := make(map[string]invocationLatencySample)
	for _, entry := range entries {
		if entry.TurnID == "" || len(entry.InvocationOutcomes) == 0 {
			continue
		}
		scope := "turn:" + entry.TurnID
		current, hasCurrent := samples[scope]
		if !hasCurrent {
			current = invocationLatencySample{Scope: scope}
		}
		for _, outcome := range entry.InvocationOutcomes {
			if outcome.FinalAttemptLatencyMS > current.FinalAttemptLatencyMS {
				current.FinalAttemptLatencyMS = outcome.FinalAttemptLatencyMS
			}
			if outcome.TotalInvocationLatencyMS > current.TotalInvocationLatencyMS {
				current.TotalInvocationLatencyMS = outcome.TotalInvocationLatencyMS
			}
		}
		samples[scope] = current
	}
	return samples
}

func invocationLatencyScopesForFixture(fixtureID string, policy replayFixturePolicy) []string {
	metadataScopes := normalizeInvocationLatencyScopes(policy.InvocationLatencyScopes)
	if len(metadataScopes) > 0 {
		return metadataScopes
	}

	scope := derivedInvocationLatencyScopeForFixture(fixtureID)
	if scope == "" {
		return nil
	}
	return []string{scope}
}

func normalizeInvocationLatencyScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		scope := strings.TrimSpace(raw)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	sort.Strings(normalized)
	return normalized
}

func derivedInvocationLatencyScopeForFixture(fixtureID string) string {
	parts := strings.Split(fixtureID, "-")
	if len(parts) < 2 || !isDigits(parts[1]) {
		return ""
	}
	return "turn:turn-" + parts[0] + "-" + parts[1]
}

func fixtureIDScope(fixtureID string) string {
	return "fixture:" + strings.TrimSpace(fixtureID)
}

func appendMissingInvocationLatencyEvidenceDivergences(
	divergences []obs.ReplayDivergence,
	scope string,
	finalThreshold *int64,
	totalThreshold *int64,
	reason string,
) []obs.ReplayDivergence {
	if finalThreshold != nil {
		divergences = append(divergences, obs.ReplayDivergence{
			Class:   obs.TimingDivergence,
			Scope:   "invocation_latency_final:" + scope,
			Message: fmt.Sprintf("invocation latency evidence missing: %s", reason),
		})
	}
	if totalThreshold != nil {
		divergences = append(divergences, obs.ReplayDivergence{
			Class:   obs.TimingDivergence,
			Scope:   "invocation_latency_total:" + scope,
			Message: fmt.Sprintf("invocation latency evidence missing: %s", reason),
		})
	}
	return divergences
}

func isDigits(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func buildReplayNoDivergence(timingToleranceMS int64) []obs.ReplayDivergence {
	return buildReplaySmokeDivergences(timingToleranceMS)
}

func buildReplayTimingDivergenceWithinTolerance(timingToleranceMS int64) []obs.ReplayDivergence {
	decision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          "sess-rd-002",
		TurnID:             "turn-rd-002",
		EventID:            "evt-rd-002",
		RuntimeTimestampMS: 100,
		WallClockMS:        100,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
	baseline := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-rd-002",
		SnapshotProvenanceRef: "snapshot-rd-002",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:100",
		AuthorityEpoch:        7,
		RuntimeTimestampMS:    100,
	}}
	replayed := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-rd-002",
		SnapshotProvenanceRef: "snapshot-rd-002",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:100",
		AuthorityEpoch:        7,
		RuntimeTimestampMS:    112,
	}}
	return replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: timingToleranceMS})
}

func buildReplayPlanDivergence(timingToleranceMS int64) []obs.ReplayDivergence {
	decision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          "sess-rd-004",
		TurnID:             "turn-rd-004",
		EventID:            "evt-rd-004",
		RuntimeTimestampMS: 100,
		WallClockMS:        100,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
	baseline := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-rd-004",
		SnapshotProvenanceRef: "snapshot-a",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:200",
		AuthorityEpoch:        9,
		RuntimeTimestampMS:    100,
	}}
	replayed := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-rd-004",
		SnapshotProvenanceRef: "snapshot-b",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:200",
		AuthorityEpoch:        9,
		RuntimeTimestampMS:    100,
	}}
	return replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: timingToleranceMS})
}

func buildReplayOrderingDivergence(timingToleranceMS int64) []obs.ReplayDivergence {
	decision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          "sess-ordering-1",
		TurnID:             "turn-ordering-approved-1",
		EventID:            "evt-ordering-1",
		RuntimeTimestampMS: 300,
		WallClockMS:        300,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
	baseline := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-ordering-1",
		SnapshotProvenanceRef: "snapshot-ordering-1",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:300",
		AuthorityEpoch:        11,
		RuntimeTimestampMS:    300,
	}}
	replayed := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-ordering-1",
		SnapshotProvenanceRef: "snapshot-ordering-1",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:301",
		AuthorityEpoch:        11,
		RuntimeTimestampMS:    300,
	}}
	return replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: timingToleranceMS})
}

func buildReplayML003OutcomeDivergence(_ int64) []obs.ReplayDivergence {
	baseline := []replaycmp.LineageRecord{
		{EventID: "evt-ml003-drop", Dropped: true, MergeGroupID: ""},
		{EventID: "evt-ml003-merge", Dropped: false, MergeGroupID: "merge-ml003"},
	}
	replayed := []replaycmp.LineageRecord{
		{EventID: "evt-ml003-drop", Dropped: true, MergeGroupID: ""},
	}
	return replaycmp.CompareLineageRecords(baseline, replayed)
}

func renderReplayRegressionSummary(report replayRegressionReport) string {
	lines := []string{
		"# Replay Regression Report",
		"",
		"Generated at (UTC): " + report.GeneratedAtUTC,
		"Gate: " + report.Gate,
		"Metadata path: " + report.MetadataPath,
		fmt.Sprintf("Fixtures evaluated: %d", report.FixtureCount),
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

func renderReplayFixtureSummary(report replayFixtureArtifact) string {
	lines := []string{
		"# Replay Fixture Report",
		"",
		"Generated at (UTC): " + report.GeneratedAtUTC,
		"Fixture: " + report.FixtureID,
		"Gate: " + report.Gate,
		"Metadata path: " + report.MetadataPath,
		fmt.Sprintf("Timing tolerance (ms): %d", report.TimingToleranceMS),
		renderThresholdSummary("Final-attempt latency threshold (ms)", report.FinalAttemptLatencyThresholdMS),
		renderThresholdSummary("Total-invocation latency threshold (ms)", report.TotalInvocationLatencyThresholdMS),
		fmt.Sprintf("Invocation latency threshold breaches: %d", report.InvocationLatencyBreaches),
		fmt.Sprintf("Total divergences: %d", report.TotalDivergences),
		fmt.Sprintf("Failing divergences: %d", report.FailingCount),
		fmt.Sprintf("Unexplained divergences: %d", report.UnexplainedCount),
		fmt.Sprintf("Missing expected divergences: %d", report.MissingExpected),
		fmt.Sprintf("Expected divergences configured: %d", report.ExpectedConfigured),
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

	if report.Status == "PASS" {
		lines = append(lines, "", "Status: PASS")
	} else if len(report.FailingClasses) == 0 {
		lines = append(lines, "", "Status: FAIL")
	} else {
		lines = append(lines, "", "Status: FAIL", "- Forbidden divergences: "+strings.Join(report.FailingClasses, ", "))
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderThresholdSummary(label string, threshold *int64) string {
	if threshold == nil {
		return label + ": unset"
	}
	return fmt.Sprintf("%s: %d", label, *threshold)
}

func buildReplaySmokeDivergences(timingToleranceMS int64) []obs.ReplayDivergence {
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
	return replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: timingToleranceMS})
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

func normalizeNonNegativeThreshold(in *int64) *int64 {
	if in == nil {
		return nil
	}
	value := *in
	if value < 0 {
		value = 0
	}
	return &value
}

func renderReplaySmokeSummary(report replaySmokeReport) string {
	lines := []string{
		"# Replay Smoke Divergence Report",
		"",
		"Generated at (UTC): " + report.GeneratedAtUTC,
		"Fixture: " + report.FixtureID,
		"Metadata path: " + report.MetadataPath,
		fmt.Sprintf("Timing tolerance (ms): %d", report.TimingToleranceMS),
		fmt.Sprintf("Total divergences: %d", report.TotalDivergences),
		fmt.Sprintf("Failing divergences: %d", report.FailingCount),
		fmt.Sprintf("Unexplained divergences: %d", report.UnexplainedCount),
		fmt.Sprintf("Missing expected divergences: %d", report.MissingExpected),
		fmt.Sprintf("Expected divergences configured: %d", report.ExpectedConfigured),
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
	GeneratedAtUTC       string               `json:"generated_at_utc"`
	BaselineArtifactPath string               `json:"baseline_artifact_path"`
	Thresholds           ops.MVPSLOThresholds `json:"thresholds"`
	Report               ops.MVPSLOGateReport `json:"report"`
}

func writeSLOGatesReport(outputPath string, baselineArtifactPath string) error {
	entries, effectiveArtifactPath, err := loadRuntimeBaselineEntries(baselineArtifactPath)
	if err != nil {
		return err
	}

	thresholds := ops.DefaultMVPSLOThresholds()
	report := ops.EvaluateMVPSLOGates(toTurnMetrics(entries), thresholds)
	artifact := sloGateArtifact{
		GeneratedAtUTC:       time.Now().UTC().Format(time.RFC3339),
		BaselineArtifactPath: effectiveArtifactPath,
		Thresholds:           thresholds,
		Report:               report,
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

func writeRuntimeBaselineArtifact(outputPath string) error {
	_, err := generateRuntimeBaselineArtifact(outputPath)
	return err
}

func loadRuntimeBaselineEntries(baselineArtifactPath string) ([]timeline.BaselineEvidence, string, error) {
	if baselineArtifactPath == "" {
		baselineArtifactPath = defaultRuntimeBaselineArtifactPath
	}

	artifact, err := timeline.ReadBaselineArtifact(baselineArtifactPath)
	if err != nil {
		return nil, baselineArtifactPath, fmt.Errorf("load runtime baseline artifact %s: %w", baselineArtifactPath, err)
	}
	return artifact.Entries, baselineArtifactPath, nil
}

func generateRuntimeBaselineArtifact(baselineArtifactPath string) ([]timeline.BaselineEvidence, error) {
	if baselineArtifactPath == "" {
		baselineArtifactPath = defaultRuntimeBaselineArtifactPath
	}
	recorder := timeline.NewRecorder(timeline.StageAConfig{
		BaselineCapacity: 64,
		DetailCapacity:   128,
	})
	arbiter := turnarbiter.NewWithRecorder(&recorder)

	open0 := int64(0)
	open80 := int64(80)
	first420 := int64(420)
	open90 := int64(90)
	first560 := int64(560)
	open70 := int64(70)
	first360 := int64(360)
	cancelAccepted := int64(500)
	cancelFence := int64(620)
	cancelSent := int64(498)
	cancelAck := int64(621)

	invocationOutcome := func(invocationID string, providerID string, finalAttemptLatencyMS int64, totalInvocationLatencyMS int64) timeline.InvocationOutcomeEvidence {
		return timeline.InvocationOutcomeEvidence{
			ProviderInvocationID:     invocationID,
			Modality:                 "llm",
			ProviderID:               providerID,
			OutcomeClass:             "success",
			Retryable:                false,
			RetryDecision:            "none",
			AttemptCount:             1,
			FinalAttemptLatencyMS:    finalAttemptLatencyMS,
			TotalInvocationLatencyMS: totalInvocationLatencyMS,
		}
	}

	invocationScenario := func(
		sessionID string,
		turnID string,
		eventID string,
		runtimeSequence int64,
		runtimeTimestampMS int64,
		turnOpenMS int64,
		firstOutputMS int64,
		invocationOutcomes []timeline.InvocationOutcomeEvidence,
	) turnarbiter.ActiveInput {
		openProposedMS := int64(0)
		turnOpenAtMS := turnOpenMS
		firstOutputAtMS := firstOutputMS
		return turnarbiter.ActiveInput{
			SessionID:            sessionID,
			TurnID:               turnID,
			EventID:              eventID,
			PipelineVersion:      "pipeline-v1",
			RuntimeSequence:      runtimeSequence,
			RuntimeTimestampMS:   runtimeTimestampMS,
			WallClockTimestampMS: runtimeTimestampMS,
			AuthorityEpoch:       7,
			TerminalSuccessReady: true,
			BaselineEvidence: &timeline.BaselineEvidence{
				SessionID:            sessionID,
				TurnID:               turnID,
				PipelineVersion:      "pipeline-v1",
				EventID:              eventID,
				EnvelopeSnapshot:     "eventabi/v1",
				PayloadTags:          []eventabi.PayloadClass{eventabi.PayloadMetadata},
				RedactionDecisions:   []eventabi.RedactionDecision{{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow}},
				PlanHash:             "plan/" + turnID,
				SnapshotProvenance:   defaultSnapshotProvenance(),
				DecisionOutcomes:     []controlplane.DecisionOutcome{sloAdmitDecision(sessionID, turnID, eventID+"-admit", turnOpenMS)},
				InvocationOutcomes:   invocationOutcomes,
				DeterminismSeed:      runtimeSequence,
				OrderingMarkers:      []string{fmt.Sprintf("runtime_sequence:%d", runtimeSequence)},
				MergeRuleID:          "merge/default",
				MergeRuleVersion:     "v1.0",
				AuthorityEpoch:       7,
				TerminalOutcome:      "commit",
				CloseEmitted:         true,
				TurnOpenProposedAtMS: &openProposedMS,
				TurnOpenAtMS:         &turnOpenAtMS,
				FirstOutputAtMS:      &firstOutputAtMS,
			},
		}
	}

	scenarios := []turnarbiter.ActiveInput{
		{
			SessionID:            "sess-slo-runtime",
			TurnID:               "turn-slo-1",
			EventID:              "evt-slo-1",
			PipelineVersion:      "pipeline-v1",
			RuntimeSequence:      100,
			RuntimeTimestampMS:   450,
			WallClockTimestampMS: 450,
			AuthorityEpoch:       7,
			TerminalSuccessReady: true,
			BaselineEvidence: &timeline.BaselineEvidence{
				SessionID:            "sess-slo-runtime",
				TurnID:               "turn-slo-1",
				PipelineVersion:      "pipeline-v1",
				EventID:              "evt-slo-1",
				EnvelopeSnapshot:     "eventabi/v1",
				PayloadTags:          []eventabi.PayloadClass{eventabi.PayloadMetadata},
				RedactionDecisions:   []eventabi.RedactionDecision{{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow}},
				PlanHash:             "plan/turn-slo-1",
				SnapshotProvenance:   defaultSnapshotProvenance(),
				DecisionOutcomes:     []controlplane.DecisionOutcome{sloAdmitDecision("sess-slo-runtime", "turn-slo-1", "evt-slo-1-admit", 80)},
				DeterminismSeed:      100,
				OrderingMarkers:      []string{"runtime_sequence:100"},
				MergeRuleID:          "merge/default",
				MergeRuleVersion:     "v1.0",
				AuthorityEpoch:       7,
				TerminalOutcome:      "commit",
				CloseEmitted:         true,
				TurnOpenProposedAtMS: &open0,
				TurnOpenAtMS:         &open80,
				FirstOutputAtMS:      &first420,
			},
		},
		{
			SessionID:            "sess-slo-runtime",
			TurnID:               "turn-slo-2",
			EventID:              "evt-slo-2",
			PipelineVersion:      "pipeline-v1",
			RuntimeSequence:      110,
			RuntimeTimestampMS:   580,
			WallClockTimestampMS: 580,
			AuthorityEpoch:       7,
			TerminalSuccessReady: true,
			BaselineEvidence: &timeline.BaselineEvidence{
				SessionID:            "sess-slo-runtime",
				TurnID:               "turn-slo-2",
				PipelineVersion:      "pipeline-v1",
				EventID:              "evt-slo-2",
				EnvelopeSnapshot:     "eventabi/v1",
				PayloadTags:          []eventabi.PayloadClass{eventabi.PayloadMetadata},
				RedactionDecisions:   []eventabi.RedactionDecision{{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow}},
				PlanHash:             "plan/turn-slo-2",
				SnapshotProvenance:   defaultSnapshotProvenance(),
				DecisionOutcomes:     []controlplane.DecisionOutcome{sloAdmitDecision("sess-slo-runtime", "turn-slo-2", "evt-slo-2-admit", 90)},
				DeterminismSeed:      110,
				OrderingMarkers:      []string{"runtime_sequence:110"},
				MergeRuleID:          "merge/default",
				MergeRuleVersion:     "v1.0",
				AuthorityEpoch:       7,
				TerminalOutcome:      "commit",
				CloseEmitted:         true,
				TurnOpenProposedAtMS: &open0,
				TurnOpenAtMS:         &open90,
				FirstOutputAtMS:      &first560,
			},
		},
		{
			SessionID:            "sess-slo-runtime",
			TurnID:               "turn-slo-3",
			EventID:              "evt-slo-3",
			PipelineVersion:      "pipeline-v1",
			RuntimeSequence:      120,
			RuntimeTimestampMS:   622,
			WallClockTimestampMS: 622,
			AuthorityEpoch:       7,
			CancelAccepted:       true,
			BaselineEvidence: &timeline.BaselineEvidence{
				SessionID:              "sess-slo-runtime",
				TurnID:                 "turn-slo-3",
				PipelineVersion:        "pipeline-v1",
				EventID:                "evt-slo-3",
				EnvelopeSnapshot:       "eventabi/v1",
				PayloadTags:            []eventabi.PayloadClass{eventabi.PayloadMetadata},
				RedactionDecisions:     []eventabi.RedactionDecision{{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow}},
				PlanHash:               "plan/turn-slo-3",
				SnapshotProvenance:     defaultSnapshotProvenance(),
				DecisionOutcomes:       []controlplane.DecisionOutcome{sloAdmitDecision("sess-slo-runtime", "turn-slo-3", "evt-slo-3-admit", 70)},
				DeterminismSeed:        120,
				OrderingMarkers:        []string{"runtime_sequence:120"},
				MergeRuleID:            "merge/default",
				MergeRuleVersion:       "v1.0",
				AuthorityEpoch:         7,
				TerminalOutcome:        "abort",
				TerminalReason:         "cancelled",
				CloseEmitted:           true,
				TurnOpenProposedAtMS:   &open0,
				TurnOpenAtMS:           &open70,
				FirstOutputAtMS:        &first360,
				CancelSentAtMS:         &cancelSent,
				CancelAcceptedAtMS:     &cancelAccepted,
				CancelFenceAppliedAtMS: &cancelFence,
				CancelAckAtMS:          &cancelAck,
			},
		},
		invocationScenario(
			"sess-rd-002-runtime",
			"turn-rd-002",
			"evt-rd-002",
			130,
			512,
			95,
			500,
			[]timeline.InvocationOutcomeEvidence{
				invocationOutcome("inv-rd-002-1", "llm-a", 10, 24),
				invocationOutcome("inv-rd-002-2", "llm-b", 14, 31),
			},
		),
		invocationScenario(
			"sess-rd-003-runtime",
			"turn-rd-003",
			"evt-rd-003",
			140,
			510,
			100,
			500,
			[]timeline.InvocationOutcomeEvidence{
				invocationOutcome("inv-rd-003-1", "llm-a", 12, 27),
			},
		),
		invocationScenario(
			"sess-ae-001-runtime",
			"turn-ae-001",
			"evt-ae-001",
			150,
			470,
			90,
			430,
			[]timeline.InvocationOutcomeEvidence{
				invocationOutcome("inv-ae-001-1", "llm-a", 9, 23),
			},
		),
		invocationScenario(
			"sess-cf-001-runtime",
			"turn-cf-001",
			"evt-cf-001",
			160,
			480,
			90,
			420,
			[]timeline.InvocationOutcomeEvidence{
				invocationOutcome("inv-cf-001-1", "llm-a", 11, 26),
			},
		),
		invocationScenario(
			"sess-ml-001-runtime",
			"turn-ml-001",
			"evt-ml-001",
			170,
			495,
			90,
			450,
			[]timeline.InvocationOutcomeEvidence{
				invocationOutcome("inv-ml-001-1", "llm-a", 13, 29),
			},
		),
		invocationScenario(
			"sess-ordering-runtime",
			"turn-ordering-approved-1",
			"evt-ordering-approved-1",
			180,
			505,
			100,
			490,
			[]timeline.InvocationOutcomeEvidence{
				invocationOutcome("inv-ordering-1", "llm-a", 8, 22),
			},
		),
	}

	for _, scenario := range scenarios {
		result, err := arbiter.HandleActive(scenario)
		if err != nil {
			return nil, err
		}
		if result.State != controlplane.TurnClosed {
			return nil, fmt.Errorf("runtime artifact scenario %s did not close turn", scenario.TurnID)
		}
	}

	entries := recorder.BaselineEntries()
	if len(entries) == 0 {
		return nil, fmt.Errorf("runtime artifact generation produced no baseline entries")
	}
	if err := timeline.WriteBaselineArtifact(baselineArtifactPath, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func defaultSnapshotProvenance() controlplane.SnapshotProvenance {
	return controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/v1",
		AdmissionPolicySnapshot:   "admission-policy/v1",
		ABICompatibilitySnapshot:  "abi-compat/v1",
		VersionResolutionSnapshot: "version-resolution/v1",
		PolicyResolutionSnapshot:  "policy-resolution/v1",
		ProviderHealthSnapshot:    "provider-health/v1",
	}
}

func sloAdmitDecision(sessionID string, turnID string, eventID string, runtimeMS int64) controlplane.DecisionOutcome {
	return controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          sessionID,
		TurnID:             turnID,
		EventID:            eventID,
		RuntimeTimestampMS: runtimeMS,
		WallClockMS:        runtimeMS,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
}

func toTurnMetrics(entries []timeline.BaselineEvidence) []ops.TurnMetrics {
	samples := make([]ops.TurnMetrics, 0, len(entries))
	for _, entry := range entries {
		terminalEvents := []string{entry.TerminalOutcome}
		if entry.CloseEmitted {
			terminalEvents = append(terminalEvents, "close")
		}
		sample := ops.TurnMetrics{
			TurnID:                   entry.TurnID,
			Accepted:                 entry.IsAcceptedTurn(),
			HappyPath:                entry.TurnOpenAtMS != nil && entry.FirstOutputAtMS != nil,
			TurnOpenProposedAtMS:     entry.TurnOpenProposedAtMS,
			TurnOpenAtMS:             entry.TurnOpenAtMS,
			FirstOutputAtMS:          entry.FirstOutputAtMS,
			CancelAcceptedAtMS:       entry.CancelAcceptedAtMS,
			CancelFenceAppliedAtMS:   entry.CancelFenceAppliedAtMS,
			BaselineComplete:         entry.ValidateCompleteness() == nil,
			AcceptedStaleEpochOutput: entry.AcceptedStaleEpochOutput,
			TerminalEvents:           terminalEvents,
		}
		samples = append(samples, sample)
	}
	return samples
}

func renderSLOGatesSummary(artifact sloGateArtifact) string {
	report := artifact.Report
	lines := []string{
		"# MVP SLO Gates Report",
		"",
		"Generated at (UTC): " + artifact.GeneratedAtUTC,
		"Baseline artifact: " + artifact.BaselineArtifactPath,
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
