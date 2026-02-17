package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	DefaultContractsReportPath            = ".codex/ops/contracts-report.json"
	DefaultReplayRegressionReportPath     = ".codex/replay/regression-report.json"
	DefaultSLOGatesReportPath             = ".codex/ops/slo-gates-report.json"
	DefaultReleaseManifestPath            = ".codex/release/release-manifest.json"
	defaultClockSkewAllowance             = 5 * time.Minute
	ContractsReportSchemaVersionV1        = "rspp.tooling.contracts-report.v1"
	ReplayRegressionReportSchemaVersionV1 = "rspp.tooling.replay-regression-report.v1"
	SLOGatesReportSchemaVersionV1         = "rspp.tooling.slo-gates-report.v1"
	MVPSLOGatesReportSchemaVersionV1      = "rspp.tooling.slo-gates-mvp-report.v1"
	ReleaseManifestSchemaVersionV1        = "rspp.tooling.release-manifest.v1"
)

var DefaultMaxArtifactAge = 24 * time.Hour

var allowedRolloutStrategies = map[string]struct{}{
	"immediate": {},
	"phased":    {},
	"canary":    {},
}

var allowedRollbackModes = map[string]struct{}{
	"automatic": {},
	"manual":    {},
}

// RollbackPosture requires explicit rollback intent for release publishing.
type RollbackPosture struct {
	Mode    string `json:"mode"`
	Trigger string `json:"trigger"`
}

// RolloutConfig captures release rollout intent for DX-04.
type RolloutConfig struct {
	PipelineVersion string          `json:"pipeline_version"`
	Strategy        string          `json:"strategy"`
	RollbackPosture RollbackPosture `json:"rollback_posture"`
}

// ArtifactSource captures source artifact identity in a release manifest.
type ArtifactSource struct {
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	GeneratedAtUTC string `json:"generated_at_utc,omitempty"`
}

// GateStatus captures one readiness gate check result.
type GateStatus struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Passed         bool   `json:"passed"`
	Reason         string `json:"reason,omitempty"`
	GeneratedAtUTC string `json:"generated_at_utc,omitempty"`
	AgeMS          int64  `json:"age_ms,omitempty"`
}

// ReadinessResult captures release readiness gate status.
type ReadinessResult struct {
	Passed           bool         `json:"passed"`
	MaxArtifactAgeMS int64        `json:"max_artifact_age_ms"`
	Checks           []GateStatus `json:"checks"`
	Violations       []string     `json:"violations,omitempty"`
}

// ReadinessInput defines artifacts used by artifact-based release readiness checks.
type ReadinessInput struct {
	ContractsReportPath        string
	ReplayRegressionReportPath string
	SLOGatesReportPath         string
	MaxArtifactAge             time.Duration
	Now                        time.Time
}

// ReleaseManifest captures deterministic release publish output for deployment handoff.
type ReleaseManifest struct {
	SchemaVersion   string                    `json:"schema_version"`
	ReleaseID       string                    `json:"release_id"`
	GeneratedAtUTC  string                    `json:"generated_at_utc"`
	SpecRef         string                    `json:"spec_ref"`
	RolloutConfig   RolloutConfig             `json:"rollout_config"`
	Readiness       ReadinessResult           `json:"readiness"`
	SourceArtifacts map[string]ArtifactSource `json:"source_artifacts"`
}

type contractsReportArtifact struct {
	SchemaVersion  string `json:"schema_version"`
	GeneratedAtUTC string `json:"generated_at_utc"`
	Passed         bool   `json:"passed"`
}

type replayRegressionArtifact struct {
	SchemaVersion  string `json:"schema_version"`
	GeneratedAtUTC string `json:"generated_at_utc"`
	FailingCount   int    `json:"failing_count"`
}

type sloGatesReportArtifact struct {
	SchemaVersion  string `json:"schema_version"`
	GeneratedAtUTC string `json:"generated_at_utc"`
	Report         struct {
		Passed bool `json:"passed"`
	} `json:"report"`
}

// LoadRolloutConfig loads and validates DX-04 rollout configuration from JSON.
func LoadRolloutConfig(path string) (RolloutConfig, ArtifactSource, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return RolloutConfig{}, ArtifactSource{}, fmt.Errorf("rollout config path is required")
	}
	raw, err := os.ReadFile(trimmed)
	if err != nil {
		return RolloutConfig{}, ArtifactSource{}, fmt.Errorf("read rollout config %s: %w", trimmed, err)
	}
	cfg := RolloutConfig{}
	if err := strictUnmarshal(raw, &cfg); err != nil {
		return RolloutConfig{}, ArtifactSource{}, fmt.Errorf("decode rollout config %s: %w", trimmed, err)
	}
	if err := ValidateRolloutConfig(cfg); err != nil {
		return RolloutConfig{}, ArtifactSource{}, err
	}
	return cfg, ArtifactSource{
		Path:   trimmed,
		SHA256: sha256Hex(raw),
	}, nil
}

// ValidateRolloutConfig validates MVP rollout config requirements.
func ValidateRolloutConfig(cfg RolloutConfig) error {
	cfg.PipelineVersion = strings.TrimSpace(cfg.PipelineVersion)
	cfg.Strategy = strings.ToLower(strings.TrimSpace(cfg.Strategy))
	cfg.RollbackPosture.Mode = strings.ToLower(strings.TrimSpace(cfg.RollbackPosture.Mode))
	cfg.RollbackPosture.Trigger = strings.TrimSpace(cfg.RollbackPosture.Trigger)

	if cfg.PipelineVersion == "" {
		return fmt.Errorf("rollout config pipeline_version is required")
	}
	if _, ok := allowedRolloutStrategies[cfg.Strategy]; !ok {
		return fmt.Errorf("rollout config strategy must be one of immediate|phased|canary")
	}
	if _, ok := allowedRollbackModes[cfg.RollbackPosture.Mode]; !ok {
		return fmt.Errorf("rollout config rollback_posture.mode must be one of automatic|manual")
	}
	if cfg.RollbackPosture.Trigger == "" {
		return fmt.Errorf("rollout config rollback_posture.trigger is required")
	}
	return nil
}

// EvaluateReadiness evaluates release readiness from existing gate artifacts.
func EvaluateReadiness(in ReadinessInput) (ReadinessResult, map[string]ArtifactSource) {
	in = normalizeReadinessInput(in)
	result := ReadinessResult{
		Passed:           true,
		MaxArtifactAgeMS: in.MaxArtifactAge.Milliseconds(),
		Checks:           make([]GateStatus, 0, 3),
		Violations:       make([]string, 0),
	}
	sources := make(map[string]ArtifactSource, 3)

	contractsStatus, contractsSource := evaluateContractsCheck(in.ContractsReportPath, in.Now, in.MaxArtifactAge)
	result.Checks = append(result.Checks, contractsStatus)
	if contractsStatus.Passed {
		sources["contracts_report"] = contractsSource
	} else {
		result.Passed = false
		result.Violations = append(result.Violations, fmt.Sprintf("contracts_report: %s", contractsStatus.Reason))
	}

	replayStatus, replaySource := evaluateReplayRegressionCheck(in.ReplayRegressionReportPath, in.Now, in.MaxArtifactAge)
	result.Checks = append(result.Checks, replayStatus)
	if replayStatus.Passed {
		sources["replay_regression_report"] = replaySource
	} else {
		result.Passed = false
		result.Violations = append(result.Violations, fmt.Sprintf("replay_regression_report: %s", replayStatus.Reason))
	}

	sloStatus, sloSource := evaluateSLOGatesCheck(in.SLOGatesReportPath, in.Now, in.MaxArtifactAge)
	result.Checks = append(result.Checks, sloStatus)
	if sloStatus.Passed {
		sources["slo_gates_report"] = sloSource
	} else {
		result.Passed = false
		result.Violations = append(result.Violations, fmt.Sprintf("slo_gates_report: %s", sloStatus.Reason))
	}

	if len(result.Violations) == 0 {
		result.Violations = nil
	}
	return result, sources
}

// BuildReleaseManifest builds a deterministic release manifest from validated inputs.
func BuildReleaseManifest(
	specRef string,
	cfg RolloutConfig,
	readiness ReadinessResult,
	sources map[string]ArtifactSource,
	now time.Time,
) (ReleaseManifest, error) {
	trimmedSpecRef := strings.TrimSpace(specRef)
	if trimmedSpecRef == "" {
		return ReleaseManifest{}, fmt.Errorf("spec_ref is required")
	}
	if err := ValidateRolloutConfig(cfg); err != nil {
		return ReleaseManifest{}, err
	}
	if !readiness.Passed {
		return ReleaseManifest{}, fmt.Errorf("release readiness failed: %v", readiness.Violations)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	seed := strings.Join([]string{
		trimmedSpecRef,
		cfg.PipelineVersion,
		cfg.Strategy,
		cfg.RollbackPosture.Mode,
		cfg.RollbackPosture.Trigger,
	}, "|")
	releaseID := fmt.Sprintf("rel-%s-%s", now.Format("20060102150405"), shortHash(seed, 10))

	normalizedSources := make(map[string]ArtifactSource, len(sources))
	for key, source := range sources {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		source.Path = strings.TrimSpace(source.Path)
		source.SHA256 = strings.TrimSpace(source.SHA256)
		source.GeneratedAtUTC = strings.TrimSpace(source.GeneratedAtUTC)
		normalizedSources[k] = source
	}

	return ReleaseManifest{
		SchemaVersion:   ReleaseManifestSchemaVersionV1,
		ReleaseID:       releaseID,
		GeneratedAtUTC:  now.Format(time.RFC3339),
		SpecRef:         trimmedSpecRef,
		RolloutConfig:   cfg,
		Readiness:       readiness,
		SourceArtifacts: normalizedSources,
	}, nil
}

func normalizeReadinessInput(in ReadinessInput) ReadinessInput {
	if strings.TrimSpace(in.ContractsReportPath) == "" {
		in.ContractsReportPath = DefaultContractsReportPath
	}
	if strings.TrimSpace(in.ReplayRegressionReportPath) == "" {
		in.ReplayRegressionReportPath = DefaultReplayRegressionReportPath
	}
	if strings.TrimSpace(in.SLOGatesReportPath) == "" {
		in.SLOGatesReportPath = DefaultSLOGatesReportPath
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	} else {
		in.Now = in.Now.UTC()
	}
	if in.MaxArtifactAge <= 0 {
		in.MaxArtifactAge = DefaultMaxArtifactAge
	}
	return in
}

func evaluateContractsCheck(path string, now time.Time, maxAge time.Duration) (GateStatus, ArtifactSource) {
	status := GateStatus{Name: "contracts_report", Path: path, Passed: false}
	raw, source, err := readArtifact(path)
	if err != nil {
		status.Reason = err.Error()
		return status, ArtifactSource{}
	}

	artifact := contractsReportArtifact{}
	if err := strictUnmarshal(raw, &artifact); err != nil {
		status.Reason = fmt.Sprintf("decode contracts report: %v", err)
		return status, ArtifactSource{}
	}
	if err := requireSchemaVersion("contracts report", artifact.SchemaVersion, ContractsReportSchemaVersionV1); err != nil {
		status.Reason = err.Error()
		return status, ArtifactSource{}
	}
	status.GeneratedAtUTC = artifact.GeneratedAtUTC
	generatedAt, freshnessErr := validateFreshness(artifact.GeneratedAtUTC, now, maxAge)
	if freshnessErr != nil {
		status.Reason = freshnessErr.Error()
		return status, ArtifactSource{}
	}
	status.AgeMS = now.Sub(generatedAt).Milliseconds()
	if !artifact.Passed {
		status.Reason = "contracts report indicates failed fixtures"
		return status, ArtifactSource{}
	}

	status.Passed = true
	source.GeneratedAtUTC = artifact.GeneratedAtUTC
	return status, source
}

func evaluateReplayRegressionCheck(path string, now time.Time, maxAge time.Duration) (GateStatus, ArtifactSource) {
	status := GateStatus{Name: "replay_regression_report", Path: path, Passed: false}
	raw, source, err := readArtifact(path)
	if err != nil {
		status.Reason = err.Error()
		return status, ArtifactSource{}
	}

	artifact := replayRegressionArtifact{}
	if err := strictUnmarshal(raw, &artifact); err != nil {
		status.Reason = fmt.Sprintf("decode replay regression report: %v", err)
		return status, ArtifactSource{}
	}
	if err := requireSchemaVersion("replay regression report", artifact.SchemaVersion, ReplayRegressionReportSchemaVersionV1); err != nil {
		status.Reason = err.Error()
		return status, ArtifactSource{}
	}
	status.GeneratedAtUTC = artifact.GeneratedAtUTC
	generatedAt, freshnessErr := validateFreshness(artifact.GeneratedAtUTC, now, maxAge)
	if freshnessErr != nil {
		status.Reason = freshnessErr.Error()
		return status, ArtifactSource{}
	}
	status.AgeMS = now.Sub(generatedAt).Milliseconds()
	if artifact.FailingCount > 0 {
		status.Reason = fmt.Sprintf("replay regression report has failing_count=%d", artifact.FailingCount)
		return status, ArtifactSource{}
	}

	status.Passed = true
	source.GeneratedAtUTC = artifact.GeneratedAtUTC
	return status, source
}

func evaluateSLOGatesCheck(path string, now time.Time, maxAge time.Duration) (GateStatus, ArtifactSource) {
	status := GateStatus{Name: "slo_gates_report", Path: path, Passed: false}
	raw, source, err := readArtifact(path)
	if err != nil {
		status.Reason = err.Error()
		return status, ArtifactSource{}
	}

	artifact := sloGatesReportArtifact{}
	if err := strictUnmarshal(raw, &artifact); err != nil {
		status.Reason = fmt.Sprintf("decode slo gates report: %v", err)
		return status, ArtifactSource{}
	}
	if err := requireSchemaVersion("slo gates report", artifact.SchemaVersion, SLOGatesReportSchemaVersionV1); err != nil {
		status.Reason = err.Error()
		return status, ArtifactSource{}
	}
	status.GeneratedAtUTC = artifact.GeneratedAtUTC
	generatedAt, freshnessErr := validateFreshness(artifact.GeneratedAtUTC, now, maxAge)
	if freshnessErr != nil {
		status.Reason = freshnessErr.Error()
		return status, ArtifactSource{}
	}
	status.AgeMS = now.Sub(generatedAt).Milliseconds()
	if !artifact.Report.Passed {
		status.Reason = "slo gates report indicates failure"
		return status, ArtifactSource{}
	}

	status.Passed = true
	source.GeneratedAtUTC = artifact.GeneratedAtUTC
	return status, source
}

func readArtifact(path string) ([]byte, ArtifactSource, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, ArtifactSource{}, fmt.Errorf("artifact path is required")
	}
	raw, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, ArtifactSource{}, fmt.Errorf("read artifact %s: %w", trimmed, err)
	}
	return raw, ArtifactSource{Path: trimmed, SHA256: sha256Hex(raw)}, nil
}

func validateFreshness(generatedAtUTC string, now time.Time, maxAge time.Duration) (time.Time, error) {
	trimmed := strings.TrimSpace(generatedAtUTC)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("generated_at_utc is required")
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse generated_at_utc: %w", err)
	}
	if parsed.After(now.Add(defaultClockSkewAllowance)) {
		return time.Time{}, fmt.Errorf("generated_at_utc is in the future")
	}
	if now.Sub(parsed) > maxAge {
		return time.Time{}, fmt.Errorf("artifact is stale (older than %s)", maxAge)
	}
	return parsed, nil
}

func requireSchemaVersion(artifactName string, actual string, expected string) error {
	if strings.TrimSpace(actual) != expected {
		return fmt.Errorf("%s schema_version must equal %q", artifactName, expected)
	}
	return nil
}

func strictUnmarshal(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected trailing JSON payload")
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func shortHash(raw string, length int) string {
	if length < 1 {
		length = 8
	}
	sum := sha256.Sum256([]byte(raw))
	full := hex.EncodeToString(sum[:])
	if length > len(full) {
		length = len(full)
	}
	return full[:length]
}
