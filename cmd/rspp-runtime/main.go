package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/distribution"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/bootstrap"
	livekittransport "github.com/tiger/realtime-speech-pipeline/transports/livekit"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now); err != nil {
		fmt.Fprintf(os.Stderr, "rspp-runtime: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, _ io.Writer, now func() time.Time) error {
	cleanupTelemetry, err := setupRuntimeTelemetry()
	if err != nil {
		return err
	}
	defer cleanupTelemetry()

	if len(args) == 0 || args[0] == "bootstrap-providers" {
		return runProviderBootstrap(stdout)
	}

	switch args[0] {
	case "retention-sweep":
		return runRetentionSweep(args[1:], stdout, now)
	case "livekit":
		return livekittransport.RunCLI(args[1:], stdout, now)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stdout)
		return fmt.Errorf("unsupported command %q", args[0])
	}
}

func setupRuntimeTelemetry() (func(), error) {
	previous := telemetry.DefaultEmitter()

	pipeline, err := telemetry.NewPipelineFromEnv()
	if err != nil {
		return nil, fmt.Errorf("runtime telemetry setup failed: %w", err)
	}
	if pipeline == nil {
		return func() {
			telemetry.SetDefaultEmitter(previous)
		}, nil
	}

	telemetry.SetDefaultEmitter(pipeline)
	return func() {
		_ = pipeline.Close()
		telemetry.SetDefaultEmitter(previous)
	}, nil
}

func runProviderBootstrap(stdout io.Writer) error {
	runtimeProviders, err := bootstrap.BuildMVPProviders()
	if err != nil {
		return fmt.Errorf("provider bootstrap failed: %w", err)
	}
	summary, err := bootstrap.Summary(runtimeProviders.Catalog)
	if err != nil {
		return fmt.Errorf("provider summary failed: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "rspp-runtime: %s\n", summary)
	return nil
}

type retentionStoreArtifact struct {
	GeneratedAtUTC string                        `json:"generated_at_utc,omitempty"`
	Records        []replay.ReplayArtifactRecord `json:"records"`
}

type retentionPolicyArtifact struct {
	DefaultPolicy  *retentionPolicyArtifactPolicy           `json:"default_policy,omitempty"`
	TenantPolicies map[string]retentionPolicyArtifactPolicy `json:"tenant_policies,omitempty"`
}

type retentionPolicyArtifactPolicy struct {
	TenantID              string                          `json:"tenant_id,omitempty"`
	DefaultRetentionMS    int64                           `json:"default_retention_ms,omitempty"`
	PIIRetentionLimitMS   int64                           `json:"pii_retention_limit_ms,omitempty"`
	PHIRetentionLimitMS   int64                           `json:"phi_retention_limit_ms,omitempty"`
	MaxRetentionByClassMS map[eventabi.PayloadClass]int64 `json:"max_retention_by_class_ms,omitempty"`
}

func (p retentionPolicyArtifactPolicy) toRuntimePolicy() replay.RetentionPolicy {
	return replay.RetentionPolicy{
		TenantID:              p.TenantID,
		DefaultRetentionMS:    p.DefaultRetentionMS,
		PIIRetentionLimitMS:   p.PIIRetentionLimitMS,
		PHIRetentionLimitMS:   p.PHIRetentionLimitMS,
		MaxRetentionByClassMS: p.MaxRetentionByClassMS,
	}
}

type retentionSweepTenantResult struct {
	TenantID           string         `json:"tenant_id"`
	EvaluatedArtifacts int            `json:"evaluated_artifacts"`
	ExpiredArtifacts   int            `json:"expired_artifacts"`
	DeletedArtifacts   int            `json:"deleted_artifacts"`
	DeletedByClass     map[string]int `json:"deleted_by_class"`
}

type retentionSweepRunResult struct {
	RunIndex             int                          `json:"run_index"`
	RunAtMS              int64                        `json:"run_at_ms"`
	PolicySource         string                       `json:"policy_source"`
	PolicyFallbackReason string                       `json:"policy_fallback_reason,omitempty"`
	TenantResults        []retentionSweepTenantResult `json:"tenant_results"`
	TotalEvaluated       int                          `json:"total_evaluated"`
	TotalExpired         int                          `json:"total_expired"`
	TotalDeleted         int                          `json:"total_deleted"`
	DeletedByClass       map[string]int               `json:"deleted_by_class"`
}

type retentionSweepReport struct {
	GeneratedAtUTC       string                    `json:"generated_at_utc"`
	StorePath            string                    `json:"store_path"`
	PolicyPath           string                    `json:"policy_path,omitempty"`
	PolicySource         string                    `json:"policy_source"`
	PolicyFallbackReason string                    `json:"policy_fallback_reason,omitempty"`
	Tenants              []string                  `json:"tenants"`
	Runs                 int                       `json:"runs"`
	IntervalMS           int64                     `json:"interval_ms"`
	RunResults           []retentionSweepRunResult `json:"run_results"`
	FinalRecords         int                       `json:"final_records"`
}

type retentionSweepPolicyErrorCode string
type retentionPolicySource string

const (
	retentionSweepPolicyReadErrorCode            retentionSweepPolicyErrorCode = "RETENTION_POLICY_READ_ERROR"
	retentionSweepPolicyDecodeErrorCode          retentionSweepPolicyErrorCode = "RETENTION_POLICY_DECODE_ERROR"
	retentionSweepPolicyArtifactInvalidErrorCode retentionSweepPolicyErrorCode = "RETENTION_POLICY_ARTIFACT_INVALID"
	retentionSweepPolicyTenantMismatchErrorCode  retentionSweepPolicyErrorCode = "RETENTION_POLICY_TENANT_MISMATCH"
	retentionSweepPolicyValidationErrorCode      retentionSweepPolicyErrorCode = "RETENTION_POLICY_VALIDATION_ERROR"
	defaultRetentionPolicyTenantID                                             = "retention-policy-default"

	retentionPolicySourceArtifactOverride   retentionPolicySource = "policy_artifact"
	retentionPolicySourceCPDistributionFile retentionPolicySource = "cp_distribution_file"
	retentionPolicySourceCPDistributionHTTP retentionPolicySource = "cp_distribution_http"
	retentionPolicySourceDefaultFallback    retentionPolicySource = "default_fallback"
	retentionPolicySourceMixed              retentionPolicySource = "mixed"

	retentionPolicyFallbackReasonCPDistributionUnconfigured = "cp_distribution_unconfigured"
	retentionPolicyFallbackReasonCPDistributionFetchFailed  = "cp_distribution_fetch_failed"
	retentionPolicyFallbackReasonCPDistributionStale        = "cp_distribution_snapshot_stale"
	retentionPolicyFallbackReasonCPDistributionInvalid      = "cp_distribution_policy_invalid"
	retentionPolicyFallbackReasonMixed                      = "mixed"
)

type retentionSweepPolicyError struct {
	Code   retentionSweepPolicyErrorCode
	Detail string
	Err    error
}

type normalizedRetentionPolicyArtifact struct {
	DefaultPolicy  *replay.RetentionPolicy
	TenantPolicies map[string]replay.RetentionPolicy
}

func (e retentionSweepPolicyError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Detail, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

func (e retentionSweepPolicyError) Unwrap() error {
	return e.Err
}

func runRetentionSweep(args []string, stdout io.Writer, now func() time.Time) error {
	fs := flag.NewFlagSet("retention-sweep", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	storePath := fs.String("store", "", "path to replay retention store artifact json")
	reportPath := fs.String("report", filepath.Join(".codex", "replay", "retention-sweep-report.json"), "path to write retention sweep report json")
	policyPath := fs.String("policy", "", "optional path to tenant retention policy artifact json")
	tenantsRaw := fs.String("tenants", "", "comma-separated tenant ids")
	nowMSFlag := fs.Int64("now-ms", -1, "optional deterministic now_ms override")
	intervalMS := fs.Int64("interval-ms", 0, "interval between runs in milliseconds (0 for no delay)")
	runs := fs.Int("runs", 1, "number of scheduled runs to execute (must be >=1)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*storePath) == "" {
		return fmt.Errorf("retention-sweep requires -store")
	}
	if strings.TrimSpace(*tenantsRaw) == "" {
		return fmt.Errorf("retention-sweep requires -tenants")
	}
	if *runs < 1 {
		return fmt.Errorf("retention-sweep requires runs >=1")
	}
	if *intervalMS < 0 {
		return fmt.Errorf("retention-sweep requires interval-ms >=0")
	}

	tenants := parseTenantList(*tenantsRaw)
	if len(tenants) == 0 {
		return fmt.Errorf("retention-sweep requires at least one tenant")
	}

	store, err := loadRetentionStore(*storePath)
	if err != nil {
		return err
	}

	runResults := make([]retentionSweepRunResult, 0, *runs)
	for runIndex := 1; runIndex <= *runs; runIndex++ {
		runAtMS := computeRunAtMS(*nowMSFlag, *intervalMS, runIndex, now)
		resolver, policySource, fallbackReason, err := loadRetentionResolver(*policyPath)
		if err != nil {
			return err
		}
		runResult := retentionSweepRunResult{
			RunIndex:             runIndex,
			RunAtMS:              runAtMS,
			PolicySource:         string(policySource),
			PolicyFallbackReason: fallbackReason,
			TenantResults:        make([]retentionSweepTenantResult, 0, len(tenants)),
			DeletedByClass:       newRetentionClassCounters(),
		}
		for _, tenantID := range tenants {
			sweepResult, err := replay.EnforceTenantRetentionWithResolverDetailed(store, resolver, tenantID, runAtMS)
			if err != nil {
				return fmt.Errorf("retention sweep tenant %s run %d: %w", tenantID, runIndex, err)
			}
			tenantDeletedByClass := newRetentionClassCounters()
			mergeRetentionClassCounters(tenantDeletedByClass, sweepResult.DeletedByClass)
			mergeRetentionClassCounters(runResult.DeletedByClass, sweepResult.DeletedByClass)

			tenantResult := retentionSweepTenantResult{
				TenantID:           tenantID,
				EvaluatedArtifacts: sweepResult.Summary.EvaluatedArtifacts,
				ExpiredArtifacts:   sweepResult.Summary.ExpiredArtifacts,
				DeletedArtifacts:   sweepResult.Summary.DeletedArtifacts,
				DeletedByClass:     tenantDeletedByClass,
			}
			runResult.TenantResults = append(runResult.TenantResults, tenantResult)
			runResult.TotalEvaluated += tenantResult.EvaluatedArtifacts
			runResult.TotalExpired += tenantResult.ExpiredArtifacts
			runResult.TotalDeleted += tenantResult.DeletedArtifacts
		}
		runResults = append(runResults, runResult)

		if *intervalMS > 0 && runIndex < *runs {
			time.Sleep(time.Duration(*intervalMS) * time.Millisecond)
		}
	}

	finalRecords := store.Snapshot()
	if err := writeRetentionStore(*storePath, finalRecords, now); err != nil {
		return err
	}

	report := retentionSweepReport{
		GeneratedAtUTC:       now().UTC().Format(time.RFC3339),
		StorePath:            *storePath,
		PolicyPath:           strings.TrimSpace(*policyPath),
		PolicySource:         summarizePolicySource(runResults),
		PolicyFallbackReason: summarizePolicyFallbackReason(runResults),
		Tenants:              tenants,
		Runs:                 *runs,
		IntervalMS:           *intervalMS,
		RunResults:           runResults,
		FinalRecords:         len(finalRecords),
	}
	if err := writeJSONArtifact(*reportPath, report); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "rspp-runtime retention-sweep: report=%s runs=%d tenants=%d final_records=%d\n", *reportPath, report.Runs, len(report.Tenants), report.FinalRecords)
	return nil
}

func computeRunAtMS(nowMS int64, intervalMS int64, runIndex int, now func() time.Time) int64 {
	if nowMS >= 0 {
		return nowMS + int64(runIndex-1)*intervalMS
	}
	return now().UnixMilli()
}

func parseTenantList(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	tenants := make([]string, 0, len(parts))
	for _, part := range parts {
		tenant := strings.TrimSpace(part)
		if tenant == "" {
			continue
		}
		if _, ok := seen[tenant]; ok {
			continue
		}
		seen[tenant] = struct{}{}
		tenants = append(tenants, tenant)
	}
	sort.Strings(tenants)
	return tenants
}

func retentionPayloadClasses() []eventabi.PayloadClass {
	return []eventabi.PayloadClass{
		eventabi.PayloadAudioRaw,
		eventabi.PayloadTextRaw,
		eventabi.PayloadPII,
		eventabi.PayloadPHI,
		eventabi.PayloadDerivedSummary,
		eventabi.PayloadMetadata,
	}
}

func newRetentionClassCounters() map[string]int {
	counters := make(map[string]int, len(retentionPayloadClasses()))
	for _, class := range retentionPayloadClasses() {
		counters[string(class)] = 0
	}
	return counters
}

func mergeRetentionClassCounters(target map[string]int, source map[eventabi.PayloadClass]int) {
	for _, class := range retentionPayloadClasses() {
		target[string(class)] += source[class]
	}
}

func loadRetentionStore(path string) (*replay.InMemoryArtifactStore, error) {
	store := replay.NewInMemoryArtifactStore()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, fmt.Errorf("read retention store artifact %s: %w", path, err)
	}
	if len(raw) == 0 {
		return store, nil
	}

	var artifact retentionStoreArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil, fmt.Errorf("decode retention store artifact %s: %w", path, err)
	}
	for _, record := range artifact.Records {
		if err := store.Add(record); err != nil {
			return nil, fmt.Errorf("load retention store artifact record %s: %w", record.ArtifactID, err)
		}
	}
	return store, nil
}

func writeRetentionStore(path string, records []replay.ReplayArtifactRecord, now func() time.Time) error {
	artifact := retentionStoreArtifact{
		GeneratedAtUTC: now().UTC().Format(time.RFC3339),
		Records:        records,
	}
	return writeJSONArtifact(path, artifact)
}

func loadRetentionResolver(policyPath string) (replay.RetentionPolicyResolver, retentionPolicySource, string, error) {
	fallback := replay.BackendRetentionPolicyResolver{
		Fallback: replay.StaticRetentionPolicyResolver{},
	}

	trimmedPath := strings.TrimSpace(policyPath)
	if trimmedPath != "" {
		resolver, err := loadRetentionResolverFromArtifactPath(trimmedPath)
		if err != nil {
			return nil, "", "", err
		}
		return resolver, retentionPolicySourceArtifactOverride, "", nil
	}

	resolver, source, err := loadRetentionResolverFromDistributionEnv()
	if err == nil {
		return resolver, source, "", nil
	}

	return fallback, retentionPolicySourceDefaultFallback, classifyDistributionFallbackReason(err), nil
}

func loadRetentionResolverFromArtifactPath(path string) (replay.RetentionPolicyResolver, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, retentionSweepPolicyError{
			Code:   retentionSweepPolicyReadErrorCode,
			Detail: fmt.Sprintf("read retention policy artifact %s", path),
			Err:    err,
		}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, retentionSweepPolicyError{
			Code:   retentionSweepPolicyArtifactInvalidErrorCode,
			Detail: fmt.Sprintf("retention policy artifact %s must not be empty", path),
		}
	}

	var artifact retentionPolicyArtifact
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return nil, retentionSweepPolicyError{
			Code:   retentionSweepPolicyDecodeErrorCode,
			Detail: fmt.Sprintf("decode retention policy artifact %s", path),
			Err:    err,
		}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, retentionSweepPolicyError{
			Code:   retentionSweepPolicyDecodeErrorCode,
			Detail: fmt.Sprintf("decode retention policy artifact %s", path),
			Err:    fmt.Errorf("trailing data after top-level object"),
		}
	}

	return retentionResolverFromArtifact(path, artifact)
}

func loadRetentionResolverFromDistributionEnv() (replay.RetentionPolicyResolver, retentionPolicySource, error) {
	snapshot, source, err := distribution.LoadRetentionPolicySnapshotFromEnv()
	if err != nil {
		switch source {
		case distribution.RetentionSnapshotSourceHTTP:
			return nil, retentionPolicySourceCPDistributionHTTP, err
		case distribution.RetentionSnapshotSourceFile:
			return nil, retentionPolicySourceCPDistributionFile, err
		default:
			return nil, retentionPolicySourceCPDistributionFile, err
		}
	}

	artifact := retentionPolicyArtifact{
		TenantPolicies: map[string]retentionPolicyArtifactPolicy{},
	}
	if snapshot.DefaultPolicy != nil {
		artifact.DefaultPolicy = &retentionPolicyArtifactPolicy{
			TenantID:              snapshot.DefaultPolicy.TenantID,
			DefaultRetentionMS:    snapshot.DefaultPolicy.DefaultRetentionMS,
			PIIRetentionLimitMS:   snapshot.DefaultPolicy.PIIRetentionLimitMS,
			PHIRetentionLimitMS:   snapshot.DefaultPolicy.PHIRetentionLimitMS,
			MaxRetentionByClassMS: cloneRetentionPolicyWindows(snapshot.DefaultPolicy.MaxRetentionByClassMS),
		}
	}
	for tenantID, policy := range snapshot.TenantPolicies {
		artifact.TenantPolicies[tenantID] = retentionPolicyArtifactPolicy{
			TenantID:              policy.TenantID,
			DefaultRetentionMS:    policy.DefaultRetentionMS,
			PIIRetentionLimitMS:   policy.PIIRetentionLimitMS,
			PHIRetentionLimitMS:   policy.PHIRetentionLimitMS,
			MaxRetentionByClassMS: cloneRetentionPolicyWindows(policy.MaxRetentionByClassMS),
		}
	}
	if len(artifact.TenantPolicies) == 0 {
		artifact.TenantPolicies = nil
	}

	resolver, err := retentionResolverFromArtifact(string(source), artifact)
	if err != nil {
		switch source {
		case distribution.RetentionSnapshotSourceHTTP:
			return nil, retentionPolicySourceCPDistributionHTTP, err
		case distribution.RetentionSnapshotSourceFile:
			return nil, retentionPolicySourceCPDistributionFile, err
		default:
			return nil, retentionPolicySourceCPDistributionFile, err
		}
	}

	switch source {
	case distribution.RetentionSnapshotSourceHTTP:
		return resolver, retentionPolicySourceCPDistributionHTTP, nil
	case distribution.RetentionSnapshotSourceFile:
		return resolver, retentionPolicySourceCPDistributionFile, nil
	default:
		return resolver, retentionPolicySourceCPDistributionFile, nil
	}
}

func retentionResolverFromArtifact(source string, artifact retentionPolicyArtifact) (replay.RetentionPolicyResolver, error) {
	normalizedArtifact, err := normalizeRetentionPolicyArtifact(source, artifact)
	if err != nil {
		return nil, err
	}

	backend := replay.StaticRetentionPolicyResolver{
		TenantPolicies: normalizedArtifact.TenantPolicies,
	}
	if normalizedArtifact.DefaultPolicy != nil {
		backend.DefaultPolicy = *normalizedArtifact.DefaultPolicy
	}
	return replay.BackendRetentionPolicyResolver{
		Backend:  backend,
		Fallback: replay.StaticRetentionPolicyResolver{},
	}, nil
}

func cloneRetentionPolicyWindows(in map[eventabi.PayloadClass]int64) map[eventabi.PayloadClass]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[eventabi.PayloadClass]int64, len(in))
	for class, window := range in {
		out[class] = window
	}
	return out
}

func classifyDistributionFallbackReason(err error) string {
	var backendErr distribution.BackendError
	if errors.As(err, &backendErr) {
		switch backendErr.Code {
		case distribution.ErrorCodeInvalidConfig:
			return retentionPolicyFallbackReasonCPDistributionUnconfigured
		case distribution.ErrorCodeSnapshotStale:
			return retentionPolicyFallbackReasonCPDistributionStale
		case distribution.ErrorCodeDecodeArtifact, distribution.ErrorCodeInvalidArtifact, distribution.ErrorCodeSnapshotMissing:
			return retentionPolicyFallbackReasonCPDistributionInvalid
		case distribution.ErrorCodeReadArtifact:
			return retentionPolicyFallbackReasonCPDistributionFetchFailed
		default:
			return retentionPolicyFallbackReasonCPDistributionFetchFailed
		}
	}

	var policyErr retentionSweepPolicyError
	if errors.As(err, &policyErr) {
		return retentionPolicyFallbackReasonCPDistributionInvalid
	}

	return retentionPolicyFallbackReasonCPDistributionFetchFailed
}

func summarizePolicySource(runResults []retentionSweepRunResult) string {
	if len(runResults) == 0 {
		return string(retentionPolicySourceDefaultFallback)
	}
	source := runResults[0].PolicySource
	for _, result := range runResults[1:] {
		if result.PolicySource != source {
			return string(retentionPolicySourceMixed)
		}
	}
	return source
}

func summarizePolicyFallbackReason(runResults []retentionSweepRunResult) string {
	if len(runResults) == 0 {
		return ""
	}
	reason := runResults[0].PolicyFallbackReason
	for _, result := range runResults[1:] {
		if result.PolicyFallbackReason != reason {
			return retentionPolicyFallbackReasonMixed
		}
	}
	return reason
}

func normalizeRetentionPolicyArtifact(path string, artifact retentionPolicyArtifact) (normalizedRetentionPolicyArtifact, error) {
	if artifact.DefaultPolicy == nil && len(artifact.TenantPolicies) == 0 {
		return normalizedRetentionPolicyArtifact{}, retentionSweepPolicyError{
			Code:   retentionSweepPolicyArtifactInvalidErrorCode,
			Detail: fmt.Sprintf("retention policy artifact %s must include default_policy or tenant_policies", path),
		}
	}

	normalized := normalizedRetentionPolicyArtifact{
		TenantPolicies: map[string]replay.RetentionPolicy{},
	}

	if artifact.DefaultPolicy != nil {
		policy := artifact.DefaultPolicy.toRuntimePolicy()
		if policy.TenantID == "" {
			policy.TenantID = defaultRetentionPolicyTenantID
		}
		if err := policy.Validate(); err != nil {
			return normalizedRetentionPolicyArtifact{}, retentionSweepPolicyError{
				Code:   retentionSweepPolicyValidationErrorCode,
				Detail: fmt.Sprintf("default policy validation failed in %s", path),
				Err:    err,
			}
		}
		normalized.DefaultPolicy = &policy
	}

	tenantIDs := make([]string, 0, len(artifact.TenantPolicies))
	for tenantID := range artifact.TenantPolicies {
		tenantIDs = append(tenantIDs, tenantID)
	}
	sort.Strings(tenantIDs)

	for _, tenantID := range tenantIDs {
		trimmedTenantID := strings.TrimSpace(tenantID)
		if trimmedTenantID == "" {
			return normalizedRetentionPolicyArtifact{}, retentionSweepPolicyError{
				Code:   retentionSweepPolicyArtifactInvalidErrorCode,
				Detail: fmt.Sprintf("tenant policy key must be non-empty in %s", path),
			}
		}

		policy := artifact.TenantPolicies[tenantID].toRuntimePolicy()
		if policy.TenantID == "" {
			policy.TenantID = trimmedTenantID
		}
		if policy.TenantID != trimmedTenantID {
			return normalizedRetentionPolicyArtifact{}, retentionSweepPolicyError{
				Code:   retentionSweepPolicyTenantMismatchErrorCode,
				Detail: fmt.Sprintf("tenant policy key %s mismatches tenant_id %s in %s", trimmedTenantID, policy.TenantID, path),
			}
		}
		if err := policy.Validate(); err != nil {
			return normalizedRetentionPolicyArtifact{}, retentionSweepPolicyError{
				Code:   retentionSweepPolicyValidationErrorCode,
				Detail: fmt.Sprintf("tenant policy %s validation failed in %s", trimmedTenantID, path),
				Err:    err,
			}
		}
		normalized.TenantPolicies[trimmedTenantID] = policy
	}

	if len(normalized.TenantPolicies) == 0 {
		normalized.TenantPolicies = nil
	}

	return normalized, nil
}

func writeJSONArtifact(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create artifact directory for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write artifact %s: %w", path, err)
	}
	return nil
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "rspp-runtime usage:")
	_, _ = fmt.Fprintln(w, "  rspp-runtime [bootstrap-providers]")
	_, _ = fmt.Fprintln(w, "  rspp-runtime livekit [-report <path>] [-events <path>] [-dry-run <bool>] [-probe <bool>] [-url <livekit_url>] [-api-key <key>] [-api-secret <secret>] [-room <room>] [-session-id <id>] [-turn-id <id>] [-pipeline-version <version>] [-authority-epoch <n>] [-default-data-class <class>] [-request-timeout-ms <ms>]")
	_, _ = fmt.Fprintln(w, "  rspp-runtime retention-sweep -store <path> -tenants <tenant_a,tenant_b> [-policy <path>] [-report <path>] [-now-ms <ms>] [-interval-ms <ms>] [-runs <n>]")
}
