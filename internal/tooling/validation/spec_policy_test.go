package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseValidationMode(t *testing.T) {
	t.Parallel()

	mode, err := ParseValidationMode("")
	if err != nil {
		t.Fatalf("unexpected default mode error: %v", err)
	}
	if mode != ValidationModeStrict {
		t.Fatalf("expected default strict mode, got %q", mode)
	}

	mode, err = ParseValidationMode("relaxed")
	if err != nil {
		t.Fatalf("unexpected relaxed mode error: %v", err)
	}
	if mode != ValidationModeRelaxed {
		t.Fatalf("expected relaxed mode, got %q", mode)
	}

	if _, err := ParseValidationMode("unsupported"); err == nil {
		t.Fatalf("expected unsupported mode to fail")
	}
}

func TestValidatePipelineSpecFileStrict(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "pipelines", "specs", "voice_assistant_spec_v1.json")
	if err := ValidatePipelineSpecFile(path, "strict"); err != nil {
		t.Fatalf("expected scaffold spec to validate in strict mode, got %v", err)
	}
}

func TestValidatePipelineSpecUnknownFieldByMode(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "schema_version": "rspp.pipeline.spec.v1",
  "spec_id": "spec-v1",
  "pipeline_version": "pipeline-v1",
  "graph_definition_ref": "graph/default",
  "nodes": [{"node_id":"stt","kind":"provider","modality":"stt"}],
  "edges": [{"from":"stt","to":"stt"}],
  "extra_field": true
}`)
	if err := ValidatePipelineSpec(raw, "strict"); err == nil {
		t.Fatalf("expected strict mode to reject unknown field")
	}
	if err := ValidatePipelineSpec(raw, "relaxed"); err != nil {
		t.Fatalf("expected relaxed mode to allow unknown field, got %v", err)
	}
}

func TestValidatePipelineSpecReportsActionableEdgeError(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "schema_version": "rspp.pipeline.spec.v1",
  "spec_id": "spec-v1",
  "pipeline_version": "pipeline-v1",
  "graph_definition_ref": "graph/default",
  "nodes": [{"node_id":"stt","kind":"provider","modality":"stt"}],
  "edges": [{"from":"missing","to":"stt"}]
}`)
	err := ValidatePipelineSpec(raw, "strict")
	if err == nil {
		t.Fatalf("expected unresolved edge endpoint to fail")
	}
	if !strings.Contains(err.Error(), "edges[0].from") {
		t.Fatalf("expected edge field path in error, got %v", err)
	}
}

func TestValidatePolicyBundleFileStrict(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "pipelines", "policies", "voice_assistant_policy_v1.json")
	if err := ValidatePolicyBundleFile(path, "strict"); err != nil {
		t.Fatalf("expected scaffold policy to validate in strict mode, got %v", err)
	}
}

func TestValidatePolicyBundleUnknownFieldByMode(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "schema_version": "rspp.policy.bundle.v1",
  "policy_resolution_snapshot": "policy-resolution/v1",
  "allowed_adaptive_actions": ["retry"],
  "provider_bindings": {"stt":"stt-a"},
  "recording_policy": {"recording_level":"L0","allowed_replay_modes":["audit"]},
  "extra_field": "ok"
}`)
	if err := ValidatePolicyBundle(raw, "strict"); err == nil {
		t.Fatalf("expected strict mode to reject unknown field")
	}
	if err := ValidatePolicyBundle(raw, "relaxed"); err != nil {
		t.Fatalf("expected relaxed mode to allow unknown field, got %v", err)
	}
}

func TestValidatePolicyBundleReportsActionableActionError(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "schema_version": "rspp.policy.bundle.v1",
  "policy_resolution_snapshot": "policy-resolution/v1",
  "allowed_adaptive_actions": ["unsupported"],
  "provider_bindings": {"stt":"stt-a"},
  "recording_policy": {"recording_level":"L0","allowed_replay_modes":["audit"]}
}`)
	err := ValidatePolicyBundle(raw, "strict")
	if err == nil {
		t.Fatalf("expected unsupported adaptive action to fail")
	}
	if !strings.Contains(err.Error(), "allowed_adaptive_actions[0]") {
		t.Fatalf("expected allowed_adaptive_actions field path in error, got %v", err)
	}
}

func TestValidatePipelineSpecFileRequiresPath(t *testing.T) {
	t.Parallel()

	if err := ValidatePipelineSpecFile("", "strict"); err == nil {
		t.Fatalf("expected empty spec path to fail")
	}
}

func TestValidatePolicyBundleFileRequiresPath(t *testing.T) {
	t.Parallel()

	if err := ValidatePolicyBundleFile("", "strict"); err == nil {
		t.Fatalf("expected empty policy path to fail")
	}
}

func TestValidatePipelineSpecFileReadFailure(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.json")
	if err := ValidatePipelineSpecFile(path, "strict"); err == nil {
		t.Fatalf("expected missing spec file to fail")
	}
}

func TestValidatePolicyBundleFileReadFailure(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.json")
	if err := ValidatePolicyBundleFile(path, "strict"); err == nil {
		t.Fatalf("expected missing policy file to fail")
	}
}

func TestValidatePipelineAndPolicyFileWithTempFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.json")
	policyPath := filepath.Join(tmp, "policy.json")

	spec := `{
  "schema_version": "rspp.pipeline.spec.v1",
  "spec_id": "spec-v1",
  "pipeline_version": "pipeline-v1",
  "graph_definition_ref": "graph/default",
  "nodes": [
    {"node_id":"stt","kind":"provider","modality":"stt"},
    {"node_id":"llm","kind":"provider","modality":"llm"}
  ],
  "edges": [{"from":"stt","to":"llm"}]
}`
	policy := `{
  "schema_version": "rspp.policy.bundle.v1",
  "policy_resolution_snapshot": "policy-resolution/v1",
  "allowed_adaptive_actions": ["retry", "provider_switch", "fallback"],
  "provider_bindings": {"stt":"stt-a", "llm":"llm-a", "tts":"tts-a"},
  "recording_policy": {"recording_level":"L0", "allowed_replay_modes":["audit"]}
}`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec fixture: %v", err)
	}
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy fixture: %v", err)
	}

	if err := ValidatePipelineSpecFile(specPath, "strict"); err != nil {
		t.Fatalf("expected strict spec validation success, got %v", err)
	}
	if err := ValidatePolicyBundleFile(policyPath, "strict"); err != nil {
		t.Fatalf("expected strict policy validation success, got %v", err)
	}
}
