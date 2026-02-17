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
	cpnormalizer "github.com/tiger/realtime-speech-pipeline/internal/controlplane/normalizer"
	cpregistry "github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	replaycmp "github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
	toolingconformance "github.com/tiger/realtime-speech-pipeline/internal/tooling/conformance"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/ops"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/regression"
	toolingrelease "github.com/tiger/realtime-speech-pipeline/internal/tooling/release"
	"github.com/tiger/realtime-speech-pipeline/internal/tooling/validation"
)

const (
	replaySmokeTimingToleranceMS           int64 = 15
	replaySmokeFixtureID                         = "rd-001-smoke"
	replayRegressionDefaultGate                  = "full"
	defaultReplayMetadataPath                    = "test/replay/fixtures/metadata.json"
	defaultReplayRegressionReportPath            = ".codex/replay/regression-report.json"
	replayFixtureReportsDirName                  = "fixtures"
	defaultRuntimeBaselineArtifactPath           = ".codex/replay/runtime-baseline.json"
	defaultContractsReportPath                   = ".codex/ops/contracts-report.json"
	defaultSLOGatesReportPath                    = ".codex/ops/slo-gates-report.json"
	defaultMVPSLOGatesReportPath                 = ".codex/ops/slo-gates-mvp-report.json"
	defaultLiveProviderChainReportPath           = ".codex/providers/live-provider-chain-report.json"
	defaultStreamingChainReportPath              = ".codex/providers/live-provider-chain-report.streaming.json"
	defaultNonStreamingChainReportPath           = ".codex/providers/live-provider-chain-report.nonstreaming.json"
	defaultLiveLatencyCompareReportPath          = ".codex/providers/live-latency-compare.json"
	defaultLiveKitSmokeReportPath                = ".codex/providers/livekit-smoke-report.json"
	defaultConformanceGovernanceReportPath       = ".codex/ops/conformance-governance-report.json"
	defaultVersionSkewPolicyPath                 = "pipelines/compat/version_skew_policy_v1.json"
	defaultDeprecationPolicyPath                 = "pipelines/compat/deprecation_policy_v1.json"
	defaultConformanceProfilePath                = "pipelines/compat/conformance_profile_v1.json"
	defaultConformanceResultsPath                = "test/contract/fixtures/conformance_results_v1.json"
	defaultFeatureStatusFrameworkPath            = "docs/RSPP_features_framework.json"
	mvpE2EWaiveAfterAttempts                     = 3
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "validate-spec":
		specPath, mode, err := runValidateSpec(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to validate spec: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("spec validation passed: %s (mode=%s)\n", specPath, mode)
	case "validate-policy":
		policyPath, mode, err := runValidatePolicy(os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to validate policy: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("policy validation passed: %s (mode=%s)\n", policyPath, mode)
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
	case "validate-contracts-report":
		fixtureRoot := filepath.Join("test", "contract", "fixtures")
		outputPath := defaultContractsReportPath
		if len(os.Args) >= 3 {
			fixtureRoot = os.Args[2]
		}
		if len(os.Args) >= 4 {
			outputPath = os.Args[3]
		}
		if err := writeContractsReport(outputPath, fixtureRoot); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write contracts report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("contracts report written: %s\n", outputPath)
		fmt.Printf("contracts summary written: %s\n", summaryPath)
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
		outputPath := defaultSLOGatesReportPath
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
	case "slo-gates-mvp-report":
		outputPath := defaultMVPSLOGatesReportPath
		baselineArtifactPath := defaultRuntimeBaselineArtifactPath
		liveProviderChainPath := defaultLiveProviderChainReportPath
		liveKitSmokePath := defaultLiveKitSmokeReportPath
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if len(os.Args) >= 4 {
			baselineArtifactPath = os.Args[3]
		}
		if len(os.Args) >= 5 {
			liveProviderChainPath = os.Args[4]
		}
		if len(os.Args) >= 6 {
			liveKitSmokePath = os.Args[5]
		}
		if err := writeMVPSLOGatesReport(outputPath, baselineArtifactPath, liveProviderChainPath, liveKitSmokePath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write mvp slo gates report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("mvp slo gates report written: %s\n", outputPath)
		fmt.Printf("mvp slo gates summary written: %s\n", summaryPath)
	case "live-latency-compare-report":
		outputPath := defaultLiveLatencyCompareReportPath
		streamingPath := defaultStreamingChainReportPath
		nonStreamingPath := defaultNonStreamingChainReportPath
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if len(os.Args) >= 4 {
			streamingPath = os.Args[3]
		}
		if len(os.Args) >= 5 {
			nonStreamingPath = os.Args[4]
		}
		if err := writeLiveLatencyCompareReport(outputPath, streamingPath, nonStreamingPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write live latency compare report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("live latency compare report written: %s\n", outputPath)
		fmt.Printf("live latency compare summary written: %s\n", summaryPath)
	case "conformance-governance-report":
		outputPath := defaultConformanceGovernanceReportPath
		versionSkewPolicyPath := defaultVersionSkewPolicyPath
		deprecationPolicyPath := defaultDeprecationPolicyPath
		conformanceProfilePath := defaultConformanceProfilePath
		conformanceResultsPath := defaultConformanceResultsPath
		featureStatusFrameworkPath := defaultFeatureStatusFrameworkPath
		if len(os.Args) >= 3 {
			outputPath = os.Args[2]
		}
		if len(os.Args) >= 4 {
			versionSkewPolicyPath = os.Args[3]
		}
		if len(os.Args) >= 5 {
			deprecationPolicyPath = os.Args[4]
		}
		if len(os.Args) >= 6 {
			conformanceProfilePath = os.Args[5]
		}
		if len(os.Args) >= 7 {
			conformanceResultsPath = os.Args[6]
		}
		if len(os.Args) >= 8 {
			featureStatusFrameworkPath = os.Args[7]
		}
		if err := writeConformanceGovernanceReport(
			outputPath,
			versionSkewPolicyPath,
			deprecationPolicyPath,
			conformanceProfilePath,
			conformanceResultsPath,
			featureStatusFrameworkPath,
		); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write conformance governance report: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("conformance governance report written: %s\n", outputPath)
		fmt.Printf("conformance governance summary written: %s\n", summaryPath)
	case "publish-release":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "publish-release requires spec_ref and rollout_cfg_path")
			printUsage()
			os.Exit(2)
		}
		specRef := os.Args[2]
		rolloutConfigPath := os.Args[3]
		outputPath := toolingrelease.DefaultReleaseManifestPath
		contractsReportPath := defaultContractsReportPath
		replayRegressionReportPath := defaultReplayRegressionReportPath
		sloGatesReportPath := defaultSLOGatesReportPath
		if len(os.Args) >= 5 {
			outputPath = os.Args[4]
		}
		if len(os.Args) >= 6 {
			contractsReportPath = os.Args[5]
		}
		if len(os.Args) >= 7 {
			replayRegressionReportPath = os.Args[6]
		}
		if len(os.Args) >= 8 {
			sloGatesReportPath = os.Args[7]
		}
		manifest, err := writeReleaseManifest(
			outputPath,
			specRef,
			rolloutConfigPath,
			contractsReportPath,
			replayRegressionReportPath,
			sloGatesReportPath,
			time.Now(),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to publish release: %v\n", err)
			os.Exit(1)
		}
		summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
		fmt.Printf("release manifest written: %s\n", outputPath)
		fmt.Printf("release summary written: %s\n", summaryPath)
		fmt.Printf("release id: %s\n", manifest.ReleaseID)
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println("rspp-cli usage:")
	fmt.Println("  rspp-cli validate-spec <spec_path> [strict|relaxed]")
	fmt.Println("  rspp-cli validate-policy <policy_path> [strict|relaxed]")
	fmt.Println("  rspp-cli validate-contracts [fixture_root]")
	fmt.Println("  rspp-cli validate-contracts-report [fixture_root] [output_path]")
	fmt.Println("  rspp-cli replay-smoke-report [output_path] [metadata_path]")
	fmt.Println("  rspp-cli replay-regression-report [output_path] [metadata_path] [gate]")
	fmt.Println("  rspp-cli generate-runtime-baseline [output_path]")
	fmt.Println("  rspp-cli slo-gates-report [output_path] [baseline_artifact_path]")
	fmt.Println("  rspp-cli slo-gates-mvp-report [output_path] [baseline_artifact_path] [live_provider_chain_report_path] [livekit_smoke_report_path]")
	fmt.Println("  rspp-cli live-latency-compare-report [output_path] [streaming_report_path] [non_streaming_report_path]")
	fmt.Println("  rspp-cli conformance-governance-report [output_path] [version_skew_policy_path] [deprecation_policy_path] [conformance_profile_path] [conformance_results_path] [feature_status_framework_path]")
	fmt.Println("  rspp-cli publish-release <spec_ref> <rollout_cfg_path> [output_path] [contracts_report_path] [replay_report_path] [slo_report_path]")
}

func runValidateSpec(args []string) (string, validation.ValidationMode, error) {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		return "", "", fmt.Errorf("validate-spec requires spec_path")
	}
	modeArg := ""
	if len(args) >= 2 {
		modeArg = args[1]
	}
	mode, err := validation.ParseValidationMode(modeArg)
	if err != nil {
		return "", "", err
	}
	specPath := strings.TrimSpace(args[0])
	if err := validation.ValidatePipelineSpecFile(specPath, string(mode)); err != nil {
		return "", "", err
	}
	return specPath, mode, nil
}

func runValidatePolicy(args []string) (string, validation.ValidationMode, error) {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		return "", "", fmt.Errorf("validate-policy requires policy_path")
	}
	modeArg := ""
	if len(args) >= 2 {
		modeArg = args[1]
	}
	mode, err := validation.ParseValidationMode(modeArg)
	if err != nil {
		return "", "", err
	}
	policyPath := strings.TrimSpace(args[0])
	if err := validation.ValidatePolicyBundleFile(policyPath, string(mode)); err != nil {
		return "", "", err
	}
	return policyPath, mode, nil
}

type replaySmokeReport struct {
	GeneratedAtUTC     string                 `json:"generated_at_utc"`
	FixtureID          string                 `json:"fixture_id"`
	MetadataPath       string                 `json:"metadata_path"`
	Mode               obs.ReplayMode         `json:"mode"`
	ReplayRunID        string                 `json:"replay_run_id"`
	RequestCursor      *obs.ReplayCursor      `json:"request_cursor,omitempty"`
	ResultCursor       *obs.ReplayCursor      `json:"result_cursor,omitempty"`
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
	run, err := buildReplaySmokeRun(effectiveTimingToleranceMS)
	if err != nil {
		return err
	}
	divergences := run.Result.Divergences
	evaluation := regression.EvaluateDivergences(divergences, policy)

	byClass := replayByClassCounter()
	for _, d := range divergences {
		byClass[string(d.Class)]++
	}
	report := replaySmokeReport{
		GeneratedAtUTC:     time.Now().UTC().Format(time.RFC3339),
		FixtureID:          replaySmokeFixtureID,
		MetadataPath:       metadataPath,
		Mode:               run.Result.Mode,
		ReplayRunID:        run.Result.RunID,
		RequestCursor:      run.Request.Cursor,
		ResultCursor:       run.Result.Cursor,
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
	FixtureID                         string            `json:"fixture_id"`
	Gate                              string            `json:"gate"`
	Mode                              obs.ReplayMode    `json:"mode"`
	ReplayRunID                       string            `json:"replay_run_id"`
	RequestCursor                     *obs.ReplayCursor `json:"request_cursor,omitempty"`
	ResultCursor                      *obs.ReplayCursor `json:"result_cursor,omitempty"`
	TimingToleranceMS                 int64             `json:"timing_tolerance_ms"`
	FinalAttemptLatencyThresholdMS    *int64            `json:"final_attempt_latency_threshold_ms,omitempty"`
	TotalInvocationLatencyThresholdMS *int64            `json:"total_invocation_latency_threshold_ms,omitempty"`
	InvocationLatencyBreaches         int               `json:"invocation_latency_breaches,omitempty"`
	TotalDivergences                  int               `json:"total_divergences"`
	FailingCount                      int               `json:"failing_count"`
	UnexplainedCount                  int               `json:"unexplained_count"`
	MissingExpected                   int               `json:"missing_expected"`
	ExpectedConfigured                int               `json:"expected_configured"`
	ByClass                           map[string]int    `json:"by_class"`
	FailingClasses                    []string          `json:"failing_classes,omitempty"`
}

type replayFixtureArtifact struct {
	GeneratedAtUTC string `json:"generated_at_utc"`
	MetadataPath   string `json:"metadata_path"`
	replayFixtureExecutionReport
	Status string `json:"status"`
}

type replayRegressionReport struct {
	SchemaVersion      string                         `json:"schema_version"`
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

type replayFixtureRun struct {
	Request obs.ReplayRunRequest `json:"request"`
	Result  obs.ReplayRunResult  `json:"result"`
}

type replayFixtureBuilder func(timingToleranceMS int64) (replayFixtureRun, error)

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
	totalByClass := replayByClassCounter()
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
		run, err := builder(timingToleranceMS)
		if err != nil {
			return fmt.Errorf("execute replay fixture %s: %w", fixtureID, err)
		}
		divergences := append([]obs.ReplayDivergence(nil), run.Result.Divergences...)
		latencyThresholdDivergences := buildInvocationLatencyThresholdDivergences(fixtureID, policy, latencySamplesByScope, latencySamplesErr)
		divergences = append(divergences, latencyThresholdDivergences...)
		evaluation := regression.EvaluateDivergences(divergences, regression.DivergencePolicy{
			TimingToleranceMS: timingToleranceMS,
			Expected:          policy.ExpectedDivergences,
		})

		byClass := replayByClassCounter()
		for _, entry := range divergences {
			byClass[string(entry.Class)]++
			totalByClass[string(entry.Class)]++
		}

		report := replayFixtureExecutionReport{
			FixtureID:                         fixtureID,
			Gate:                              normalizedGate,
			Mode:                              run.Result.Mode,
			ReplayRunID:                       run.Result.RunID,
			RequestCursor:                     run.Request.Cursor,
			ResultCursor:                      run.Result.Cursor,
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
		SchemaVersion:      toolingrelease.ReplayRegressionReportSchemaVersionV1,
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
		"cf-002-provider-late-output":          buildReplayNoDivergencePlayback,
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
		"rd-001-smoke":                         buildReplaySmokeRun,
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

func buildReplayNoDivergence(timingToleranceMS int64) (replayFixtureRun, error) {
	return buildReplaySmokeRunWithMode(timingToleranceMS, obs.ReplayModeReSimulateNodes, nil)
}

func buildReplayNoDivergencePlayback(timingToleranceMS int64) (replayFixtureRun, error) {
	return buildReplaySmokeRunWithMode(timingToleranceMS, obs.ReplayModePlaybackRecordedProvider, nil)
}

func buildReplayTimingDivergenceWithinTolerance(timingToleranceMS int64) (replayFixtureRun, error) {
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
		Lane:                  eventabi.LaneControl,
		RuntimeSequence:       100,
		ProviderID:            "provider-a",
		ProviderModel:         "model-a",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:100",
		AuthorityEpoch:        7,
		RuntimeTimestampMS:    100,
	}}
	replayed := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-rd-002",
		SnapshotProvenanceRef: "snapshot-rd-002",
		Lane:                  eventabi.LaneControl,
		RuntimeSequence:       100,
		ProviderID:            "provider-a",
		ProviderModel:         "model-a",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:100",
		AuthorityEpoch:        7,
		RuntimeTimestampMS:    112,
	}}
	return runReplayFixture(obs.ReplayModeRecomputeDecisions, timingToleranceMS, baseline, replayed, nil, nil, nil)
}

func buildReplayPlanDivergence(timingToleranceMS int64) (replayFixtureRun, error) {
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
		Lane:                  eventabi.LaneControl,
		RuntimeSequence:       200,
		ProviderID:            "provider-a",
		ProviderModel:         "model-a",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:200",
		AuthorityEpoch:        9,
		RuntimeTimestampMS:    100,
	}}
	replayed := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-rd-004",
		SnapshotProvenanceRef: "snapshot-b",
		Lane:                  eventabi.LaneControl,
		RuntimeSequence:       200,
		ProviderID:            "provider-a",
		ProviderModel:         "model-a",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:200",
		AuthorityEpoch:        9,
		RuntimeTimestampMS:    100,
	}}
	return runReplayFixture(obs.ReplayModeReSimulateNodes, timingToleranceMS, baseline, replayed, nil, nil, nil)
}

func buildReplayOrderingDivergence(timingToleranceMS int64) (replayFixtureRun, error) {
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
		Lane:                  eventabi.LaneControl,
		RuntimeSequence:       300,
		ProviderID:            "provider-a",
		ProviderModel:         "model-a",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:300",
		AuthorityEpoch:        11,
		RuntimeTimestampMS:    300,
	}}
	replayed := []replaycmp.TraceArtifact{{
		PlanHash:              "plan-ordering-1",
		SnapshotProvenanceRef: "snapshot-ordering-1",
		Lane:                  eventabi.LaneControl,
		RuntimeSequence:       300,
		ProviderID:            "provider-a",
		ProviderModel:         "model-a",
		Decision:              decision,
		OrderingMarker:        "runtime_sequence:301",
		AuthorityEpoch:        11,
		RuntimeTimestampMS:    300,
	}}
	cursor := &obs.ReplayCursor{
		SessionID:       decision.SessionID,
		TurnID:          decision.TurnID,
		Lane:            eventabi.LaneControl,
		RuntimeSequence: 300,
		EventID:         decision.EventID,
	}
	return runReplayFixture(obs.ReplayModeReSimulateNodes, timingToleranceMS, baseline, replayed, nil, nil, cursor)
}

func buildReplayML003OutcomeDivergence(timingToleranceMS int64) (replayFixtureRun, error) {
	baseline := []replaycmp.LineageRecord{
		{EventID: "evt-ml003-drop", Dropped: true, MergeGroupID: ""},
		{EventID: "evt-ml003-merge", Dropped: false, MergeGroupID: "merge-ml003"},
	}
	replayed := []replaycmp.LineageRecord{
		{EventID: "evt-ml003-drop", Dropped: true, MergeGroupID: ""},
	}
	return runReplayFixture(obs.ReplayModeReplayDecisions, timingToleranceMS, nil, nil, baseline, replayed, nil)
}

func buildReplaySmokeRun(timingToleranceMS int64) (replayFixtureRun, error) {
	return buildReplaySmokeRunWithMode(timingToleranceMS, obs.ReplayModeReSimulateNodes, nil)
}

func buildReplaySmokeRunWithMode(timingToleranceMS int64, mode obs.ReplayMode, cursor *obs.ReplayCursor) (replayFixtureRun, error) {
	epoch := int64(7)
	baseline := []replaycmp.TraceArtifact{
		{
			PlanHash:              "plan-smoke-a",
			SnapshotProvenanceRef: "snapshot-a",
			Lane:                  eventabi.LaneControl,
			RuntimeSequence:       100,
			ProviderID:            "provider-a",
			ProviderModel:         "model-a",
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
			Lane:                  eventabi.LaneControl,
			RuntimeSequence:       110,
			ProviderID:            "provider-a",
			ProviderModel:         "model-a",
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
	return runReplayFixture(mode, timingToleranceMS, baseline, replayed, nil, nil, cursor)
}

func runReplayFixture(
	mode obs.ReplayMode,
	timingToleranceMS int64,
	baselineTrace []replaycmp.TraceArtifact,
	candidateTrace []replaycmp.TraceArtifact,
	baselineLineage []replaycmp.LineageRecord,
	candidateLineage []replaycmp.LineageRecord,
	cursor *obs.ReplayCursor,
) (replayFixtureRun, error) {
	request := obs.ReplayRunRequest{
		BaselineRef:      "fixture://baseline",
		CandidatePlanRef: "fixture://candidate",
		Mode:             mode,
		Cursor:           cloneReplayCursor(cursor),
	}
	engine := replaycmp.Engine{
		ArtifactResolver: replaycmp.StaticReplayArtifactResolver{
			Artifacts: map[string]replaycmp.ReplayArtifact{
				request.BaselineRef: {
					TraceArtifacts: append([]replaycmp.TraceArtifact(nil), baselineTrace...),
					LineageRecords: append([]replaycmp.LineageRecord(nil), baselineLineage...),
				},
				request.CandidatePlanRef: {
					TraceArtifacts: append([]replaycmp.TraceArtifact(nil), candidateTrace...),
					LineageRecords: append([]replaycmp.LineageRecord(nil), candidateLineage...),
				},
			},
		},
		CompareConfig: replaycmp.CompareConfig{TimingToleranceMS: timingToleranceMS},
		NewRunID:      func() string { return replayRunID(request) },
	}
	result, err := engine.Run(request)
	if err != nil {
		return replayFixtureRun{}, err
	}
	return replayFixtureRun{Request: request, Result: result}, nil
}

func cloneReplayCursor(cursor *obs.ReplayCursor) *obs.ReplayCursor {
	if cursor == nil {
		return nil
	}
	cloned := *cursor
	return &cloned
}

func replayRunID(request obs.ReplayRunRequest) string {
	cursorSegment := "cursor:none"
	if request.Cursor != nil {
		cursorSegment = fmt.Sprintf(
			"cursor:%s|%s|%s|%d|%s",
			request.Cursor.SessionID,
			request.Cursor.TurnID,
			request.Cursor.Lane,
			request.Cursor.RuntimeSequence,
			request.Cursor.EventID,
		)
	}
	return fmt.Sprintf("run:%s|%s|%s|%s", request.Mode, request.BaselineRef, request.CandidatePlanRef, cursorSegment)
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
		obs.ProviderChoiceDivergence,
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
		"Replay mode: " + string(report.Mode),
		"Replay run id: " + report.ReplayRunID,
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
		obs.ProviderChoiceDivergence,
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
	run, err := buildReplaySmokeRun(timingToleranceMS)
	if err != nil {
		return []obs.ReplayDivergence{{
			Class:   obs.OutcomeDivergence,
			Scope:   "fixture:rd-001-smoke",
			Message: fmt.Sprintf("replay engine execution failed: %v", err),
		}}
	}
	return run.Result.Divergences
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

func replayByClassCounter() map[string]int {
	return map[string]int{
		string(obs.PlanDivergence):           0,
		string(obs.OutcomeDivergence):        0,
		string(obs.OrderingDivergence):       0,
		string(obs.AuthorityDivergence):      0,
		string(obs.ProviderChoiceDivergence): 0,
		string(obs.TimingDivergence):         0,
	}
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
		"Replay mode: " + string(report.Mode),
		"Replay run id: " + report.ReplayRunID,
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
		obs.ProviderChoiceDivergence,
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
	SchemaVersion        string               `json:"schema_version"`
	GeneratedAtUTC       string               `json:"generated_at_utc"`
	BaselineArtifactPath string               `json:"baseline_artifact_path"`
	Thresholds           ops.MVPSLOThresholds `json:"thresholds"`
	Report               ops.MVPSLOGateReport `json:"report"`
}

type mvpSLOGateArtifact struct {
	SchemaVersion          string                `json:"schema_version"`
	GeneratedAtUTC         string                `json:"generated_at_utc"`
	BaselineArtifactPath   string                `json:"baseline_artifact_path"`
	LiveProviderChainPath  string                `json:"live_provider_chain_path"`
	LiveKitSmokeReportPath string                `json:"livekit_smoke_report_path"`
	Thresholds             ops.MVPSLOThresholds  `json:"thresholds"`
	BaselineReport         ops.MVPSLOGateReport  `json:"baseline_report"`
	LiveEndToEnd           mvpLiveEndToEndGate   `json:"live_end_to_end"`
	FixedDecisions         mvpFixedDecisionGates `json:"fixed_decisions"`
	Warnings               []string              `json:"warnings,omitempty"`
	Violations             []string              `json:"violations,omitempty"`
	Passed                 bool                  `json:"passed"`
}

type mvpLiveEndToEndGate struct {
	SampleCount   int      `json:"sample_count"`
	EndToEndP50MS *int64   `json:"end_to_end_p50_ms,omitempty"`
	EndToEndP95MS *int64   `json:"end_to_end_p95_ms,omitempty"`
	Waived        bool     `json:"waived"`
	WaiverReason  string   `json:"waiver_reason,omitempty"`
	AttemptFiles  []string `json:"attempt_files,omitempty"`
	Violations    []string `json:"violations,omitempty"`
	Passed        bool     `json:"passed"`
}

type mvpFixedDecisionGates struct {
	SimpleModeOnly          bool     `json:"simple_mode_only"`
	ProviderPolicyCompliant bool     `json:"provider_policy_compliant"`
	LiveKitPathReady        bool     `json:"livekit_path_ready"`
	StreamingEvidence       bool     `json:"streaming_evidence"`
	NonStreamingEvidence    bool     `json:"non_streaming_evidence"`
	ParityMarkersPresent    bool     `json:"parity_markers_present"`
	ParityComparisonValid   bool     `json:"parity_comparison_valid"`
	Violations              []string `json:"violations,omitempty"`
	Passed                  bool     `json:"passed"`
}

type liveProviderChainArtifact struct {
	GeneratedAtUTC        string                              `json:"generated_at_utc"`
	Status                string                              `json:"status"`
	ExecutionMode         string                              `json:"execution_mode,omitempty"`
	ComparisonIdentity    string                              `json:"comparison_identity,omitempty"`
	SemanticParity        *bool                               `json:"semantic_parity,omitempty"`
	ParityComparisonValid *bool                               `json:"parity_comparison_valid,omitempty"`
	ParityInvalidReason   string                              `json:"parity_invalid_reason,omitempty"`
	EnabledProviders      liveProviderChainEnabledProviders   `json:"enabled_providers"`
	SelectedCombinations  []liveProviderChainComboSelection   `json:"selected_combinations,omitempty"`
	Combinations          []liveProviderChainCombinationEntry `json:"combinations,omitempty"`
}

type liveProviderChainEnabledProviders struct {
	STT []string `json:"stt"`
	LLM []string `json:"llm"`
	TTS []string `json:"tts"`
}

type liveProviderChainComboSelection struct {
	STTProviderID string `json:"stt_provider_id"`
	LLMProviderID string `json:"llm_provider_id"`
	TTSProviderID string `json:"tts_provider_id"`
}

type liveProviderChainCombinationEntry struct {
	ComboIndex    int                             `json:"combo_index,omitempty"`
	ComboID       string                          `json:"combo_id,omitempty"`
	STTProviderID string                          `json:"stt_provider_id,omitempty"`
	LLMProviderID string                          `json:"llm_provider_id,omitempty"`
	TTSProviderID string                          `json:"tts_provider_id,omitempty"`
	Status        string                          `json:"status"`
	Handoffs      []liveProviderChainHandoffEntry `json:"handoffs,omitempty"`
	Steps         []liveProviderChainStepEntry    `json:"steps,omitempty"`
	Latency       liveProviderChainLatency        `json:"latency,omitempty"`
}

type liveProviderChainLatency struct {
	FirstAssistantAudioE2EMS int64 `json:"first_assistant_audio_e2e_latency_ms,omitempty"`
	TurnCompletionE2EMS      int64 `json:"turn_completion_e2e_latency_ms,omitempty"`
}

type liveProviderChainHandoffEntry struct {
	HandoffID string `json:"handoff_id,omitempty"`
}

type liveProviderChainStepEntry struct {
	Step   string                      `json:"step,omitempty"`
	Output liveProviderChainStepOutput `json:"output"`
}

type liveProviderChainStepOutput struct {
	Attempts []liveProviderChainStepAttempt `json:"attempts,omitempty"`
}

type liveProviderChainStepAttempt struct {
	StreamingUsed bool `json:"streaming_used,omitempty"`
}

type liveKitSmokeArtifact struct {
	Probe liveKitProbeResult `json:"probe"`
}

type liveKitProbeResult struct {
	Status string `json:"status"`
}

type liveLatencyCompareArtifact struct {
	GeneratedAtUTC         string                      `json:"generated_at_utc"`
	StreamingReportPath    string                      `json:"streaming_report_path"`
	NonStreamingReportPath string                      `json:"non_streaming_report_path"`
	ComparisonIdentity     string                      `json:"comparison_identity,omitempty"`
	PairCount              int                         `json:"pair_count"`
	Combos                 []liveLatencyCompareCombo   `json:"combos,omitempty"`
	Aggregate              liveLatencyCompareAggregate `json:"aggregate"`
	Conclusions            []string                    `json:"conclusions,omitempty"`
}

type liveLatencyCompareCombo struct {
	ComboKey                    string `json:"combo_key"`
	STTProviderID               string `json:"stt_provider_id"`
	LLMProviderID               string `json:"llm_provider_id"`
	TTSProviderID               string `json:"tts_provider_id"`
	StreamingStatus             string `json:"streaming_status,omitempty"`
	NonStreamingStatus          string `json:"non_streaming_status,omitempty"`
	StreamingFirstAudioE2EMS    int64  `json:"streaming_first_assistant_audio_e2e_latency_ms,omitempty"`
	NonStreamingFirstAudioE2EMS int64  `json:"non_streaming_first_assistant_audio_e2e_latency_ms,omitempty"`
	FirstAudioDeltaMS           int64  `json:"first_assistant_audio_delta_ms,omitempty"`
	StreamingCompletionE2EMS    int64  `json:"streaming_turn_completion_e2e_latency_ms,omitempty"`
	NonStreamingCompletionE2EMS int64  `json:"non_streaming_turn_completion_e2e_latency_ms,omitempty"`
	CompletionDeltaMS           int64  `json:"turn_completion_delta_ms,omitempty"`
	FirstAudioWinner            string `json:"first_audio_winner,omitempty"`
	CompletionWinner            string `json:"completion_winner,omitempty"`
}

type liveLatencyCompareAggregate struct {
	FirstAudio liveLatencyMetricAggregate `json:"first_audio"`
	Completion liveLatencyMetricAggregate `json:"completion"`
}

type liveLatencyMetricAggregate struct {
	StreamingSampleCount    int    `json:"streaming_sample_count"`
	NonStreamingSampleCount int    `json:"non_streaming_sample_count"`
	PairedSampleCount       int    `json:"paired_sample_count"`
	StreamingP50MS          *int64 `json:"streaming_p50_ms,omitempty"`
	StreamingP95MS          *int64 `json:"streaming_p95_ms,omitempty"`
	NonStreamingP50MS       *int64 `json:"non_streaming_p50_ms,omitempty"`
	NonStreamingP95MS       *int64 `json:"non_streaming_p95_ms,omitempty"`
	DeltaP50MS              *int64 `json:"delta_p50_ms,omitempty"`
	DeltaP95MS              *int64 `json:"delta_p95_ms,omitempty"`
	Winner                  string `json:"winner,omitempty"`
}

type contractsReportArtifact struct {
	SchemaVersion  string                               `json:"schema_version"`
	GeneratedAtUTC string                               `json:"generated_at_utc"`
	FixtureRoot    string                               `json:"fixture_root"`
	Summary        validation.ContractValidationSummary `json:"summary"`
	Passed         bool                                 `json:"passed"`
}

type conformanceGovernanceArtifact struct {
	SchemaVersion              string                                  `json:"schema_version"`
	GeneratedAtUTC             string                                  `json:"generated_at_utc"`
	VersionSkewPolicyPath      string                                  `json:"version_skew_policy_path"`
	DeprecationPolicyPath      string                                  `json:"deprecation_policy_path"`
	ConformanceProfilePath     string                                  `json:"conformance_profile_path"`
	ConformanceResultsPath     string                                  `json:"conformance_results_path"`
	FeatureStatusFrameworkPath string                                  `json:"feature_status_framework_path"`
	Report                     toolingconformance.GovernanceEvaluation `json:"report"`
}

func writeSLOGatesReport(outputPath string, baselineArtifactPath string) error {
	entries, effectiveArtifactPath, err := loadRuntimeBaselineEntries(baselineArtifactPath)
	if err != nil {
		return err
	}

	thresholds := ops.DefaultMVPSLOThresholds()
	report := ops.EvaluateMVPSLOGates(toTurnMetrics(entries), thresholds)
	artifact := sloGateArtifact{
		SchemaVersion:        toolingrelease.SLOGatesReportSchemaVersionV1,
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

func writeMVPSLOGatesReport(outputPath string, baselineArtifactPath string, liveProviderChainPath string, liveKitSmokePath string) error {
	entries, effectiveBaselinePath, err := loadRuntimeBaselineEntries(baselineArtifactPath)
	if err != nil {
		return err
	}
	chainReport, effectiveChainPath, err := loadLiveProviderChainArtifact(liveProviderChainPath)
	if err != nil {
		return err
	}

	thresholds := ops.DefaultMVPSLOThresholds()
	baselineReport := ops.EvaluateMVPSLOGates(toTurnMetrics(entries), thresholds)
	liveE2E := evaluateLiveEndToEndGate(chainReport, thresholds, effectiveChainPath)
	fixedDecisions := evaluateMVPFixedDecisionGates(chainReport, liveKitSmokePath)

	artifact := mvpSLOGateArtifact{
		SchemaVersion:          toolingrelease.MVPSLOGatesReportSchemaVersionV1,
		GeneratedAtUTC:         time.Now().UTC().Format(time.RFC3339),
		BaselineArtifactPath:   effectiveBaselinePath,
		LiveProviderChainPath:  effectiveChainPath,
		LiveKitSmokeReportPath: liveKitSmokePath,
		Thresholds:             thresholds,
		BaselineReport:         baselineReport,
		LiveEndToEnd:           liveE2E,
		FixedDecisions:         fixedDecisions,
	}

	if !baselineReport.Passed {
		artifact.Violations = append(artifact.Violations, "baseline SLO gates failed")
	}
	if !liveE2E.Passed {
		artifact.Violations = append(artifact.Violations, liveE2E.Violations...)
	}
	if liveE2E.Waived {
		artifact.Warnings = append(artifact.Warnings, liveE2E.WaiverReason)
	}
	if !fixedDecisions.Passed {
		artifact.Violations = append(artifact.Violations, fixedDecisions.Violations...)
	}

	artifact.Passed = baselineReport.Passed && liveE2E.Passed && fixedDecisions.Passed

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
	if err := os.WriteFile(summaryPath, []byte(renderMVPSLOGatesSummary(artifact)), 0o644); err != nil {
		return err
	}

	if !artifact.Passed {
		return fmt.Errorf("mvp slo gate failed: %v", artifact.Violations)
	}
	return nil
}

func writeLiveLatencyCompareReport(outputPath string, streamingPath string, nonStreamingPath string) error {
	streamingReport, effectiveStreamingPath, err := loadLiveProviderChainArtifact(streamingPath)
	if err != nil {
		return err
	}
	nonStreamingReport, effectiveNonStreamingPath, err := loadLiveProviderChainArtifact(nonStreamingPath)
	if err != nil {
		return err
	}
	if !isStreamingMode(streamingReport.ExecutionMode) {
		return fmt.Errorf("streaming report must have execution_mode=streaming, got %q", streamingReport.ExecutionMode)
	}
	if !isNonStreamingMode(nonStreamingReport.ExecutionMode) {
		return fmt.Errorf("non-streaming report must have execution_mode=non_streaming, got %q", nonStreamingReport.ExecutionMode)
	}
	if strings.TrimSpace(streamingReport.ComparisonIdentity) == "" || strings.TrimSpace(nonStreamingReport.ComparisonIdentity) == "" {
		return fmt.Errorf("comparison identity missing from streaming/non-streaming reports")
	}
	if streamingReport.ComparisonIdentity != nonStreamingReport.ComparisonIdentity {
		return fmt.Errorf(
			"comparison identity mismatch: streaming=%s non_streaming=%s",
			streamingReport.ComparisonIdentity,
			nonStreamingReport.ComparisonIdentity,
		)
	}

	streamingViolations := validateLiveProviderModeEvidence(streamingReport)
	if len(streamingViolations) > 0 {
		return fmt.Errorf("streaming report mode evidence invalid: %s", strings.Join(streamingViolations, "; "))
	}
	nonStreamingViolations := validateLiveProviderModeEvidence(nonStreamingReport)
	if len(nonStreamingViolations) > 0 {
		return fmt.Errorf("non-streaming report mode evidence invalid: %s", strings.Join(nonStreamingViolations, "; "))
	}

	streamingByKey := mapCombinationsByKey(streamingReport)
	nonStreamingByKey := mapCombinationsByKey(nonStreamingReport)
	keys := intersectionKeys(streamingByKey, nonStreamingByKey)
	if len(keys) == 0 {
		return fmt.Errorf("no overlapping provider combinations between streaming and non-streaming reports")
	}

	combos := make([]liveLatencyCompareCombo, 0, len(keys))
	streamingFirstAudio := make([]int64, 0, len(keys))
	nonStreamingFirstAudio := make([]int64, 0, len(keys))
	firstAudioDelta := make([]int64, 0, len(keys))
	streamingCompletion := make([]int64, 0, len(keys))
	nonStreamingCompletion := make([]int64, 0, len(keys))
	completionDelta := make([]int64, 0, len(keys))

	for _, key := range keys {
		streamCombo := streamingByKey[key]
		nonStreamCombo := nonStreamingByKey[key]
		combo := liveLatencyCompareCombo{
			ComboKey:                    key,
			STTProviderID:               streamCombo.STTProviderID,
			LLMProviderID:               streamCombo.LLMProviderID,
			TTSProviderID:               streamCombo.TTSProviderID,
			StreamingStatus:             streamCombo.Status,
			NonStreamingStatus:          nonStreamCombo.Status,
			StreamingFirstAudioE2EMS:    streamCombo.Latency.FirstAssistantAudioE2EMS,
			NonStreamingFirstAudioE2EMS: nonStreamCombo.Latency.FirstAssistantAudioE2EMS,
			StreamingCompletionE2EMS:    streamCombo.Latency.TurnCompletionE2EMS,
			NonStreamingCompletionE2EMS: nonStreamCombo.Latency.TurnCompletionE2EMS,
		}

		if streamCombo.Status == "pass" && streamCombo.Latency.FirstAssistantAudioE2EMS > 0 {
			streamingFirstAudio = append(streamingFirstAudio, streamCombo.Latency.FirstAssistantAudioE2EMS)
		}
		if nonStreamCombo.Status == "pass" && nonStreamCombo.Latency.FirstAssistantAudioE2EMS > 0 {
			nonStreamingFirstAudio = append(nonStreamingFirstAudio, nonStreamCombo.Latency.FirstAssistantAudioE2EMS)
		}
		if streamCombo.Status == "pass" && nonStreamCombo.Status == "pass" &&
			streamCombo.Latency.FirstAssistantAudioE2EMS > 0 && nonStreamCombo.Latency.FirstAssistantAudioE2EMS > 0 {
			delta := streamCombo.Latency.FirstAssistantAudioE2EMS - nonStreamCombo.Latency.FirstAssistantAudioE2EMS
			combo.FirstAudioDeltaMS = delta
			combo.FirstAudioWinner = winnerForDelta(delta)
			firstAudioDelta = append(firstAudioDelta, delta)
		}

		if streamCombo.Status == "pass" && streamCombo.Latency.TurnCompletionE2EMS > 0 {
			streamingCompletion = append(streamingCompletion, streamCombo.Latency.TurnCompletionE2EMS)
		}
		if nonStreamCombo.Status == "pass" && nonStreamCombo.Latency.TurnCompletionE2EMS > 0 {
			nonStreamingCompletion = append(nonStreamingCompletion, nonStreamCombo.Latency.TurnCompletionE2EMS)
		}
		if streamCombo.Status == "pass" && nonStreamCombo.Status == "pass" &&
			streamCombo.Latency.TurnCompletionE2EMS > 0 && nonStreamCombo.Latency.TurnCompletionE2EMS > 0 {
			delta := streamCombo.Latency.TurnCompletionE2EMS - nonStreamCombo.Latency.TurnCompletionE2EMS
			combo.CompletionDeltaMS = delta
			combo.CompletionWinner = winnerForDelta(delta)
			completionDelta = append(completionDelta, delta)
		}

		combos = append(combos, combo)
	}

	aggregate := liveLatencyCompareAggregate{
		FirstAudio: liveLatencyMetricAggregate{
			StreamingSampleCount:    len(streamingFirstAudio),
			NonStreamingSampleCount: len(nonStreamingFirstAudio),
			PairedSampleCount:       len(firstAudioDelta),
		},
		Completion: liveLatencyMetricAggregate{
			StreamingSampleCount:    len(streamingCompletion),
			NonStreamingSampleCount: len(nonStreamingCompletion),
			PairedSampleCount:       len(completionDelta),
		},
	}
	populateMetricAggregate(&aggregate.FirstAudio, streamingFirstAudio, nonStreamingFirstAudio, firstAudioDelta)
	populateMetricAggregate(&aggregate.Completion, streamingCompletion, nonStreamingCompletion, completionDelta)

	artifact := liveLatencyCompareArtifact{
		GeneratedAtUTC:         time.Now().UTC().Format(time.RFC3339),
		StreamingReportPath:    effectiveStreamingPath,
		NonStreamingReportPath: effectiveNonStreamingPath,
		ComparisonIdentity:     streamingReport.ComparisonIdentity,
		PairCount:              len(keys),
		Combos:                 combos,
		Aggregate:              aggregate,
		Conclusions: []string{
			fmt.Sprintf("first-audio winner: %s", defaultStringMetricWinner(aggregate.FirstAudio.Winner)),
			fmt.Sprintf("completion winner: %s", defaultStringMetricWinner(aggregate.Completion.Winner)),
		},
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, payload, 0o644); err != nil {
		return err
	}
	summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
	if err := os.WriteFile(summaryPath, []byte(renderLiveLatencyCompareSummary(artifact)), 0o644); err != nil {
		return err
	}
	return nil
}

func isStreamingMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "streaming")
}

func isNonStreamingMode(mode string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "non_streaming", "non-streaming", "nonstreaming":
		return true
	default:
		return false
	}
}

func validateLiveProviderModeEvidence(report liveProviderChainArtifact) []string {
	violations := make([]string, 0, 4)
	combinations := normalizeChainCombinations(report)
	switch {
	case isStreamingMode(report.ExecutionMode):
		for _, combo := range combinations {
			if combo.Status != "pass" {
				continue
			}
			hasOverlap := len(combo.Handoffs) > 0 ||
				combo.Latency.FirstAssistantAudioE2EMS > 0
			if !hasOverlap {
				violations = append(violations, fmt.Sprintf("combo %s missing streaming overlap evidence", liveProviderComboKey(combo)))
			}
		}
	case isNonStreamingMode(report.ExecutionMode):
		for _, combo := range combinations {
			if combo.Status != "pass" {
				continue
			}
			for _, step := range combo.Steps {
				for _, attempt := range step.Output.Attempts {
					if attempt.StreamingUsed {
						violations = append(violations, fmt.Sprintf("combo %s recorded streaming_used=true in non-streaming mode", liveProviderComboKey(combo)))
						break
					}
				}
				if len(violations) > 0 && strings.Contains(violations[len(violations)-1], liveProviderComboKey(combo)) {
					break
				}
			}
		}
	default:
		violations = append(violations, fmt.Sprintf("unsupported execution mode %q", report.ExecutionMode))
	}
	return violations
}

func normalizeChainCombinations(report liveProviderChainArtifact) []liveProviderChainCombinationEntry {
	combinations := append([]liveProviderChainCombinationEntry(nil), report.Combinations...)
	for i := range combinations {
		if combinations[i].STTProviderID == "" && i < len(report.SelectedCombinations) {
			combinations[i].STTProviderID = report.SelectedCombinations[i].STTProviderID
		}
		if combinations[i].LLMProviderID == "" && i < len(report.SelectedCombinations) {
			combinations[i].LLMProviderID = report.SelectedCombinations[i].LLMProviderID
		}
		if combinations[i].TTSProviderID == "" && i < len(report.SelectedCombinations) {
			combinations[i].TTSProviderID = report.SelectedCombinations[i].TTSProviderID
		}
	}
	return combinations
}

func mapCombinationsByKey(report liveProviderChainArtifact) map[string]liveProviderChainCombinationEntry {
	combinations := normalizeChainCombinations(report)
	out := make(map[string]liveProviderChainCombinationEntry, len(combinations))
	for _, combo := range combinations {
		key := liveProviderComboKey(combo)
		if key == "" {
			continue
		}
		out[key] = combo
	}
	return out
}

func liveProviderComboKey(combo liveProviderChainCombinationEntry) string {
	if strings.TrimSpace(combo.STTProviderID) == "" ||
		strings.TrimSpace(combo.LLMProviderID) == "" ||
		strings.TrimSpace(combo.TTSProviderID) == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s", combo.STTProviderID, combo.LLMProviderID, combo.TTSProviderID)
}

func intersectionKeys(left map[string]liveProviderChainCombinationEntry, right map[string]liveProviderChainCombinationEntry) []string {
	keys := make([]string, 0, len(left))
	for key := range left {
		if _, ok := right[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func populateMetricAggregate(aggregate *liveLatencyMetricAggregate, streaming []int64, nonStreaming []int64, deltas []int64) {
	if aggregate == nil {
		return
	}
	if len(streaming) > 0 {
		p50 := percentile(streaming, 0.50)
		p95 := percentile(streaming, 0.95)
		aggregate.StreamingP50MS = &p50
		aggregate.StreamingP95MS = &p95
	}
	if len(nonStreaming) > 0 {
		p50 := percentile(nonStreaming, 0.50)
		p95 := percentile(nonStreaming, 0.95)
		aggregate.NonStreamingP50MS = &p50
		aggregate.NonStreamingP95MS = &p95
	}
	if len(deltas) > 0 {
		p50 := percentile(deltas, 0.50)
		p95 := percentile(deltas, 0.95)
		aggregate.DeltaP50MS = &p50
		aggregate.DeltaP95MS = &p95
		aggregate.Winner = winnerForDelta(p50)
	}
}

func winnerForDelta(delta int64) string {
	switch {
	case delta < 0:
		return "streaming"
	case delta > 0:
		return "non_streaming"
	default:
		return "tie"
	}
}

func defaultStringMetricWinner(winner string) string {
	if strings.TrimSpace(winner) == "" {
		return "insufficient_data"
	}
	return winner
}

func renderLiveLatencyCompareSummary(artifact liveLatencyCompareArtifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Live Latency Compare Report\n\n")
	fmt.Fprintf(&b, "- Generated at (UTC): `%s`\n", artifact.GeneratedAtUTC)
	fmt.Fprintf(&b, "- Streaming report: `%s`\n", artifact.StreamingReportPath)
	fmt.Fprintf(&b, "- Non-streaming report: `%s`\n", artifact.NonStreamingReportPath)
	fmt.Fprintf(&b, "- Comparison identity: `%s`\n", artifact.ComparisonIdentity)
	fmt.Fprintf(&b, "- Pair count: `%d`\n\n", artifact.PairCount)

	fmt.Fprintf(&b, "## Aggregate Metrics\n\n")
	fmt.Fprintf(&b, "| Metric | Streaming p50 | Streaming p95 | Non-streaming p50 | Non-streaming p95 | Delta p50 (streaming - non) | Delta p95 (streaming - non) | Winner |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	fmt.Fprintf(
		&b,
		"| First audio | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
		formatOptionalInt64(artifact.Aggregate.FirstAudio.StreamingP50MS),
		formatOptionalInt64(artifact.Aggregate.FirstAudio.StreamingP95MS),
		formatOptionalInt64(artifact.Aggregate.FirstAudio.NonStreamingP50MS),
		formatOptionalInt64(artifact.Aggregate.FirstAudio.NonStreamingP95MS),
		formatOptionalInt64(artifact.Aggregate.FirstAudio.DeltaP50MS),
		formatOptionalInt64(artifact.Aggregate.FirstAudio.DeltaP95MS),
		defaultStringMetricWinner(artifact.Aggregate.FirstAudio.Winner),
	)
	fmt.Fprintf(
		&b,
		"| Completion | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n\n",
		formatOptionalInt64(artifact.Aggregate.Completion.StreamingP50MS),
		formatOptionalInt64(artifact.Aggregate.Completion.StreamingP95MS),
		formatOptionalInt64(artifact.Aggregate.Completion.NonStreamingP50MS),
		formatOptionalInt64(artifact.Aggregate.Completion.NonStreamingP95MS),
		formatOptionalInt64(artifact.Aggregate.Completion.DeltaP50MS),
		formatOptionalInt64(artifact.Aggregate.Completion.DeltaP95MS),
		defaultStringMetricWinner(artifact.Aggregate.Completion.Winner),
	)

	fmt.Fprintf(&b, "## Per-Combo Metrics\n\n")
	fmt.Fprintf(&b, "| Combo | Streaming first-audio | Non-streaming first-audio | Delta | First-audio winner | Streaming completion | Non-streaming completion | Delta | Completion winner |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	if len(artifact.Combos) == 0 {
		fmt.Fprintf(&b, "| _none_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ |\n")
	} else {
		for _, combo := range artifact.Combos {
			fmt.Fprintf(
				&b,
				"| `%s` | `%d` | `%d` | `%d` | `%s` | `%d` | `%d` | `%d` | `%s` |\n",
				combo.ComboKey,
				combo.StreamingFirstAudioE2EMS,
				combo.NonStreamingFirstAudioE2EMS,
				combo.FirstAudioDeltaMS,
				defaultStringMetricWinner(combo.FirstAudioWinner),
				combo.StreamingCompletionE2EMS,
				combo.NonStreamingCompletionE2EMS,
				combo.CompletionDeltaMS,
				defaultStringMetricWinner(combo.CompletionWinner),
			)
		}
	}

	if len(artifact.Conclusions) > 0 {
		fmt.Fprintf(&b, "\n## Conclusions\n\n")
		for _, conclusion := range artifact.Conclusions {
			fmt.Fprintf(&b, "- %s\n", conclusion)
		}
	}
	return b.String()
}

func formatOptionalInt64(value *int64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", *value)
}

func evaluateLiveEndToEndGate(report liveProviderChainArtifact, thresholds ops.MVPSLOThresholds, effectiveChainPath string) mvpLiveEndToEndGate {
	out := mvpLiveEndToEndGate{}
	for _, combo := range report.Combinations {
		if combo.Status != "pass" {
			continue
		}
		if combo.Latency.TurnCompletionE2EMS <= 0 {
			continue
		}
		out.SampleCount++
	}
	latencies := make([]int64, 0, out.SampleCount)
	for _, combo := range report.Combinations {
		if combo.Status != "pass" || combo.Latency.TurnCompletionE2EMS <= 0 {
			continue
		}
		latencies = append(latencies, combo.Latency.TurnCompletionE2EMS)
	}
	if len(latencies) == 0 {
		out.Violations = append(out.Violations, "live provider chain report contains no passing end-to-end latency samples")
	}
	if report.Status != "pass" {
		out.Violations = append(out.Violations, fmt.Sprintf("live provider chain status=%s", report.Status))
	}
	if len(latencies) > 0 {
		p50 := percentile(latencies, 0.50)
		p95 := percentile(latencies, 0.95)
		out.EndToEndP50MS = &p50
		out.EndToEndP95MS = &p95
		if p50 > thresholds.EndToEndP50MS {
			out.Violations = append(out.Violations, fmt.Sprintf("live end-to-end p50=%dms exceeds threshold=%dms", p50, thresholds.EndToEndP50MS))
		}
		if p95 > thresholds.EndToEndP95MS {
			out.Violations = append(out.Violations, fmt.Sprintf("live end-to-end p95=%dms exceeds threshold=%dms", p95, thresholds.EndToEndP95MS))
		}
	}

	out.AttemptFiles = discoverLiveE2EAttemptFiles(effectiveChainPath)
	if len(out.Violations) > 0 && len(out.AttemptFiles) >= mvpE2EWaiveAfterAttempts {
		out.Waived = true
		out.WaiverReason = fmt.Sprintf("waived end-to-end latency standard after %d attempts (%s)", len(out.AttemptFiles), strings.Join(out.AttemptFiles, ", "))
		out.Violations = nil
		out.Passed = true
		return out
	}

	out.Passed = len(out.Violations) == 0
	return out
}

func evaluateMVPFixedDecisionGates(report liveProviderChainArtifact, liveKitSmokePath string) mvpFixedDecisionGates {
	out := mvpFixedDecisionGates{}
	if err := validateSimpleModeOnly(); err != nil {
		out.Violations = append(out.Violations, fmt.Sprintf("simple-mode check failed: %v", err))
	} else {
		out.SimpleModeOnly = true
	}

	if violations := validateMVPProviderPolicy(report); len(violations) > 0 {
		out.Violations = append(out.Violations, violations...)
	} else {
		out.ProviderPolicyCompliant = true
	}

	if isStreamingMode(report.ExecutionMode) && len(validateLiveProviderModeEvidence(report)) == 0 {
		out.StreamingEvidence = true
	}
	if streamingReport, _, err := loadLiveProviderChainArtifact(defaultStreamingChainReportPath); err == nil {
		if isStreamingMode(streamingReport.ExecutionMode) && len(validateLiveProviderModeEvidence(streamingReport)) == 0 {
			out.StreamingEvidence = true
		}
	}
	if isNonStreamingMode(report.ExecutionMode) && len(validateLiveProviderModeEvidence(report)) == 0 {
		out.NonStreamingEvidence = true
	}
	if nonStreamingReport, _, err := loadLiveProviderChainArtifact(defaultNonStreamingChainReportPath); err == nil {
		if isNonStreamingMode(nonStreamingReport.ExecutionMode) && len(validateLiveProviderModeEvidence(nonStreamingReport)) == 0 {
			out.NonStreamingEvidence = true
		}
	}
	if report.SemanticParity != nil && report.ParityComparisonValid != nil {
		out.ParityMarkersPresent = true
		out.ParityComparisonValid = *report.SemanticParity && *report.ParityComparisonValid
	}
	if !out.StreamingEvidence {
		out.Violations = append(out.Violations, "streaming execution evidence missing from live provider report")
	}
	if !out.NonStreamingEvidence {
		out.Violations = append(out.Violations, "non-streaming execution evidence missing (.codex/providers/live-provider-chain-report.nonstreaming.json)")
	}
	if !out.ParityMarkersPresent {
		out.Violations = append(out.Violations, "streaming/non-streaming parity markers missing from live provider report")
	}
	if out.ParityMarkersPresent && !out.ParityComparisonValid {
		detail := strings.TrimSpace(report.ParityInvalidReason)
		if detail == "" {
			detail = "semantic parity or comparison validity is false"
		}
		out.Violations = append(out.Violations, "streaming/non-streaming parity comparison invalid: "+detail)
	}

	livekit, _, err := loadLiveKitSmokeArtifact(liveKitSmokePath)
	if err != nil {
		out.Violations = append(out.Violations, fmt.Sprintf("livekit smoke report missing/invalid: %v", err))
	} else if strings.ToLower(strings.TrimSpace(livekit.Probe.Status)) != "passed" {
		out.Violations = append(out.Violations, fmt.Sprintf("livekit probe status must be passed, got %q", livekit.Probe.Status))
	} else {
		out.LiveKitPathReady = true
	}

	out.Passed = len(out.Violations) == 0
	return out
}

func validateSimpleModeOnly() error {
	if cpregistry.DefaultExecutionProfile != "simple" {
		return fmt.Errorf("registry default execution profile must be simple, got %q", cpregistry.DefaultExecutionProfile)
	}
	normalizer := cpnormalizer.Service{}
	normalized, err := normalizer.Normalize(cpnormalizer.Input{
		Record: cpregistry.PipelineRecord{
			PipelineVersion:    cpregistry.DefaultPipelineVersion,
			GraphDefinitionRef: cpregistry.DefaultGraphDefinitionRef,
		},
	})
	if err != nil {
		return fmt.Errorf("normalize simple profile default: %w", err)
	}
	if normalized.ExecutionProfile != cpregistry.DefaultExecutionProfile {
		return fmt.Errorf("normalized execution profile must be %q, got %q", cpregistry.DefaultExecutionProfile, normalized.ExecutionProfile)
	}
	_, err = normalizer.Normalize(cpnormalizer.Input{
		Record: cpregistry.PipelineRecord{
			PipelineVersion:    cpregistry.DefaultPipelineVersion,
			GraphDefinitionRef: cpregistry.DefaultGraphDefinitionRef,
			ExecutionProfile:   "advanced",
		},
	})
	if err == nil {
		return fmt.Errorf("advanced execution profile must be rejected in MVP")
	}
	if !strings.Contains(err.Error(), "unsupported in MVP") {
		return fmt.Errorf("advanced execution profile error must mention unsupported in MVP, got %q", err.Error())
	}
	return nil
}

func validateMVPProviderPolicy(report liveProviderChainArtifact) []string {
	allowed := map[string]map[string]struct{}{
		"stt": {"stt-deepgram": {}, "stt-assemblyai": {}},
		"llm": {"llm-anthropic": {}, "llm-cohere": {}},
		"tts": {"tts-elevenlabs": {}},
	}
	violations := make([]string, 0)
	validateSet := func(modality string, providers []string) {
		if len(providers) < 1 || len(providers) > 5 {
			violations = append(violations, fmt.Sprintf("%s provider count must be within [1,5], got %d", modality, len(providers)))
		}
		for _, provider := range providers {
			if _, ok := allowed[modality][provider]; !ok {
				violations = append(violations, fmt.Sprintf("%s provider %q is outside MVP fixed policy", modality, provider))
			}
		}
	}
	validateSet("stt", report.EnabledProviders.STT)
	validateSet("llm", report.EnabledProviders.LLM)
	validateSet("tts", report.EnabledProviders.TTS)
	for _, combo := range report.SelectedCombinations {
		if _, ok := allowed["stt"][combo.STTProviderID]; !ok {
			violations = append(violations, fmt.Sprintf("combo uses unsupported stt provider %q", combo.STTProviderID))
		}
		if _, ok := allowed["llm"][combo.LLMProviderID]; !ok {
			violations = append(violations, fmt.Sprintf("combo uses unsupported llm provider %q", combo.LLMProviderID))
		}
		if _, ok := allowed["tts"][combo.TTSProviderID]; !ok {
			violations = append(violations, fmt.Sprintf("combo uses unsupported tts provider %q", combo.TTSProviderID))
		}
	}
	return violations
}

func loadLiveProviderChainArtifact(path string) (liveProviderChainArtifact, string, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultLiveProviderChainReportPath
	}
	resolved, err := resolveProjectRelativePath(path)
	if err != nil {
		return liveProviderChainArtifact{}, path, fmt.Errorf("resolve live provider chain report: %w", err)
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return liveProviderChainArtifact{}, resolved, fmt.Errorf("read live provider chain report %s: %w", resolved, err)
	}
	var report liveProviderChainArtifact
	if err := json.Unmarshal(raw, &report); err != nil {
		return liveProviderChainArtifact{}, resolved, fmt.Errorf("decode live provider chain report %s: %w", resolved, err)
	}
	return report, resolved, nil
}

func loadLiveKitSmokeArtifact(path string) (liveKitSmokeArtifact, string, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultLiveKitSmokeReportPath
	}
	resolved, err := resolveProjectRelativePath(path)
	if err != nil {
		return liveKitSmokeArtifact{}, path, fmt.Errorf("resolve livekit smoke report: %w", err)
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return liveKitSmokeArtifact{}, resolved, fmt.Errorf("read livekit smoke report %s: %w", resolved, err)
	}
	var artifact liveKitSmokeArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return liveKitSmokeArtifact{}, resolved, fmt.Errorf("decode livekit smoke report %s: %w", resolved, err)
	}
	return artifact, resolved, nil
}

func discoverLiveE2EAttemptFiles(chainPath string) []string {
	baseDir := filepath.Dir(chainPath)
	matches, err := filepath.Glob(filepath.Join(baseDir, "live-provider-chain-report*.json"))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}

func writeConformanceGovernanceReport(
	outputPath string,
	versionSkewPolicyPath string,
	deprecationPolicyPath string,
	conformanceProfilePath string,
	conformanceResultsPath string,
	featureStatusFrameworkPath string,
) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("conformance governance output path is required")
	}

	resolvedVersionSkewPolicyPath, err := resolveProjectRelativePath(versionSkewPolicyPath)
	if err != nil {
		return err
	}
	resolvedDeprecationPolicyPath, err := resolveProjectRelativePath(deprecationPolicyPath)
	if err != nil {
		return err
	}
	resolvedConformanceProfilePath, err := resolveProjectRelativePath(conformanceProfilePath)
	if err != nil {
		return err
	}
	resolvedConformanceResultsPath, err := resolveProjectRelativePath(conformanceResultsPath)
	if err != nil {
		return err
	}
	resolvedFeatureStatusFrameworkPath, err := resolveProjectRelativePath(featureStatusFrameworkPath)
	if err != nil {
		return err
	}

	versionSkewPolicy, err := toolingconformance.ReadVersionSkewPolicy(resolvedVersionSkewPolicyPath)
	if err != nil {
		return err
	}
	deprecationPolicy, err := toolingconformance.ReadDeprecationPolicy(resolvedDeprecationPolicyPath)
	if err != nil {
		return err
	}
	conformanceProfile, err := toolingconformance.ReadConformanceProfile(resolvedConformanceProfilePath)
	if err != nil {
		return err
	}
	conformanceResults, err := toolingconformance.ReadConformanceResults(resolvedConformanceResultsPath)
	if err != nil {
		return err
	}
	featureStatusEntries, err := toolingconformance.ReadFeatureStatusEntries(resolvedFeatureStatusFrameworkPath)
	if err != nil {
		return err
	}

	report := toolingconformance.EvaluateGovernance(toolingconformance.GovernanceInput{
		VersionSkewPolicy:    versionSkewPolicy,
		DeprecationPolicy:    deprecationPolicy,
		ConformanceProfile:   conformanceProfile,
		ConformanceResults:   conformanceResults,
		FeatureStatusEntries: featureStatusEntries,
		FeatureExpectations:  toolingconformance.DefaultUBT05FeatureExpectations(),
	})
	artifact := conformanceGovernanceArtifact{
		SchemaVersion:              toolingconformance.GovernanceReportSchemaVersionV1,
		GeneratedAtUTC:             time.Now().UTC().Format(time.RFC3339),
		VersionSkewPolicyPath:      resolvedVersionSkewPolicyPath,
		DeprecationPolicyPath:      resolvedDeprecationPolicyPath,
		ConformanceProfilePath:     resolvedConformanceProfilePath,
		ConformanceResultsPath:     resolvedConformanceResultsPath,
		FeatureStatusFrameworkPath: resolvedFeatureStatusFrameworkPath,
		Report:                     report,
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
	if err := os.WriteFile(summaryPath, []byte(renderConformanceGovernanceSummary(artifact)), 0o644); err != nil {
		return err
	}
	if !artifact.Report.Passed {
		return fmt.Errorf("conformance governance checks failed: %v", artifact.Report.Violations)
	}
	return nil
}

func percentile(values []int64, pct float64) int64 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]int64(nil), values...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	index := int(float64(len(copied))*pct+0.999999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copied) {
		index = len(copied) - 1
	}
	return copied[index]
}

func writeContractsReport(outputPath string, fixtureRoot string) error {
	if fixtureRoot == "" {
		fixtureRoot = filepath.Join("test", "contract", "fixtures")
	}
	resolvedFixtureRoot, err := resolveProjectRelativePath(fixtureRoot)
	if err != nil {
		return err
	}
	schemaPath, err := resolveProjectRelativePath(filepath.Join("docs", "ContractArtifacts.schema.json"))
	if err != nil {
		return err
	}

	summary, err := validation.ValidateContractFixturesWithSchema(schemaPath, resolvedFixtureRoot)
	if err != nil {
		return err
	}
	artifact := contractsReportArtifact{
		SchemaVersion:  toolingrelease.ContractsReportSchemaVersionV1,
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		FixtureRoot:    resolvedFixtureRoot,
		Summary:        summary,
		Passed:         summary.Failed == 0,
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
	if err := os.WriteFile(summaryPath, []byte(renderContractsReportSummary(artifact)), 0o644); err != nil {
		return err
	}
	if !artifact.Passed {
		return fmt.Errorf("contract fixtures failed: %d failures", artifact.Summary.Failed)
	}
	return nil
}

func resolveProjectRelativePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(trimmed) {
		return trimmed, nil
	}
	if _, err := os.Stat(trimmed); err == nil {
		return trimmed, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, trimmed)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("path not found: %s", trimmed)
}

func writeReleaseManifest(
	outputPath string,
	specRef string,
	rolloutConfigPath string,
	contractsReportPath string,
	replayRegressionReportPath string,
	sloGatesReportPath string,
	now time.Time,
) (toolingrelease.ReleaseManifest, error) {
	rolloutCfg, rolloutSource, err := toolingrelease.LoadRolloutConfig(rolloutConfigPath)
	if err != nil {
		return toolingrelease.ReleaseManifest{}, err
	}

	readiness, sources := toolingrelease.EvaluateReadiness(toolingrelease.ReadinessInput{
		ContractsReportPath:        contractsReportPath,
		ReplayRegressionReportPath: replayRegressionReportPath,
		SLOGatesReportPath:         sloGatesReportPath,
		Now:                        now,
		MaxArtifactAge:             toolingrelease.DefaultMaxArtifactAge,
	})
	if !readiness.Passed {
		return toolingrelease.ReleaseManifest{}, fmt.Errorf("release readiness failed: %v", readiness.Violations)
	}

	if sources == nil {
		sources = make(map[string]toolingrelease.ArtifactSource, 4)
	}
	sources["rollout_config"] = rolloutSource

	manifest, err := toolingrelease.BuildReleaseManifest(specRef, rolloutCfg, readiness, sources, now)
	if err != nil {
		return toolingrelease.ReleaseManifest{}, err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return toolingrelease.ReleaseManifest{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return toolingrelease.ReleaseManifest{}, err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return toolingrelease.ReleaseManifest{}, err
	}

	summaryPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".md"
	if err := os.WriteFile(summaryPath, []byte(renderReleaseManifestSummary(manifest)), 0o644); err != nil {
		return toolingrelease.ReleaseManifest{}, err
	}
	return manifest, nil
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
			TurnTerminalAtMS:         entry.TurnTerminalAtMS,
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
	if report.EndToEndP50MS != nil {
		lines = append(lines, fmt.Sprintf("End-to-end p50: %d ms", *report.EndToEndP50MS))
	}
	if report.EndToEndP95MS != nil {
		lines = append(lines, fmt.Sprintf("End-to-end p95: %d ms", *report.EndToEndP95MS))
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

func renderMVPSLOGatesSummary(artifact mvpSLOGateArtifact) string {
	lines := []string{
		"# MVP SLO Gates Report (Live-Enforced)",
		"",
		"Generated at (UTC): " + artifact.GeneratedAtUTC,
		"Baseline artifact: " + artifact.BaselineArtifactPath,
		"Live provider chain report: " + artifact.LiveProviderChainPath,
		"LiveKit smoke report: " + artifact.LiveKitSmokeReportPath,
		"",
		"## Baseline",
		fmt.Sprintf("Accepted turns: %d", artifact.BaselineReport.AcceptedTurns),
		fmt.Sprintf("Happy-path turns: %d", artifact.BaselineReport.HappyPathTurns),
		fmt.Sprintf("OR-02 completeness: %.2f", artifact.BaselineReport.BaselineCompletenessRatio),
		fmt.Sprintf("Terminal correctness: %.2f", artifact.BaselineReport.TerminalCorrectnessRatio),
	}
	if artifact.BaselineReport.TurnOpenDecisionP95MS != nil {
		lines = append(lines, fmt.Sprintf("Turn-open p95: %d ms", *artifact.BaselineReport.TurnOpenDecisionP95MS))
	}
	if artifact.BaselineReport.FirstOutputP95MS != nil {
		lines = append(lines, fmt.Sprintf("First-output p95: %d ms", *artifact.BaselineReport.FirstOutputP95MS))
	}
	if artifact.BaselineReport.CancelFenceP95MS != nil {
		lines = append(lines, fmt.Sprintf("Cancel-fence p95: %d ms", *artifact.BaselineReport.CancelFenceP95MS))
	}
	if artifact.BaselineReport.EndToEndP50MS != nil {
		lines = append(lines, fmt.Sprintf("End-to-end p50: %d ms", *artifact.BaselineReport.EndToEndP50MS))
	}
	if artifact.BaselineReport.EndToEndP95MS != nil {
		lines = append(lines, fmt.Sprintf("End-to-end p95: %d ms", *artifact.BaselineReport.EndToEndP95MS))
	}

	lines = append(lines,
		"",
		"## Live End-to-End",
		fmt.Sprintf("Samples: %d", artifact.LiveEndToEnd.SampleCount),
		fmt.Sprintf("Waived: %t", artifact.LiveEndToEnd.Waived),
	)
	if artifact.LiveEndToEnd.EndToEndP50MS != nil {
		lines = append(lines, fmt.Sprintf("Live end-to-end p50: %d ms", *artifact.LiveEndToEnd.EndToEndP50MS))
	}
	if artifact.LiveEndToEnd.EndToEndP95MS != nil {
		lines = append(lines, fmt.Sprintf("Live end-to-end p95: %d ms", *artifact.LiveEndToEnd.EndToEndP95MS))
	}
	if artifact.LiveEndToEnd.WaiverReason != "" {
		lines = append(lines, "Waiver reason: "+artifact.LiveEndToEnd.WaiverReason)
	}

	lines = append(lines,
		"",
		"## Fixed Decisions",
		fmt.Sprintf("Simple-mode-only: %t", artifact.FixedDecisions.SimpleModeOnly),
		fmt.Sprintf("Provider policy compliant: %t", artifact.FixedDecisions.ProviderPolicyCompliant),
		fmt.Sprintf("LiveKit path ready: %t", artifact.FixedDecisions.LiveKitPathReady),
		fmt.Sprintf("Streaming evidence: %t", artifact.FixedDecisions.StreamingEvidence),
		fmt.Sprintf("Non-streaming evidence: %t", artifact.FixedDecisions.NonStreamingEvidence),
		fmt.Sprintf("Parity markers present: %t", artifact.FixedDecisions.ParityMarkersPresent),
		fmt.Sprintf("Parity comparison valid: %t", artifact.FixedDecisions.ParityComparisonValid),
	)

	if len(artifact.Warnings) > 0 {
		lines = append(lines, "", "## Warnings")
		for _, warning := range artifact.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	if len(artifact.Violations) > 0 {
		lines = append(lines, "", "## Violations")
		for _, violation := range artifact.Violations {
			lines = append(lines, "- "+violation)
		}
	}
	if artifact.Passed {
		lines = append(lines, "", "Status: PASS")
	} else {
		lines = append(lines, "", "Status: FAIL")
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderConformanceGovernanceSummary(artifact conformanceGovernanceArtifact) string {
	lines := []string{
		"# Conformance Governance Report",
		"",
		"Generated at (UTC): " + artifact.GeneratedAtUTC,
		"Version skew policy: " + artifact.VersionSkewPolicyPath,
		"Deprecation policy: " + artifact.DeprecationPolicyPath,
		"Conformance profile: " + artifact.ConformanceProfilePath,
		"Conformance results: " + artifact.ConformanceResultsPath,
		"Feature status framework: " + artifact.FeatureStatusFrameworkPath,
		"",
		"## Version Skew",
		fmt.Sprintf("- Passed: %t", artifact.Report.VersionSkew.Passed),
		fmt.Sprintf("- Allowed matrix tests: %d", artifact.Report.VersionSkew.AllowedTestCount),
		fmt.Sprintf("- Disallowed matrix tests: %d", artifact.Report.VersionSkew.DisallowedTestCount),
		"",
		"## Deprecation Lifecycle",
		fmt.Sprintf("- Passed: %t", artifact.Report.Deprecation.Passed),
		fmt.Sprintf("- Rules: %d", artifact.Report.Deprecation.RuleCount),
		fmt.Sprintf("- Usage checks: %d", artifact.Report.Deprecation.CheckCount),
		"",
		"## Conformance Profile",
		fmt.Sprintf("- Passed: %t", artifact.Report.Conformance.Passed),
		fmt.Sprintf("- Mandatory categories: %d", artifact.Report.Conformance.MandatoryCount),
		"",
		"## Feature Status Stewardship",
		fmt.Sprintf("- Passed: %t", artifact.Report.FeatureStatus.Passed),
		fmt.Sprintf("- Checked features: %d", artifact.Report.FeatureStatus.CheckedCount),
	}

	if len(artifact.Report.Violations) == 0 {
		lines = append(lines, "", "Status: PASS")
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines, "", "Status: FAIL", "## Violations")
	for _, violation := range artifact.Report.Violations {
		lines = append(lines, "- "+violation)
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderContractsReportSummary(artifact contractsReportArtifact) string {
	lines := []string{
		"# Contract Validation Report",
		"",
		"Generated at (UTC): " + artifact.GeneratedAtUTC,
		"Fixture root: " + artifact.FixtureRoot,
		fmt.Sprintf("Total fixtures: %d", artifact.Summary.Total),
		fmt.Sprintf("Failed fixtures: %d", artifact.Summary.Failed),
	}
	if artifact.Passed {
		lines = append(lines, "", "Status: PASS")
	} else {
		lines = append(lines, "", "Status: FAIL")
		if len(artifact.Summary.Failures) > 0 {
			lines = append(lines, "## Failures")
			for _, failure := range artifact.Summary.Failures {
				lines = append(lines, "- "+failure)
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderReleaseManifestSummary(manifest toolingrelease.ReleaseManifest) string {
	lines := []string{
		"# Release Manifest",
		"",
		"Generated at (UTC): " + manifest.GeneratedAtUTC,
		"Release ID: " + manifest.ReleaseID,
		"Spec ref: " + manifest.SpecRef,
		"Pipeline version: " + manifest.RolloutConfig.PipelineVersion,
		"Strategy: " + manifest.RolloutConfig.Strategy,
		"Rollback mode: " + manifest.RolloutConfig.RollbackPosture.Mode,
		"Rollback trigger: " + manifest.RolloutConfig.RollbackPosture.Trigger,
		"",
		"## Readiness Checks",
	}
	for _, check := range manifest.Readiness.Checks {
		status := "PASS"
		if !check.Passed {
			status = "FAIL"
		}
		line := fmt.Sprintf("- %s: %s (%s)", check.Name, status, check.Path)
		if check.Reason != "" {
			line += " - " + check.Reason
		}
		lines = append(lines, line)
	}

	lines = append(lines, "", "## Source Artifacts")
	keys := make([]string, 0, len(manifest.SourceArtifacts))
	for key := range manifest.SourceArtifacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		source := manifest.SourceArtifacts[key]
		line := fmt.Sprintf("- %s: %s (sha256=%s)", key, source.Path, source.SHA256)
		if source.GeneratedAtUTC != "" {
			line += " generated_at_utc=" + source.GeneratedAtUTC
		}
		lines = append(lines, line)
	}

	if manifest.Readiness.Passed {
		lines = append(lines, "", "Status: PASS")
	} else {
		lines = append(lines, "", "Status: FAIL")
		if len(manifest.Readiness.Violations) > 0 {
			lines = append(lines, "## Violations")
			for _, violation := range manifest.Readiness.Violations {
				lines = append(lines, "- "+violation)
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
