package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRolloutConfigAndValidate(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "rollout.json")
	mustWriteJSON(t, cfgPath, map[string]any{
		"pipeline_version": "pipeline-v2",
		"strategy":         "canary",
		"rollback_posture": map[string]any{
			"mode":    "automatic",
			"trigger": "replay_or_slo_failure",
		},
	})

	cfg, source, err := LoadRolloutConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected load rollout config error: %v", err)
	}
	if cfg.PipelineVersion != "pipeline-v2" || cfg.Strategy != "canary" {
		t.Fatalf("unexpected rollout config: %+v", cfg)
	}
	if source.Path != cfgPath || source.SHA256 == "" {
		t.Fatalf("unexpected rollout source: %+v", source)
	}
}

func TestLoadRolloutConfigRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "rollout-invalid.json")
	mustWriteJSON(t, cfgPath, map[string]any{
		"pipeline_version": "pipeline-v2",
		"strategy":         "invalid",
		"rollback_posture": map[string]any{
			"mode":    "automatic",
			"trigger": "replay_or_slo_failure",
		},
	})
	if _, _, err := LoadRolloutConfig(cfgPath); err == nil {
		t.Fatalf("expected invalid rollout strategy error")
	}
}

func TestEvaluateReadinessPass(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 11, 4, 0, 0, 0, time.UTC)
	tmp := t.TempDir()

	contractsPath := filepath.Join(tmp, "contracts.json")
	replayPath := filepath.Join(tmp, "replay.json")
	sloPath := filepath.Join(tmp, "slo.json")

	mustWriteJSON(t, contractsPath, map[string]any{
		"schema_version":   ContractsReportSchemaVersionV1,
		"generated_at_utc": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"passed":           true,
	})
	mustWriteJSON(t, replayPath, map[string]any{
		"schema_version":   ReplayRegressionReportSchemaVersionV1,
		"generated_at_utc": now.Add(-30 * time.Minute).Format(time.RFC3339),
		"failing_count":    0,
	})
	mustWriteJSON(t, sloPath, map[string]any{
		"schema_version":   SLOGatesReportSchemaVersionV1,
		"generated_at_utc": now.Add(-10 * time.Minute).Format(time.RFC3339),
		"report": map[string]any{
			"passed": true,
		},
	})

	readiness, sources := EvaluateReadiness(ReadinessInput{
		ContractsReportPath:        contractsPath,
		ReplayRegressionReportPath: replayPath,
		SLOGatesReportPath:         sloPath,
		MaxArtifactAge:             24 * time.Hour,
		Now:                        now,
	})
	if !readiness.Passed {
		t.Fatalf("expected readiness pass, got %+v", readiness)
	}
	if len(readiness.Checks) != 3 || len(sources) != 3 {
		t.Fatalf("expected three checks/sources, got checks=%d sources=%d", len(readiness.Checks), len(sources))
	}
}

func TestEvaluateReadinessFailClosed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 11, 4, 0, 0, 0, time.UTC)
	tmp := t.TempDir()

	contractsPath := filepath.Join(tmp, "contracts.json")
	replayPath := filepath.Join(tmp, "replay.json")
	sloPath := filepath.Join(tmp, "slo.json")

	mustWriteJSON(t, contractsPath, map[string]any{
		"schema_version":   ContractsReportSchemaVersionV1,
		"generated_at_utc": now.Add(-48 * time.Hour).Format(time.RFC3339),
		"passed":           true,
	})
	mustWriteJSON(t, replayPath, map[string]any{
		"schema_version":   ReplayRegressionReportSchemaVersionV1,
		"generated_at_utc": now.Add(-10 * time.Minute).Format(time.RFC3339),
		"failing_count":    2,
	})
	mustWriteJSON(t, sloPath, map[string]any{
		"schema_version":   SLOGatesReportSchemaVersionV1,
		"generated_at_utc": now.Add(-10 * time.Minute).Format(time.RFC3339),
		"report": map[string]any{
			"passed": false,
		},
	})

	readiness, sources := EvaluateReadiness(ReadinessInput{
		ContractsReportPath:        contractsPath,
		ReplayRegressionReportPath: replayPath,
		SLOGatesReportPath:         sloPath,
		MaxArtifactAge:             24 * time.Hour,
		Now:                        now,
	})
	if readiness.Passed {
		t.Fatalf("expected readiness failure, got %+v", readiness)
	}
	if len(readiness.Violations) != 3 {
		t.Fatalf("expected three violations, got %+v", readiness.Violations)
	}
	if len(sources) != 0 {
		t.Fatalf("expected no accepted sources on fail-closed readiness, got %+v", sources)
	}
}

func TestEvaluateReadinessRejectsUnexpectedArtifactSchemaVersion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 11, 4, 0, 0, 0, time.UTC)
	tmp := t.TempDir()

	contractsPath := filepath.Join(tmp, "contracts.json")
	replayPath := filepath.Join(tmp, "replay.json")
	sloPath := filepath.Join(tmp, "slo.json")

	mustWriteJSON(t, contractsPath, map[string]any{
		"schema_version":   "rspp.tooling.contracts-report.v0",
		"generated_at_utc": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"passed":           true,
	})
	mustWriteJSON(t, replayPath, map[string]any{
		"schema_version":   ReplayRegressionReportSchemaVersionV1,
		"generated_at_utc": now.Add(-30 * time.Minute).Format(time.RFC3339),
		"failing_count":    0,
	})
	mustWriteJSON(t, sloPath, map[string]any{
		"schema_version":   SLOGatesReportSchemaVersionV1,
		"generated_at_utc": now.Add(-10 * time.Minute).Format(time.RFC3339),
		"report": map[string]any{
			"passed": true,
		},
	})

	readiness, _ := EvaluateReadiness(ReadinessInput{
		ContractsReportPath:        contractsPath,
		ReplayRegressionReportPath: replayPath,
		SLOGatesReportPath:         sloPath,
		MaxArtifactAge:             24 * time.Hour,
		Now:                        now,
	})
	if readiness.Passed {
		t.Fatalf("expected readiness failure on contracts schema version mismatch")
	}
	if len(readiness.Violations) == 0 {
		t.Fatalf("expected readiness violations on schema mismatch")
	}
}

func TestBuildReleaseManifest(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 11, 4, 5, 6, 0, time.UTC)
	cfg := RolloutConfig{
		PipelineVersion: "pipeline-v2",
		Strategy:        "phased",
		RollbackPosture: RollbackPosture{Mode: "manual", Trigger: "operator_gate_replay_regression"},
	}
	readiness := ReadinessResult{Passed: true, Checks: []GateStatus{{Name: "contracts_report", Passed: true}}}
	sources := map[string]ArtifactSource{
		"contracts_report": {Path: ".codex/ops/contracts-report.json", SHA256: "abc"},
	}

	manifest, err := BuildReleaseManifest("specs/pipeline-v2.json", cfg, readiness, sources, now)
	if err != nil {
		t.Fatalf("unexpected build manifest error: %v", err)
	}
	if !strings.HasPrefix(manifest.ReleaseID, "rel-20260211040506-") {
		t.Fatalf("unexpected release id: %s", manifest.ReleaseID)
	}
	if manifest.SpecRef != "specs/pipeline-v2.json" || manifest.RolloutConfig.PipelineVersion != "pipeline-v2" {
		t.Fatalf("unexpected manifest payload: %+v", manifest)
	}
	if manifest.SchemaVersion != ReleaseManifestSchemaVersionV1 {
		t.Fatalf("unexpected release manifest schema_version: %s", manifest.SchemaVersion)
	}

	readiness.Passed = false
	if _, err := BuildReleaseManifest("specs/pipeline-v2.json", cfg, readiness, sources, now); err == nil {
		t.Fatalf("expected readiness failure to reject manifest build")
	}
}

func mustWriteJSON(t *testing.T, path string, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
}
