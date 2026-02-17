package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateVersionSkewPolicyPass(t *testing.T) {
	t.Parallel()

	eval := EvaluateVersionSkewPolicy(VersionSkewPolicy{
		SchemaVersion: VersionSkewPolicySchemaVersionV1,
		PolicyID:      "skew-v1",
		AllowedCells: []VersionSkewCell{
			{RuntimeVersion: "v1.0.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0"},
			{RuntimeVersion: "v1.1.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0"},
		},
		MatrixTests: []VersionSkewExpectation{
			{RuntimeVersion: "v1.0.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0", Allowed: true},
			{RuntimeVersion: "v1.1.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0", Allowed: true},
			{RuntimeVersion: "v2.0.0", ControlPlaneVersion: "v1.1.0", SchemaVersion: "v1.1.0", Allowed: false},
		},
	})
	if !eval.Passed {
		t.Fatalf("expected skew policy evaluation to pass, got %+v", eval)
	}
	if eval.AllowedTestCount != 2 || eval.DisallowedTestCount != 1 {
		t.Fatalf("unexpected skew matrix test counts: %+v", eval)
	}
}

func TestEvaluateVersionSkewPolicyFailsOnExpectedDisallowedMismatch(t *testing.T) {
	t.Parallel()

	eval := EvaluateVersionSkewPolicy(VersionSkewPolicy{
		SchemaVersion: VersionSkewPolicySchemaVersionV1,
		PolicyID:      "skew-v1",
		AllowedCells: []VersionSkewCell{
			{RuntimeVersion: "v1.0.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0"},
		},
		MatrixTests: []VersionSkewExpectation{
			{RuntimeVersion: "v1.0.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0", Allowed: false},
			{RuntimeVersion: "v2.0.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0", Allowed: false},
		},
	})
	if eval.Passed {
		t.Fatalf("expected skew policy evaluation to fail, got %+v", eval)
	}
}

func TestEvaluateDeprecationPolicyPass(t *testing.T) {
	t.Parallel()

	eval := EvaluateDeprecationPolicy(DeprecationPolicy{
		SchemaVersion: DeprecationPolicySchemaVersionV1,
		PolicyID:      "deprecation-v1",
		Rules: []DeprecationRule{
			{
				FieldPath:      "policy.recording_policy.legacy_mode",
				WarningFromUTC: "2026-01-01T00:00:00Z",
				EnforceFromUTC: "2026-03-01T00:00:00Z",
				RemoveFromUTC:  "2026-06-01T00:00:00Z",
				MigrationHint:  "Use recording_policy.recording_level.",
			},
		},
		UsageChecks: []DeprecationUsageCheck{
			{
				FieldPath:     "policy.recording_policy.legacy_mode",
				ObservedAtUTC: "2025-12-15T00:00:00Z",
				ExpectedState: DeprecationStateAllowed,
			},
			{
				FieldPath:     "policy.recording_policy.legacy_mode",
				ObservedAtUTC: "2026-02-01T00:00:00Z",
				ExpectedState: DeprecationStateWarning,
			},
			{
				FieldPath:     "policy.recording_policy.legacy_mode",
				ObservedAtUTC: "2026-04-01T00:00:00Z",
				ExpectedState: DeprecationStateEnforced,
			},
			{
				FieldPath:     "policy.recording_policy.legacy_mode",
				ObservedAtUTC: "2026-07-01T00:00:00Z",
				ExpectedState: DeprecationStateRemoved,
			},
		},
	})
	if !eval.Passed {
		t.Fatalf("expected deprecation policy evaluation to pass, got %+v", eval)
	}
}

func TestEvaluateDeprecationPolicyFailsWhenExpectedStateMismatches(t *testing.T) {
	t.Parallel()

	eval := EvaluateDeprecationPolicy(DeprecationPolicy{
		SchemaVersion: DeprecationPolicySchemaVersionV1,
		PolicyID:      "deprecation-v1",
		Rules: []DeprecationRule{
			{
				FieldPath:      "spec.nodes[].legacy_option",
				WarningFromUTC: "2026-01-01T00:00:00Z",
				EnforceFromUTC: "2026-03-01T00:00:00Z",
				RemoveFromUTC:  "2026-06-01T00:00:00Z",
				MigrationHint:  "Use spec.nodes[].new_option.",
			},
		},
		UsageChecks: []DeprecationUsageCheck{
			{
				FieldPath:     "spec.nodes[].legacy_option",
				ObservedAtUTC: "2026-04-01T00:00:00Z",
				ExpectedState: DeprecationStateWarning,
			},
		},
	})
	if eval.Passed {
		t.Fatalf("expected deprecation policy evaluation to fail, got %+v", eval)
	}
}

func TestEvaluateConformancePass(t *testing.T) {
	t.Parallel()

	eval := EvaluateConformance(
		ConformanceProfile{
			SchemaVersion:       ConformanceProfileSchemaVersionV1,
			ProfileID:           "mvp-v1",
			MandatoryCategories: []string{"transport_adapter", "provider_adapter", "external_node_boundary"},
		},
		ConformanceResults{
			SchemaVersion: ConformanceResultsSchemaVersionV1,
			ProfileID:     "mvp-v1",
			Categories: map[string]bool{
				"transport_adapter":      true,
				"provider_adapter":       true,
				"external_node_boundary": true,
			},
		},
	)
	if !eval.Passed {
		t.Fatalf("expected conformance evaluation to pass, got %+v", eval)
	}
}

func TestEvaluateConformanceFailsOnMissingMandatoryCategory(t *testing.T) {
	t.Parallel()

	eval := EvaluateConformance(
		ConformanceProfile{
			SchemaVersion:       ConformanceProfileSchemaVersionV1,
			ProfileID:           "mvp-v1",
			MandatoryCategories: []string{"transport_adapter", "provider_adapter", "external_node_boundary"},
		},
		ConformanceResults{
			SchemaVersion: ConformanceResultsSchemaVersionV1,
			ProfileID:     "mvp-v1",
			Categories: map[string]bool{
				"transport_adapter": true,
				"provider_adapter":  false,
			},
		},
	)
	if eval.Passed {
		t.Fatalf("expected conformance evaluation to fail, got %+v", eval)
	}
	if len(eval.Missing) == 0 || len(eval.Failed) == 0 {
		t.Fatalf("expected missing and failed category markers, got %+v", eval)
	}
}

func TestEvaluateFeatureStatusStewardshipPass(t *testing.T) {
	t.Parallel()

	eval := EvaluateFeatureStatusStewardship(
		[]FeatureStatusEntry{
			{ID: "NF-010", Passes: true, ReleasePhase: FeatureReleasePhaseMVP, Status: FeatureVerifiedStatus, SourceRefs: []string{"gate:make verify-full"}},
			{ID: "NF-032", Passes: true, ReleasePhase: FeatureReleasePhaseMVP, Status: FeatureVerifiedStatus, SourceRefs: []string{"gate:make verify-full"}},
			{ID: "F-162", Passes: false, ReleasePhase: FeatureReleasePhasePostMVP},
			{ID: "F-163", Passes: false, ReleasePhase: FeatureReleasePhasePostMVP},
			{ID: "F-164", Passes: false, ReleasePhase: FeatureReleasePhasePostMVP},
		},
		DefaultUBT05FeatureExpectations(),
	)
	if !eval.Passed {
		t.Fatalf("expected feature status stewardship to pass, got %+v", eval)
	}
}

func TestEvaluateGovernanceAggregatesViolations(t *testing.T) {
	t.Parallel()

	report := EvaluateGovernance(GovernanceInput{
		VersionSkewPolicy: VersionSkewPolicy{
			SchemaVersion: VersionSkewPolicySchemaVersionV1,
			PolicyID:      "skew",
			AllowedCells: []VersionSkewCell{
				{RuntimeVersion: "v1.0.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0"},
			},
			MatrixTests: []VersionSkewExpectation{
				{RuntimeVersion: "v1.0.0", ControlPlaneVersion: "v1.0.0", SchemaVersion: "v1.0.0", Allowed: false},
			},
		},
		DeprecationPolicy: DeprecationPolicy{
			SchemaVersion: DeprecationPolicySchemaVersionV1,
			PolicyID:      "deprecation",
			Rules: []DeprecationRule{
				{
					FieldPath:      "policy.legacy",
					WarningFromUTC: "2026-01-01T00:00:00Z",
					EnforceFromUTC: "2026-03-01T00:00:00Z",
					RemoveFromUTC:  "2026-06-01T00:00:00Z",
					MigrationHint:  "use policy.new",
				},
			},
			UsageChecks: []DeprecationUsageCheck{
				{
					FieldPath:     "policy.legacy",
					ObservedAtUTC: "2026-04-01T00:00:00Z",
					ExpectedState: DeprecationStateWarning,
				},
			},
		},
		ConformanceProfile: ConformanceProfile{
			SchemaVersion:       ConformanceProfileSchemaVersionV1,
			ProfileID:           "mvp-v1",
			MandatoryCategories: []string{"transport_adapter"},
		},
		ConformanceResults: ConformanceResults{
			SchemaVersion: ConformanceResultsSchemaVersionV1,
			ProfileID:     "mvp-v1",
			Categories:    map[string]bool{},
		},
		FeatureStatusEntries: []FeatureStatusEntry{
			{ID: "NF-010", Passes: false, ReleasePhase: FeatureReleasePhaseMVP},
		},
		FeatureExpectations: DefaultUBT05FeatureExpectations(),
	})
	if report.Passed {
		t.Fatalf("expected governance report to fail, got %+v", report)
	}
	if len(report.Violations) == 0 {
		t.Fatalf("expected aggregated governance violations, got %+v", report)
	}
}

func TestReadFeatureStatusEntries(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "features.json")
	raw, err := json.Marshal([]FeatureStatusEntry{
		{ID: "NF-010", Passes: true, ReleasePhase: FeatureReleasePhaseMVP, Status: FeatureVerifiedStatus},
	})
	if err != nil {
		t.Fatalf("marshal features: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write features fixture: %v", err)
	}

	entries, err := ReadFeatureStatusEntries(path)
	if err != nil {
		t.Fatalf("read feature status entries: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "NF-010" {
		t.Fatalf("unexpected feature status entries: %+v", entries)
	}
}
