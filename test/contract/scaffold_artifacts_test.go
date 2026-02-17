package contract_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPipelineAndDeployScaffoldArtifactsExistAndLink(t *testing.T) {
	t.Parallel()

	required := []string{
		"pipelines/specs/voice_assistant_spec_v1.json",
		"pipelines/profiles/simple_profile_v1.json",
		"pipelines/bundles/voice_assistant_bundle_v1.json",
		"pipelines/rollouts/voice_assistant_rollout_v1.json",
		"pipelines/policies/voice_assistant_policy_v1.json",
		"pipelines/compat/voice_assistant_compat_v1.json",
		"pipelines/extensions/voice_assistant_extensions_v1.json",
		"pipelines/replay/voice_assistant_replay_manifest_v1.json",
		"deploy/profiles/mvp-single-region/rollout.json",
		"deploy/verification/gates/mvp_gate_runner.sh",
		"scripts/prepare-mvp-live-artifacts.sh",
	}
	for _, relPath := range required {
		if _, err := os.Stat(repoPath(t, relPath)); err != nil {
			t.Fatalf("expected scaffold artifact %s: %v", relPath, err)
		}
	}

	bundle := readJSONMap(t, "pipelines/bundles/voice_assistant_bundle_v1.json")
	for _, key := range []string{
		"spec_ref",
		"profile_ref",
		"rollout_ref",
		"policy_ref",
		"compat_ref",
		"extensions_ref",
		"replay_manifest_ref",
	} {
		ref, ok := bundle[key].(string)
		if !ok || ref == "" {
			t.Fatalf("bundle key %s must be non-empty string", key)
		}
		if _, err := os.Stat(repoPath(t, ref)); err != nil {
			t.Fatalf("bundle ref %s (%s) missing: %v", key, ref, err)
		}
	}

	deployRollout := readJSONMap(t, "deploy/profiles/mvp-single-region/rollout.json")
	bundleRef, ok := deployRollout["bundle_ref"].(string)
	if !ok || bundleRef == "" {
		t.Fatalf("deploy rollout bundle_ref must be non-empty string")
	}
	if _, err := os.Stat(repoPath(t, bundleRef)); err != nil {
		t.Fatalf("deploy rollout bundle_ref missing: %v", err)
	}
}

func TestMVPGateFixturesAreCompareCompatible(t *testing.T) {
	t.Parallel()

	streaming := readJSONMap(t, "test/integration/fixtures/live-provider-chain-report.streaming.json")
	nonStreaming := readJSONMap(t, "test/integration/fixtures/live-provider-chain-report.nonstreaming.json")

	streamingMode := streaming["execution_mode"]
	nonStreamingMode := nonStreaming["execution_mode"]
	if streamingMode != "streaming" {
		t.Fatalf("expected streaming fixture execution_mode=streaming, got %v", streamingMode)
	}
	if nonStreamingMode != "non_streaming" {
		t.Fatalf("expected non-streaming fixture execution_mode=non_streaming, got %v", nonStreamingMode)
	}

	streamingIdentity, ok := streaming["comparison_identity"].(string)
	if !ok || streamingIdentity == "" {
		t.Fatalf("streaming comparison_identity must be non-empty")
	}
	nonStreamingIdentity, ok := nonStreaming["comparison_identity"].(string)
	if !ok || nonStreamingIdentity == "" {
		t.Fatalf("non-streaming comparison_identity must be non-empty")
	}
	if streamingIdentity != nonStreamingIdentity {
		t.Fatalf("fixture comparison_identity mismatch: streaming=%s non-streaming=%s", streamingIdentity, nonStreamingIdentity)
	}
}

func TestScaffoldArtifactsHaveExpectedSchemaAndCrossLinks(t *testing.T) {
	t.Parallel()

	spec := readJSONMap(t, "pipelines/specs/voice_assistant_spec_v1.json")
	profile := readJSONMap(t, "pipelines/profiles/simple_profile_v1.json")
	bundle := readJSONMap(t, "pipelines/bundles/voice_assistant_bundle_v1.json")
	rollout := readJSONMap(t, "pipelines/rollouts/voice_assistant_rollout_v1.json")
	policy := readJSONMap(t, "pipelines/policies/voice_assistant_policy_v1.json")
	compat := readJSONMap(t, "pipelines/compat/voice_assistant_compat_v1.json")
	extensions := readJSONMap(t, "pipelines/extensions/voice_assistant_extensions_v1.json")
	replayManifest := readJSONMap(t, "pipelines/replay/voice_assistant_replay_manifest_v1.json")
	deployRollout := readJSONMap(t, "deploy/profiles/mvp-single-region/rollout.json")

	expectStringField(t, spec, "schema_version", "rspp.pipeline.spec.v1")
	expectStringField(t, profile, "schema_version", "rspp.execution.profile.v1")
	expectStringField(t, bundle, "schema_version", "rspp.pipeline.bundle.v1")
	expectStringField(t, rollout, "schema_version", "rspp.rollout.v1")
	expectStringField(t, policy, "schema_version", "rspp.policy.bundle.v1")
	expectStringField(t, compat, "schema_version", "rspp.compatibility.v1")
	expectStringField(t, extensions, "schema_version", "rspp.extensions.v1")
	expectStringField(t, replayManifest, "schema_version", "rspp.replay.manifest.v1")
	expectStringField(t, deployRollout, "schema_version", "rspp.deploy.rollout.v1")

	specPipelineVersion := requiredStringField(t, spec, "pipeline_version")
	if specPipelineVersion == "" {
		t.Fatalf("spec pipeline_version must be non-empty")
	}
	if requiredStringField(t, bundle, "pipeline_version") != specPipelineVersion {
		t.Fatalf("bundle pipeline_version must match spec pipeline_version")
	}
	if requiredStringField(t, rollout, "default_pipeline_version") != specPipelineVersion {
		t.Fatalf("rollout default_pipeline_version must match spec pipeline_version")
	}
	if requiredStringField(t, compat, "pipeline_version") != specPipelineVersion {
		t.Fatalf("compat pipeline_version must match spec pipeline_version")
	}
	if requiredStringField(t, replayManifest, "pipeline_version") != specPipelineVersion {
		t.Fatalf("replay manifest pipeline_version must match spec pipeline_version")
	}
	if requiredStringField(t, deployRollout, "bundle_ref") != "pipelines/bundles/voice_assistant_bundle_v1.json" {
		t.Fatalf("deploy rollout bundle_ref must target canonical bundle path")
	}

	if requiredStringField(t, profile, "execution_profile") != "simple" {
		t.Fatalf("simple profile execution_profile must be simple")
	}

	providerBindings, ok := policy["provider_bindings"].(map[string]any)
	if !ok {
		t.Fatalf("policy provider_bindings must be object")
	}
	for _, key := range []string{"stt", "llm", "tts"} {
		value, ok := providerBindings[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("policy provider_bindings.%s must be non-empty string", key)
		}
	}

	transportMatrix, ok := compat["transport_matrix"].([]any)
	if !ok || len(transportMatrix) == 0 {
		t.Fatalf("compat transport_matrix must be non-empty array")
	}
	livekitSupported := false
	for _, raw := range transportMatrix {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("compat transport_matrix entry must be object")
		}
		kind, _ := entry["transport_kind"].(string)
		supported, _ := entry["supported"].(bool)
		if kind == "livekit" && supported {
			livekitSupported = true
			break
		}
	}
	if !livekitSupported {
		t.Fatalf("compat transport_matrix must include livekit supported=true")
	}
}

func TestDeployScaffoldTemplatesContainExpectedBaselineMarkers(t *testing.T) {
	t.Parallel()

	checkContains := func(relPath string, patterns ...string) {
		t.Helper()
		raw, err := os.ReadFile(repoPath(t, relPath))
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		content := string(raw)
		for _, pattern := range patterns {
			if !strings.Contains(content, pattern) {
				t.Fatalf("expected %s to contain %q", relPath, pattern)
			}
		}
	}

	checkContains(
		"deploy/k8s/mvp-single-region/runtime-deployment.yaml",
		"kind: Deployment",
		"name: rspp-runtime",
		"RSPP_PROFILE_ID",
	)
	checkContains(
		"deploy/k8s/mvp-single-region/control-plane-deployment.yaml",
		"kind: Deployment",
		"name: rspp-control-plane",
	)
	checkContains(
		"deploy/helm/rspp-mvp/Chart.yaml",
		"name: rspp-mvp",
		"type: application",
	)
	checkContains(
		"deploy/terraform/mvp_single_region/main.tf",
		"required_version",
		"null_resource",
	)
}

func TestMVPGateRunnerScriptHasDeterministicExecutionOrder(t *testing.T) {
	t.Parallel()

	path := repoPath(t, "deploy/verification/gates/mvp_gate_runner.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read gate runner script: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, "set -euo pipefail") {
		t.Fatalf("gate runner must enable strict shell settings")
	}
	if !strings.Contains(content, "mvp_gate_runner: PASS") {
		t.Fatalf("gate runner must emit deterministic PASS marker")
	}

	sequence := []string{
		"bash scripts/prepare-mvp-live-artifacts.sh",
		"make verify-quick",
		"make verify-full",
		"make verify-mvp",
		"make security-baseline-check",
	}
	last := -1
	for _, marker := range sequence {
		idx := strings.Index(content, marker)
		if idx < 0 {
			t.Fatalf("gate runner missing required command: %s", marker)
		}
		if idx <= last {
			t.Fatalf("gate runner command order is not deterministic around %s", marker)
		}
		last = idx
	}
}

func TestGateScriptsAreExecutable(t *testing.T) {
	t.Parallel()

	for _, relPath := range []string{
		"deploy/verification/gates/mvp_gate_runner.sh",
		"scripts/prepare-mvp-live-artifacts.sh",
	} {
		info, err := os.Stat(repoPath(t, relPath))
		if err != nil {
			t.Fatalf("expected script %s: %v", relPath, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("script %s must be executable", relPath)
		}
	}
}

func requiredStringField(t *testing.T, in map[string]any, key string) string {
	t.Helper()
	value, ok := in[key].(string)
	if !ok {
		t.Fatalf("expected %s to be string", key)
	}
	return strings.TrimSpace(value)
}

func expectStringField(t *testing.T, in map[string]any, key string, expected string) {
	t.Helper()
	if value := requiredStringField(t, in, key); value != expected {
		t.Fatalf("expected %s=%q, got %q", key, expected, value)
	}
}

func readJSONMap(t *testing.T, relPath string) map[string]any {
	t.Helper()
	path := repoPath(t, relPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

func repoPath(t *testing.T, relPath string) string {
	t.Helper()
	root := repoRoot(t)
	return filepath.Join(root, filepath.FromSlash(relPath))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("failed to find repo root from %s: %v", wd, fmt.Errorf("go.mod not found"))
		}
		dir = parent
	}
}
