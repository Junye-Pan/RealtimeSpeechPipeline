package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	VersionSkewPolicySchemaVersionV1                   = "rspp.version-skew-policy.v1"
	DeprecationPolicySchemaVersionV1                   = "rspp.deprecation-policy.v1"
	ConformanceProfileSchemaVersionV1                  = "rspp.conformance-profile.v1"
	ConformanceResultsSchemaVersionV1                  = "rspp.conformance-results.v1"
	GovernanceReportSchemaVersionV1                    = "rspp.tooling.conformance-governance-report.v1"
	FeatureReleasePhaseMVP                             = "mvp"
	FeatureReleasePhasePostMVP                         = "post_mvp"
	FeatureVerifiedStatus                              = "verified"
	DeprecationStateAllowed           DeprecationState = "allowed"
	DeprecationStateWarning           DeprecationState = "warning"
	DeprecationStateEnforced          DeprecationState = "enforced"
	DeprecationStateRemoved           DeprecationState = "removed"
)

// VersionSkewCell identifies one runtime/control-plane/schema combination.
type VersionSkewCell struct {
	RuntimeVersion      string `json:"runtime_version"`
	ControlPlaneVersion string `json:"control_plane_version"`
	SchemaVersion       string `json:"schema_version"`
}

// VersionSkewExpectation asserts whether one skew cell should be allowed.
type VersionSkewExpectation struct {
	RuntimeVersion      string `json:"runtime_version"`
	ControlPlaneVersion string `json:"control_plane_version"`
	SchemaVersion       string `json:"schema_version"`
	Allowed             bool   `json:"allowed"`
}

// VersionSkewPolicy defines supported and unsupported skew matrix cases.
type VersionSkewPolicy struct {
	SchemaVersion string                   `json:"schema_version"`
	PolicyID      string                   `json:"policy_id"`
	AllowedCells  []VersionSkewCell        `json:"allowed_cells"`
	MatrixTests   []VersionSkewExpectation `json:"matrix_tests"`
}

// VersionSkewEvaluation captures skew-matrix governance results.
type VersionSkewEvaluation struct {
	Passed              bool     `json:"passed"`
	AllowedTestCount    int      `json:"allowed_test_count"`
	DisallowedTestCount int      `json:"disallowed_test_count"`
	ViolationCount      int      `json:"violation_count"`
	Violations          []string `json:"violations,omitempty"`
}

// DeprecationState models lifecycle phases for deprecated fields.
type DeprecationState string

// DeprecationRule defines one field lifecycle schedule.
type DeprecationRule struct {
	FieldPath      string `json:"field_path"`
	WarningFromUTC string `json:"warning_from_utc"`
	EnforceFromUTC string `json:"enforce_from_utc"`
	RemoveFromUTC  string `json:"remove_from_utc"`
	MigrationHint  string `json:"migration_hint"`
}

// DeprecationUsageCheck asserts expected lifecycle state for one timestamped usage.
type DeprecationUsageCheck struct {
	FieldPath     string           `json:"field_path"`
	ObservedAtUTC string           `json:"observed_at_utc"`
	ExpectedState DeprecationState `json:"expected_state"`
}

// DeprecationPolicy defines rule timelines and concrete lifecycle checks.
type DeprecationPolicy struct {
	SchemaVersion string                  `json:"schema_version"`
	PolicyID      string                  `json:"policy_id"`
	Rules         []DeprecationRule       `json:"rules"`
	UsageChecks   []DeprecationUsageCheck `json:"usage_checks"`
}

// DeprecationEvaluation captures lifecycle validation outcomes.
type DeprecationEvaluation struct {
	Passed         bool     `json:"passed"`
	RuleCount      int      `json:"rule_count"`
	CheckCount     int      `json:"check_count"`
	ViolationCount int      `json:"violation_count"`
	Violations     []string `json:"violations,omitempty"`
}

// ConformanceProfile lists mandatory certification categories.
type ConformanceProfile struct {
	SchemaVersion       string   `json:"schema_version"`
	ProfileID           string   `json:"profile_id"`
	MandatoryCategories []string `json:"mandatory_categories"`
}

// ConformanceResults captures observed certification outcomes by category.
type ConformanceResults struct {
	SchemaVersion string          `json:"schema_version"`
	ProfileID     string          `json:"profile_id"`
	Categories    map[string]bool `json:"categories"`
}

// ConformanceEvaluation captures mandatory-category governance outcomes.
type ConformanceEvaluation struct {
	Passed         bool     `json:"passed"`
	MandatoryCount int      `json:"mandatory_count"`
	Missing        []string `json:"missing,omitempty"`
	Failed         []string `json:"failed,omitempty"`
	Violations     []string `json:"violations,omitempty"`
}

// FeatureStatusEntry is the subset used for stewardship checks.
type FeatureStatusEntry struct {
	ID           string   `json:"id"`
	Passes       bool     `json:"passes"`
	ReleasePhase string   `json:"release_phase"`
	Status       string   `json:"status,omitempty"`
	SourceRefs   []string `json:"source_refs,omitempty"`
}

// FeatureStatusExpectation defines expected stewardship values for one feature.
type FeatureStatusExpectation struct {
	ID                string
	ExpectedPasses    bool
	ExpectedPhase     string
	ExpectedStatus    string
	RequireStatus     bool
	RequireSourceRefs bool
	DisallowVerified  bool
}

// FeatureStatusEvaluation captures stewardship-check outcomes.
type FeatureStatusEvaluation struct {
	Passed         bool     `json:"passed"`
	CheckedCount   int      `json:"checked_count"`
	ViolationCount int      `json:"violation_count"`
	Violations     []string `json:"violations,omitempty"`
}

// GovernanceInput bundles all artifacts required for UB-T05 governance checks.
type GovernanceInput struct {
	VersionSkewPolicy    VersionSkewPolicy
	DeprecationPolicy    DeprecationPolicy
	ConformanceProfile   ConformanceProfile
	ConformanceResults   ConformanceResults
	FeatureStatusEntries []FeatureStatusEntry
	FeatureExpectations  []FeatureStatusExpectation
}

// GovernanceEvaluation is the aggregate UB-T05 governance report.
type GovernanceEvaluation struct {
	Passed        bool                    `json:"passed"`
	VersionSkew   VersionSkewEvaluation   `json:"version_skew"`
	Deprecation   DeprecationEvaluation   `json:"deprecation"`
	Conformance   ConformanceEvaluation   `json:"conformance"`
	FeatureStatus FeatureStatusEvaluation `json:"feature_status"`
	Violations    []string                `json:"violations,omitempty"`
}

// DefaultUBT05FeatureExpectations returns canonical stewardship checks for UB-T05.
func DefaultUBT05FeatureExpectations() []FeatureStatusExpectation {
	return []FeatureStatusExpectation{
		{
			ID:                "NF-010",
			ExpectedPasses:    true,
			ExpectedPhase:     FeatureReleasePhaseMVP,
			ExpectedStatus:    FeatureVerifiedStatus,
			RequireStatus:     true,
			RequireSourceRefs: true,
		},
		{
			ID:                "NF-032",
			ExpectedPasses:    true,
			ExpectedPhase:     FeatureReleasePhaseMVP,
			ExpectedStatus:    FeatureVerifiedStatus,
			RequireStatus:     true,
			RequireSourceRefs: true,
		},
		{
			ID:               "F-162",
			ExpectedPasses:   false,
			ExpectedPhase:    FeatureReleasePhasePostMVP,
			DisallowVerified: true,
		},
		{
			ID:               "F-163",
			ExpectedPasses:   false,
			ExpectedPhase:    FeatureReleasePhasePostMVP,
			DisallowVerified: true,
		},
		{
			ID:               "F-164",
			ExpectedPasses:   false,
			ExpectedPhase:    FeatureReleasePhasePostMVP,
			DisallowVerified: true,
		},
	}
}

// ReadVersionSkewPolicy loads a skew policy JSON artifact.
func ReadVersionSkewPolicy(path string) (VersionSkewPolicy, error) {
	raw, err := readFile(path)
	if err != nil {
		return VersionSkewPolicy{}, err
	}
	var out VersionSkewPolicy
	if err := decodeStrict(raw, &out); err != nil {
		return VersionSkewPolicy{}, fmt.Errorf("decode version skew policy %s: %w", path, err)
	}
	return out, nil
}

// ReadDeprecationPolicy loads a deprecation policy JSON artifact.
func ReadDeprecationPolicy(path string) (DeprecationPolicy, error) {
	raw, err := readFile(path)
	if err != nil {
		return DeprecationPolicy{}, err
	}
	var out DeprecationPolicy
	if err := decodeStrict(raw, &out); err != nil {
		return DeprecationPolicy{}, fmt.Errorf("decode deprecation policy %s: %w", path, err)
	}
	return out, nil
}

// ReadConformanceProfile loads a conformance profile JSON artifact.
func ReadConformanceProfile(path string) (ConformanceProfile, error) {
	raw, err := readFile(path)
	if err != nil {
		return ConformanceProfile{}, err
	}
	var out ConformanceProfile
	if err := decodeStrict(raw, &out); err != nil {
		return ConformanceProfile{}, fmt.Errorf("decode conformance profile %s: %w", path, err)
	}
	return out, nil
}

// ReadConformanceResults loads a conformance results JSON artifact.
func ReadConformanceResults(path string) (ConformanceResults, error) {
	raw, err := readFile(path)
	if err != nil {
		return ConformanceResults{}, err
	}
	var out ConformanceResults
	if err := decodeStrict(raw, &out); err != nil {
		return ConformanceResults{}, fmt.Errorf("decode conformance results %s: %w", path, err)
	}
	return out, nil
}

// ReadFeatureStatusEntries loads feature rows from docs/RSPP_features_framework.json.
func ReadFeatureStatusEntries(path string) ([]FeatureStatusEntry, error) {
	raw, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var out []FeatureStatusEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode feature status entries %s: %w", path, err)
	}
	return out, nil
}

// EvaluateGovernance executes UB-T05 governance checks.
func EvaluateGovernance(input GovernanceInput) GovernanceEvaluation {
	skew := EvaluateVersionSkewPolicy(input.VersionSkewPolicy)
	deprecation := EvaluateDeprecationPolicy(input.DeprecationPolicy)
	conformance := EvaluateConformance(input.ConformanceProfile, input.ConformanceResults)
	featureStatus := EvaluateFeatureStatusStewardship(input.FeatureStatusEntries, input.FeatureExpectations)

	violations := make([]string, 0)
	violations = appendPrefixedViolations(violations, "version_skew", skew.Violations)
	violations = appendPrefixedViolations(violations, "deprecation", deprecation.Violations)
	violations = appendPrefixedViolations(violations, "conformance", conformance.Violations)
	violations = appendPrefixedViolations(violations, "feature_status", featureStatus.Violations)

	out := GovernanceEvaluation{
		Passed:        len(violations) == 0,
		VersionSkew:   skew,
		Deprecation:   deprecation,
		Conformance:   conformance,
		FeatureStatus: featureStatus,
		Violations:    normalizeViolations(violations),
	}
	return out
}

// EvaluateVersionSkewPolicy validates supported/disallowed skew matrix tests.
func EvaluateVersionSkewPolicy(policy VersionSkewPolicy) VersionSkewEvaluation {
	violations := make([]string, 0)
	if strings.TrimSpace(policy.SchemaVersion) != VersionSkewPolicySchemaVersionV1 {
		violations = append(violations, fmt.Sprintf("schema_version must be %s", VersionSkewPolicySchemaVersionV1))
	}
	if strings.TrimSpace(policy.PolicyID) == "" {
		violations = append(violations, "policy_id is required")
	}
	if len(policy.AllowedCells) == 0 {
		violations = append(violations, "allowed_cells must be non-empty")
	}
	if len(policy.MatrixTests) == 0 {
		violations = append(violations, "matrix_tests must be non-empty")
	}

	allowed := make(map[string]struct{}, len(policy.AllowedCells))
	for idx, cell := range policy.AllowedCells {
		key, err := skewCellKey(cell.RuntimeVersion, cell.ControlPlaneVersion, cell.SchemaVersion)
		if err != nil {
			violations = append(violations, fmt.Sprintf("allowed_cells[%d]: %v", idx, err))
			continue
		}
		allowed[key] = struct{}{}
	}

	allowedTests := 0
	disallowedTests := 0
	for idx, test := range policy.MatrixTests {
		key, err := skewCellKey(test.RuntimeVersion, test.ControlPlaneVersion, test.SchemaVersion)
		if err != nil {
			violations = append(violations, fmt.Sprintf("matrix_tests[%d]: %v", idx, err))
			continue
		}
		_, present := allowed[key]
		if test.Allowed {
			allowedTests++
			if !present {
				violations = append(violations, fmt.Sprintf("matrix_tests[%d] expected allowed cell not present in allowed_cells", idx))
			}
			continue
		}
		disallowedTests++
		if present {
			violations = append(violations, fmt.Sprintf("matrix_tests[%d] expected disallowed cell but it is present in allowed_cells", idx))
		}
	}
	if allowedTests == 0 {
		violations = append(violations, "matrix_tests must include at least one allowed=true case")
	}
	if disallowedTests == 0 {
		violations = append(violations, "matrix_tests must include at least one allowed=false case")
	}

	return VersionSkewEvaluation{
		Passed:              len(violations) == 0,
		AllowedTestCount:    allowedTests,
		DisallowedTestCount: disallowedTests,
		ViolationCount:      len(violations),
		Violations:          normalizeViolations(violations),
	}
}

// EvaluateDeprecationPolicy validates lifecycle timelines and usage-check expectations.
func EvaluateDeprecationPolicy(policy DeprecationPolicy) DeprecationEvaluation {
	violations := make([]string, 0)
	if strings.TrimSpace(policy.SchemaVersion) != DeprecationPolicySchemaVersionV1 {
		violations = append(violations, fmt.Sprintf("schema_version must be %s", DeprecationPolicySchemaVersionV1))
	}
	if strings.TrimSpace(policy.PolicyID) == "" {
		violations = append(violations, "policy_id is required")
	}
	if len(policy.Rules) == 0 {
		violations = append(violations, "rules must be non-empty")
	}
	if len(policy.UsageChecks) == 0 {
		violations = append(violations, "usage_checks must be non-empty")
	}

	parsedRules := make(map[string]parsedDeprecationRule, len(policy.Rules))
	for idx, rule := range policy.Rules {
		fieldPath := strings.TrimSpace(rule.FieldPath)
		if fieldPath == "" {
			violations = append(violations, fmt.Sprintf("rules[%d].field_path is required", idx))
			continue
		}
		parsed, err := parseDeprecationRule(rule)
		if err != nil {
			violations = append(violations, fmt.Sprintf("rules[%d] %s: %v", idx, fieldPath, err))
			continue
		}
		parsedRules[fieldPath] = parsed
	}

	stateCoverage := map[DeprecationState]int{
		DeprecationStateAllowed:  0,
		DeprecationStateWarning:  0,
		DeprecationStateEnforced: 0,
		DeprecationStateRemoved:  0,
	}

	for idx, usage := range policy.UsageChecks {
		fieldPath := strings.TrimSpace(usage.FieldPath)
		if fieldPath == "" {
			violations = append(violations, fmt.Sprintf("usage_checks[%d].field_path is required", idx))
			continue
		}
		rule, ok := parsedRules[fieldPath]
		if !ok {
			violations = append(violations, fmt.Sprintf("usage_checks[%d] references unknown field_path %s", idx, fieldPath))
			continue
		}
		observedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(usage.ObservedAtUTC))
		if err != nil {
			violations = append(violations, fmt.Sprintf("usage_checks[%d] invalid observed_at_utc: %v", idx, err))
			continue
		}
		expected := normalizeDeprecationState(usage.ExpectedState)
		if !isDeprecationState(expected) {
			violations = append(violations, fmt.Sprintf("usage_checks[%d] expected_state must be one of allowed|warning|enforced|removed", idx))
			continue
		}
		actual := evaluateDeprecationState(rule, observedAt.UTC())
		stateCoverage[actual]++
		if expected != actual {
			violations = append(violations, fmt.Sprintf("usage_checks[%d] expected_state=%s got=%s for %s at %s", idx, expected, actual, fieldPath, observedAt.UTC().Format(time.RFC3339)))
		}
	}

	if stateCoverage[DeprecationStateWarning] == 0 {
		violations = append(violations, "usage_checks must cover warning state at least once")
	}
	if stateCoverage[DeprecationStateEnforced] == 0 {
		violations = append(violations, "usage_checks must cover enforced state at least once")
	}
	if stateCoverage[DeprecationStateRemoved] == 0 {
		violations = append(violations, "usage_checks must cover removed state at least once")
	}

	return DeprecationEvaluation{
		Passed:         len(violations) == 0,
		RuleCount:      len(policy.Rules),
		CheckCount:     len(policy.UsageChecks),
		ViolationCount: len(violations),
		Violations:     normalizeViolations(violations),
	}
}

// EvaluateConformance validates mandatory-category certification outcomes.
func EvaluateConformance(profile ConformanceProfile, results ConformanceResults) ConformanceEvaluation {
	violations := make([]string, 0)
	if strings.TrimSpace(profile.SchemaVersion) != ConformanceProfileSchemaVersionV1 {
		violations = append(violations, fmt.Sprintf("profile schema_version must be %s", ConformanceProfileSchemaVersionV1))
	}
	if strings.TrimSpace(results.SchemaVersion) != ConformanceResultsSchemaVersionV1 {
		violations = append(violations, fmt.Sprintf("results schema_version must be %s", ConformanceResultsSchemaVersionV1))
	}
	if strings.TrimSpace(profile.ProfileID) == "" {
		violations = append(violations, "profile_id is required")
	}
	if strings.TrimSpace(results.ProfileID) == "" {
		violations = append(violations, "results profile_id is required")
	}
	if strings.TrimSpace(profile.ProfileID) != "" && strings.TrimSpace(results.ProfileID) != "" && strings.TrimSpace(profile.ProfileID) != strings.TrimSpace(results.ProfileID) {
		violations = append(violations, "profile_id mismatch between profile and results")
	}

	mandatory := normalizeCategoryList(profile.MandatoryCategories)
	if len(mandatory) == 0 {
		violations = append(violations, "mandatory_categories must be non-empty")
	}
	missing := make([]string, 0)
	failed := make([]string, 0)
	for _, category := range mandatory {
		passed, ok := results.Categories[category]
		if !ok {
			missing = append(missing, category)
			continue
		}
		if !passed {
			failed = append(failed, category)
		}
	}
	if len(missing) > 0 {
		violations = append(violations, "missing mandatory category results: "+strings.Join(missing, ", "))
	}
	if len(failed) > 0 {
		violations = append(violations, "failed mandatory categories: "+strings.Join(failed, ", "))
	}

	return ConformanceEvaluation{
		Passed:         len(violations) == 0,
		MandatoryCount: len(mandatory),
		Missing:        normalizeStrings(missing),
		Failed:         normalizeStrings(failed),
		Violations:     normalizeViolations(violations),
	}
}

// EvaluateFeatureStatusStewardship validates required feature statuses in the framework doc.
func EvaluateFeatureStatusStewardship(entries []FeatureStatusEntry, expectations []FeatureStatusExpectation) FeatureStatusEvaluation {
	violations := make([]string, 0)
	index := make(map[string]FeatureStatusEntry, len(entries))
	for _, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		index[id] = entry
	}

	for _, expected := range expectations {
		id := strings.TrimSpace(expected.ID)
		if id == "" {
			violations = append(violations, "feature expectation id is required")
			continue
		}
		entry, ok := index[id]
		if !ok {
			violations = append(violations, fmt.Sprintf("feature %s missing from feature-status framework", id))
			continue
		}
		if entry.Passes != expected.ExpectedPasses {
			violations = append(violations, fmt.Sprintf("feature %s passes mismatch: expected %t got %t", id, expected.ExpectedPasses, entry.Passes))
		}
		if expected.ExpectedPhase != "" && strings.TrimSpace(entry.ReleasePhase) != expected.ExpectedPhase {
			violations = append(violations, fmt.Sprintf("feature %s release_phase mismatch: expected %s got %s", id, expected.ExpectedPhase, strings.TrimSpace(entry.ReleasePhase)))
		}
		if expected.RequireStatus {
			if strings.TrimSpace(entry.Status) != expected.ExpectedStatus {
				violations = append(violations, fmt.Sprintf("feature %s status mismatch: expected %s got %s", id, expected.ExpectedStatus, strings.TrimSpace(entry.Status)))
			}
		}
		if expected.RequireSourceRefs && len(entry.SourceRefs) == 0 {
			violations = append(violations, fmt.Sprintf("feature %s must include source_refs when verified", id))
		}
		if expected.DisallowVerified && strings.EqualFold(strings.TrimSpace(entry.Status), FeatureVerifiedStatus) {
			violations = append(violations, fmt.Sprintf("feature %s must not be marked verified", id))
		}
	}

	return FeatureStatusEvaluation{
		Passed:         len(violations) == 0,
		CheckedCount:   len(expectations),
		ViolationCount: len(violations),
		Violations:     normalizeViolations(violations),
	}
}

type parsedDeprecationRule struct {
	WarningFrom time.Time
	EnforceFrom time.Time
	RemoveFrom  time.Time
}

func parseDeprecationRule(rule DeprecationRule) (parsedDeprecationRule, error) {
	warningFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(rule.WarningFromUTC))
	if err != nil {
		return parsedDeprecationRule{}, fmt.Errorf("warning_from_utc: %w", err)
	}
	enforceFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(rule.EnforceFromUTC))
	if err != nil {
		return parsedDeprecationRule{}, fmt.Errorf("enforce_from_utc: %w", err)
	}
	removeFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(rule.RemoveFromUTC))
	if err != nil {
		return parsedDeprecationRule{}, fmt.Errorf("remove_from_utc: %w", err)
	}
	if !warningFrom.Before(enforceFrom) {
		return parsedDeprecationRule{}, fmt.Errorf("warning_from_utc must be before enforce_from_utc")
	}
	if !enforceFrom.Before(removeFrom) {
		return parsedDeprecationRule{}, fmt.Errorf("enforce_from_utc must be before remove_from_utc")
	}
	return parsedDeprecationRule{
		WarningFrom: warningFrom.UTC(),
		EnforceFrom: enforceFrom.UTC(),
		RemoveFrom:  removeFrom.UTC(),
	}, nil
}

func evaluateDeprecationState(rule parsedDeprecationRule, observedAt time.Time) DeprecationState {
	switch {
	case observedAt.Before(rule.WarningFrom):
		return DeprecationStateAllowed
	case observedAt.Before(rule.EnforceFrom):
		return DeprecationStateWarning
	case observedAt.Before(rule.RemoveFrom):
		return DeprecationStateEnforced
	default:
		return DeprecationStateRemoved
	}
}

func skewCellKey(runtimeVersion string, controlPlaneVersion string, schemaVersion string) (string, error) {
	runtimeVersion = strings.TrimSpace(runtimeVersion)
	controlPlaneVersion = strings.TrimSpace(controlPlaneVersion)
	schemaVersion = strings.TrimSpace(schemaVersion)
	if runtimeVersion == "" || controlPlaneVersion == "" || schemaVersion == "" {
		return "", fmt.Errorf("runtime_version, control_plane_version, and schema_version are required")
	}
	return runtimeVersion + "|" + controlPlaneVersion + "|" + schemaVersion, nil
}

func normalizeDeprecationState(state DeprecationState) DeprecationState {
	return DeprecationState(strings.ToLower(strings.TrimSpace(string(state))))
}

func isDeprecationState(state DeprecationState) bool {
	switch state {
	case DeprecationStateAllowed, DeprecationStateWarning, DeprecationStateEnforced, DeprecationStateRemoved:
		return true
	default:
		return false
	}
}

func normalizeCategoryList(categories []string) []string {
	if len(categories) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(categories))
	out := make([]string, 0, len(categories))
	for _, raw := range categories {
		cat := strings.TrimSpace(raw)
		if cat == "" {
			continue
		}
		if _, ok := seen[cat]; ok {
			continue
		}
		seen[cat] = struct{}{}
		out = append(out, cat)
	}
	return out
}

func appendPrefixedViolations(dst []string, prefix string, items []string) []string {
	for _, item := range items {
		dst = append(dst, fmt.Sprintf("%s: %s", prefix, item))
	}
	return dst
}

func normalizeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeViolations(violations []string) []string {
	normalized := normalizeStrings(violations)
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func decodeStrict(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func readFile(path string) ([]byte, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("path is required")
	}
	raw, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", trimmed, err)
	}
	return raw, nil
}
