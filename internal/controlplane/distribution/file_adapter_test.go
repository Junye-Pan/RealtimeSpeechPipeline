package distribution

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

func TestFileAdapterConfigFromEnvRequiresPath(t *testing.T) {
	t.Setenv(EnvFileAdapterPath, "")
	_, err := FileAdapterConfigFromEnv()
	if err == nil {
		t.Fatalf("expected missing env path error")
	}

	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeInvalidConfig {
		t.Fatalf("expected invalid_config error code, got %s", backendErr.Code)
	}
}

func TestNewFileBackendsResolvesAllServices(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "default_pipeline_version": "pipeline-file",
    "records": {
      "pipeline-file": {
        "pipeline_version": "pipeline-file",
        "graph_definition_ref": "graph/file",
        "execution_profile": "simple"
      }
    }
  },
  "rollout": {
    "version_resolution_snapshot": "version-resolution/file",
    "default_pipeline_version": "pipeline-file",
    "by_requested_version": {
      "pipeline-requested": "pipeline-file"
    }
  },
  "routing_view": {
    "default": {
      "routing_view_snapshot": "routing-view/file",
      "admission_policy_snapshot": "admission-policy/file",
      "abi_compatibility_snapshot": "abi-compat/file"
    }
  },
  "policy": {
    "default": {
      "policy_resolution_snapshot": "policy-resolution/file",
      "allowed_adaptive_actions": ["fallback", "retry"]
    }
  },
  "provider_health": {
    "default": {
      "provider_health_snapshot": "provider-health/file"
    }
  },
  "graph_compiler": {
    "default": {
      "graph_compilation_snapshot": "graph-compiler/file",
      "graph_fingerprint": "graph-fingerprint/file"
    }
  },
  "admission": {
    "default": {
      "admission_policy_snapshot": "admission-policy/file",
      "outcome_kind": "admit",
      "scope": "session",
      "reason": "cp_admission_allowed"
    }
  },
  "lease": {
    "default": {
      "lease_resolution_snapshot": "lease-resolution/file",
      "authority_epoch": 9,
      "authority_epoch_valid": true,
      "authority_authorized": true,
      "reason": "lease_authorized"
    }
  }
}`)

	backends, err := NewFileBackends(FileAdapterConfig{Path: path})
	if err != nil {
		t.Fatalf("expected file-backed backends, got %v", err)
	}

	record, err := backends.Registry.ResolvePipelineRecord("pipeline-requested")
	if err == nil {
		t.Fatalf("expected registry lookup mismatch to fail for unknown key")
	}
	_ = record

	record, err = backends.Registry.ResolvePipelineRecord("pipeline-file")
	if err != nil {
		t.Fatalf("registry resolve: %v", err)
	}
	if record.GraphDefinitionRef != "graph/file" || record.ExecutionProfile != "simple" {
		t.Fatalf("unexpected registry record: %+v", record)
	}

	versionOut, err := backends.Rollout.ResolvePipelineVersion(rollout.ResolveVersionInput{
		SessionID:                "sess-file-1",
		RequestedPipelineVersion: "pipeline-requested",
	})
	if err != nil {
		t.Fatalf("rollout resolve: %v", err)
	}
	if versionOut.PipelineVersion != "pipeline-file" || versionOut.VersionResolutionSnapshot != "version-resolution/file" {
		t.Fatalf("unexpected rollout output: %+v", versionOut)
	}

	routingOut, err := backends.RoutingView.GetSnapshot(routingview.Input{SessionID: "sess-file-1", PipelineVersion: "pipeline-file", AuthorityEpoch: 1})
	if err != nil {
		t.Fatalf("routing resolve: %v", err)
	}
	if routingOut.RoutingViewSnapshot != "routing-view/file" || routingOut.AdmissionPolicySnapshot != "admission-policy/file" || routingOut.ABICompatibilitySnapshot != "abi-compat/file" {
		t.Fatalf("unexpected routing output: %+v", routingOut)
	}

	policyOut, err := backends.Policy.Evaluate(policy.Input{SessionID: "sess-file-1", PipelineVersion: "pipeline-file"})
	if err != nil {
		t.Fatalf("policy resolve: %v", err)
	}
	if policyOut.PolicyResolutionSnapshot != "policy-resolution/file" || len(policyOut.AllowedAdaptiveActions) != 2 {
		t.Fatalf("unexpected policy output: %+v", policyOut)
	}

	providerHealthOut, err := backends.ProviderHealth.GetSnapshot(providerhealth.Input{Scope: "sess-file-1", PipelineVersion: "pipeline-file"})
	if err != nil {
		t.Fatalf("provider health resolve: %v", err)
	}
	if providerHealthOut.ProviderHealthSnapshot != "provider-health/file" {
		t.Fatalf("unexpected provider health output: %+v", providerHealthOut)
	}

	graphOut, err := backends.GraphCompiler.Compile(graphcompiler.Input{
		PipelineVersion:    "pipeline-file",
		GraphDefinitionRef: "graph/file",
		ExecutionProfile:   "simple",
	})
	if err != nil {
		t.Fatalf("graph compiler resolve: %v", err)
	}
	if graphOut.GraphCompilationSnapshot != "graph-compiler/file" || graphOut.GraphFingerprint != "graph-fingerprint/file" {
		t.Fatalf("unexpected graph compiler output: %+v", graphOut)
	}

	admissionOut, err := backends.Admission.Evaluate(admission.Input{
		SessionID:       "sess-file-1",
		TurnID:          "turn-file-1",
		PipelineVersion: "pipeline-file",
	})
	if err != nil {
		t.Fatalf("admission resolve: %v", err)
	}
	if admissionOut.AdmissionPolicySnapshot != "admission-policy/file" || admissionOut.OutcomeKind != controlplane.OutcomeAdmit {
		t.Fatalf("unexpected admission output: %+v", admissionOut)
	}

	leaseOut, err := backends.Lease.Resolve(lease.Input{
		SessionID:               "sess-file-1",
		PipelineVersion:         "pipeline-file",
		RequestedAuthorityEpoch: 5,
	})
	if err != nil {
		t.Fatalf("lease resolve: %v", err)
	}
	if leaseOut.LeaseResolutionSnapshot != "lease-resolution/file" || leaseOut.AuthorityEpoch != 9 {
		t.Fatalf("unexpected lease output: %+v", leaseOut)
	}
	if leaseOut.AuthorityEpochValid == nil || !*leaseOut.AuthorityEpochValid || leaseOut.AuthorityAuthorized == nil || !*leaseOut.AuthorityAuthorized {
		t.Fatalf("expected lease authority booleans to be true, got %+v", leaseOut)
	}
}

func TestNewFileBackendsFromEnv(t *testing.T) {
	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {"records": {"pipeline-v1": {"graph_definition_ref": "graph/default", "execution_profile": "simple"}}},
  "rollout": {"default_pipeline_version": "pipeline-v1"},
  "routing_view": {"default": {"routing_view_snapshot": "routing-view/v1", "admission_policy_snapshot": "admission-policy/v1", "abi_compatibility_snapshot": "abi-compat/v1"}},
  "policy": {"default": {"policy_resolution_snapshot": "policy-resolution/v1", "allowed_adaptive_actions": ["retry"]}},
  "provider_health": {"default": {"provider_health_snapshot": "provider-health/v1"}},
  "graph_compiler": {"default": {"graph_compilation_snapshot": "graph-compiler/v1"}},
  "admission": {"default": {"admission_policy_snapshot": "admission-policy/v1", "outcome_kind": "admit", "scope": "session", "reason": "cp_admission_allowed"}},
  "lease": {"default": {"lease_resolution_snapshot": "lease-resolution/v1", "authority_epoch_valid": true, "authority_authorized": true}}
}`)
	t.Setenv(EnvFileAdapterPath, path)

	backends, err := NewFileBackendsFromEnv()
	if err != nil {
		t.Fatalf("expected env-backed file adapter to load, got %v", err)
	}
	if backends.Registry == nil || backends.Rollout == nil || backends.RoutingView == nil || backends.Policy == nil || backends.ProviderHealth == nil || backends.GraphCompiler == nil || backends.Admission == nil || backends.Lease == nil {
		t.Fatalf("expected all service backends to be initialized")
	}
}

func TestNewFileBackendsStaleSnapshotErrorClassification(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "routing_view": {"stale": true}
}`)

	backends, err := NewFileBackends(FileAdapterConfig{Path: path})
	if err != nil {
		t.Fatalf("new file backends: %v", err)
	}

	_, err = backends.RoutingView.GetSnapshot(routingview.Input{SessionID: "sess-stale-1", PipelineVersion: "pipeline-v1", AuthorityEpoch: 1})
	if err == nil {
		t.Fatalf("expected stale snapshot error")
	}
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeSnapshotStale {
		t.Fatalf("expected stale snapshot code, got %s", backendErr.Code)
	}
	if !backendErr.StaleSnapshot() {
		t.Fatalf("expected stale snapshot marker")
	}
}

func TestNewFileBackendsRequiresValidSchema(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{"schema_version":"unsupported-schema"}`)
	_, err := NewFileBackends(FileAdapterConfig{Path: path})
	if err == nil {
		t.Fatalf("expected unsupported schema failure")
	}
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeInvalidArtifact {
		t.Fatalf("expected invalid artifact code, got %s", backendErr.Code)
	}
}

func writeDistributionArtifact(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "distribution.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	return path
}
