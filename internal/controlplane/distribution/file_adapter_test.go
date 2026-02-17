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
      "abi_compatibility_snapshot": "abi-compat/file",
      "transport_kind": "livekit",
      "transport_endpoint": "wss://runtime-file.rspp.local/livekit/pipeline-file",
      "runtime_id": "runtime-file-a"
    }
  },
  "policy": {
    "default": {
      "policy_resolution_snapshot": "policy-resolution/file",
      "allowed_adaptive_actions": ["fallback", "retry"],
      "budgets": {
        "turn_budget_ms": 6500,
        "node_budget_ms_default": 1400,
        "path_budget_ms_default": 4100,
        "edge_budget_ms_default": 700
      },
      "provider_bindings": {
        "stt": "stt-file",
        "llm": "llm-file",
        "tts": "tts-file"
      },
      "edge_buffer_policies": {
        "default": {
          "strategy": "drop",
          "max_queue_items": 40,
          "max_queue_ms": 220,
          "max_queue_bytes": 180000,
          "max_latency_contribution_ms": 90,
          "watermarks": {
            "queue_items": {"high": 25, "low": 10}
          },
          "lane_handling": {
            "DataLane": "drop",
            "ControlLane": "non_blocking_priority",
            "TelemetryLane": "best_effort_drop"
          },
          "defaulting_source": "explicit_edge_config"
        }
      },
      "node_execution_policies": {
        "provider-heavy": {
          "concurrency_limit": 1,
          "fairness_key": "provider-heavy"
        }
      },
      "flow_control": {
        "mode_by_lane": {
          "DataLane": "signal",
          "ControlLane": "signal",
          "TelemetryLane": "signal"
        },
        "watermarks": {
          "DataLane": {"high": 70, "low": 20},
          "ControlLane": {"high": 20, "low": 10},
          "TelemetryLane": {"high": 100, "low": 40}
        },
        "shedding_strategy_by_lane": {
          "DataLane": "drop",
          "ControlLane": "none",
          "TelemetryLane": "sample"
        }
      },
      "recording_policy": {
        "recording_level": "L0",
        "allowed_replay_modes": ["replay_decisions"]
      }
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
      "reason": "cp_admission_allowed",
      "session_rate_limit_per_min": 100,
      "session_rate_observed_per_min": 10
    }
  },
  "lease": {
    "default": {
      "lease_resolution_snapshot": "lease-resolution/file",
      "authority_epoch": 9,
      "authority_epoch_valid": true,
      "authority_authorized": true,
      "reason": "lease_authorized",
      "lease_token_id": "lease-token-file",
      "lease_expires_at_utc": "2026-02-17T00:45:00Z"
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
	if routingOut.TransportKind != "livekit" || routingOut.TransportEndpoint == "" || routingOut.RuntimeID != "runtime-file-a" {
		t.Fatalf("expected routing transport metadata, got %+v", routingOut)
	}

	policyOut, err := backends.Policy.Evaluate(policy.Input{SessionID: "sess-file-1", PipelineVersion: "pipeline-file"})
	if err != nil {
		t.Fatalf("policy resolve: %v", err)
	}
	if policyOut.PolicyResolutionSnapshot != "policy-resolution/file" || len(policyOut.AllowedAdaptiveActions) != 2 {
		t.Fatalf("unexpected policy output: %+v", policyOut)
	}
	if policyOut.ResolvedPolicy.Budgets.TurnBudgetMS != 6500 {
		t.Fatalf("expected policy budgets from distribution snapshot, got %+v", policyOut.ResolvedPolicy.Budgets)
	}
	if policyOut.ResolvedPolicy.ProviderBindings["llm"] != "llm-file" {
		t.Fatalf("expected policy provider_bindings from distribution snapshot, got %+v", policyOut.ResolvedPolicy.ProviderBindings)
	}
	if edge, ok := policyOut.ResolvedPolicy.EdgeBufferPolicies["default"]; !ok || edge.MaxQueueItems != 40 {
		t.Fatalf("expected policy edge_buffer_policies from distribution snapshot, got %+v", policyOut.ResolvedPolicy.EdgeBufferPolicies)
	}
	if nodePolicy, ok := policyOut.ResolvedPolicy.NodeExecutionPolicies["provider-heavy"]; !ok || nodePolicy.ConcurrencyLimit != 1 || nodePolicy.FairnessKey != "provider-heavy" {
		t.Fatalf("expected policy node_execution_policies from distribution snapshot, got %+v", policyOut.ResolvedPolicy.NodeExecutionPolicies)
	}
	if err := policyOut.ResolvedPolicy.FlowControl.Validate(); err != nil {
		t.Fatalf("expected flow_control from distribution snapshot to validate, got %v", err)
	}
	if err := policyOut.ResolvedPolicy.RecordingPolicy.Validate(); err != nil {
		t.Fatalf("expected recording_policy from distribution snapshot to validate, got %v", err)
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
	if admissionOut.SessionRateLimitPerMin != 100 || admissionOut.SessionRateObservedPM != 10 {
		t.Fatalf("expected admission quota fields from distribution snapshot, got %+v", admissionOut)
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
	if leaseOut.LeaseTokenID != "lease-token-file" || leaseOut.LeaseExpiresAtUTC != "2026-02-17T00:45:00Z" {
		t.Fatalf("expected lease token metadata from distribution snapshot, got %+v", leaseOut)
	}
}

func TestFileRolloutBackendCanaryAllowlistAndPercentage(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "records": {
      "pipeline-v1": {
        "pipeline_version": "pipeline-v1",
        "graph_definition_ref": "graph/default",
        "execution_profile": "simple"
      }
    }
  },
  "rollout": {
    "default_pipeline_version": "pipeline-v1",
    "canary_pipeline_version": "pipeline-v2",
    "canary_percentage": 100,
    "tenant_allowlist": ["tenant-gold"]
  },
  "routing_view": {"default": {"routing_view_snapshot": "routing-view/v1", "admission_policy_snapshot": "admission-policy/v1", "abi_compatibility_snapshot": "abi-compat/v1"}},
  "policy": {"default": {"policy_resolution_snapshot": "policy-resolution/v1", "allowed_adaptive_actions": ["retry"]}},
  "provider_health": {"default": {"provider_health_snapshot": "provider-health/v1"}},
  "graph_compiler": {"default": {"graph_compilation_snapshot": "graph-compiler/v1"}},
  "admission": {"default": {"admission_policy_snapshot": "admission-policy/v1", "outcome_kind": "admit", "scope": "session", "reason": "cp_admission_allowed"}},
  "lease": {"default": {"lease_resolution_snapshot": "lease-resolution/v1", "authority_epoch_valid": true, "authority_authorized": true}}
}`)

	backends, err := NewFileBackends(FileAdapterConfig{Path: path})
	if err != nil {
		t.Fatalf("expected file-backed backends, got %v", err)
	}

	allowlisted, err := backends.Rollout.ResolvePipelineVersion(rollout.ResolveVersionInput{
		SessionID:                "sess-allowlisted",
		TenantID:                 "tenant-gold",
		RequestedPipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("allowlisted rollout resolve: %v", err)
	}
	if allowlisted.PipelineVersion != "pipeline-v2" {
		t.Fatalf("expected allowlisted tenant to route to canary, got %+v", allowlisted)
	}

	percentageRouted, err := backends.Rollout.ResolvePipelineVersion(rollout.ResolveVersionInput{
		SessionID:                "sess-percentage",
		TenantID:                 "tenant-standard",
		RequestedPipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("percentage rollout resolve: %v", err)
	}
	if percentageRouted.PipelineVersion != "pipeline-v2" {
		t.Fatalf("expected 100%% canary routing to select canary, got %+v", percentageRouted)
	}
}

func TestNewFileBackendsRejectsInvalidRolloutCanaryConfig(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {"records": {"pipeline-v1": {"graph_definition_ref": "graph/default", "execution_profile": "simple"}}},
  "rollout": {"default_pipeline_version": "pipeline-v1", "canary_percentage": 10},
  "routing_view": {"default": {"routing_view_snapshot": "routing-view/v1", "admission_policy_snapshot": "admission-policy/v1", "abi_compatibility_snapshot": "abi-compat/v1"}},
  "policy": {"default": {"policy_resolution_snapshot": "policy-resolution/v1", "allowed_adaptive_actions": ["retry"]}},
  "provider_health": {"default": {"provider_health_snapshot": "provider-health/v1"}},
  "graph_compiler": {"default": {"graph_compilation_snapshot": "graph-compiler/v1"}},
  "admission": {"default": {"admission_policy_snapshot": "admission-policy/v1", "outcome_kind": "admit", "scope": "session", "reason": "cp_admission_allowed"}},
  "lease": {"default": {"lease_resolution_snapshot": "lease-resolution/v1", "authority_epoch_valid": true, "authority_authorized": true}}
}`)

	_, err := NewFileBackends(FileAdapterConfig{Path: path})
	if err == nil {
		t.Fatalf("expected invalid canary rollout config error")
	}
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeInvalidArtifact {
		t.Fatalf("expected invalid artifact code, got %s", backendErr.Code)
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

func TestFilePolicyBackendTenantOverride(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {"records": {"pipeline-v1": {"pipeline_version": "pipeline-v1", "graph_definition_ref": "graph/default", "execution_profile": "simple"}}},
  "rollout": {"default_pipeline_version": "pipeline-v1"},
  "routing_view": {"default": {"routing_view_snapshot": "routing-view/v1", "admission_policy_snapshot": "admission-policy/v1", "abi_compatibility_snapshot": "abi-compat/v1"}},
  "policy": {
    "default": {
      "policy_resolution_snapshot": "policy-resolution/default",
      "node_execution_policies": {
        "provider-heavy": {"concurrency_limit": 2, "fairness_key": "default-heavy"}
      }
    },
    "by_tenant": {
      "tenant-gold": {
        "policy_resolution_snapshot": "policy-resolution/tenant-gold",
        "node_execution_policies": {
          "provider-heavy": {"concurrency_limit": 1, "fairness_key": "tenant-gold-heavy"}
        }
      }
    }
  },
  "provider_health": {"default": {"provider_health_snapshot": "provider-health/v1"}},
  "graph_compiler": {"default": {"graph_compilation_snapshot": "graph-compiler/v1"}},
  "admission": {"default": {"admission_policy_snapshot": "admission-policy/v1", "outcome_kind": "admit", "scope": "session", "reason": "cp_admission_allowed"}},
  "lease": {"default": {"lease_resolution_snapshot": "lease-resolution/v1", "authority_epoch_valid": true, "authority_authorized": true}}
}`)

	backends, err := NewFileBackends(FileAdapterConfig{Path: path})
	if err != nil {
		t.Fatalf("new file backends: %v", err)
	}

	defaultPolicyOut, err := backends.Policy.Evaluate(policy.Input{
		SessionID:       "sess-policy-default-1",
		PipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("default policy evaluate: %v", err)
	}
	if defaultPolicyOut.PolicyResolutionSnapshot != "policy-resolution/default" {
		t.Fatalf("expected default policy snapshot, got %+v", defaultPolicyOut)
	}
	if nodePolicy, ok := defaultPolicyOut.ResolvedPolicy.NodeExecutionPolicies["provider-heavy"]; !ok || nodePolicy.ConcurrencyLimit != 2 || nodePolicy.FairnessKey != "default-heavy" {
		t.Fatalf("unexpected default node execution policy: %+v", defaultPolicyOut.ResolvedPolicy.NodeExecutionPolicies)
	}

	tenantPolicyOut, err := backends.Policy.Evaluate(policy.Input{
		TenantID:        "tenant-gold",
		SessionID:       "sess-policy-tenant-1",
		PipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("tenant policy evaluate: %v", err)
	}
	if tenantPolicyOut.PolicyResolutionSnapshot != "policy-resolution/tenant-gold" {
		t.Fatalf("expected tenant policy snapshot override, got %+v", tenantPolicyOut)
	}
	if nodePolicy, ok := tenantPolicyOut.ResolvedPolicy.NodeExecutionPolicies["provider-heavy"]; !ok || nodePolicy.ConcurrencyLimit != 1 || nodePolicy.FairnessKey != "tenant-gold-heavy" {
		t.Fatalf("unexpected tenant node execution policy override: %+v", tenantPolicyOut.ResolvedPolicy.NodeExecutionPolicies)
	}
}

func TestFileAdmissionBackendTenantOverridePrecedence(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "admission": {
    "default": {
      "admission_policy_snapshot": "admission-policy/default",
      "outcome_kind": "admit",
      "scope": "session",
      "reason": "cp_admission_allowed"
    },
    "by_pipeline": {
      "pipeline-v1": {
        "admission_policy_snapshot": "admission-policy/pipeline-v1",
        "outcome_kind": "reject",
        "scope": "session",
        "reason": "cp_admission_reject_policy"
      }
    },
    "by_tenant": {
      "tenant-gold": {
        "admission_policy_snapshot": "admission-policy/tenant-gold",
        "outcome_kind": "defer",
        "scope": "tenant",
        "reason": "cp_admission_defer_capacity",
        "session_rate_limit_per_min": 2,
        "session_rate_observed_per_min": 3
      }
    }
  }
}`)

	backends, err := NewFileBackends(FileAdapterConfig{Path: path})
	if err != nil {
		t.Fatalf("new file backends: %v", err)
	}

	byPipeline, err := backends.Admission.Evaluate(admission.Input{
		SessionID:       "sess-admission-pipeline-1",
		TurnID:          "turn-admission-pipeline-1",
		PipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("pipeline admission evaluate: %v", err)
	}
	if byPipeline.AdmissionPolicySnapshot != "admission-policy/pipeline-v1" ||
		byPipeline.OutcomeKind != controlplane.OutcomeReject ||
		byPipeline.Scope != controlplane.ScopeSession ||
		byPipeline.Reason != "cp_admission_reject_policy" {
		t.Fatalf("unexpected pipeline admission output: %+v", byPipeline)
	}

	byTenant, err := backends.Admission.Evaluate(admission.Input{
		TenantID:        "tenant-gold",
		SessionID:       "sess-admission-tenant-1",
		TurnID:          "turn-admission-tenant-1",
		PipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("tenant admission evaluate: %v", err)
	}
	if byTenant.AdmissionPolicySnapshot != "admission-policy/tenant-gold" ||
		byTenant.OutcomeKind != controlplane.OutcomeDefer ||
		byTenant.Scope != controlplane.ScopeTenant ||
		byTenant.Reason != "cp_admission_defer_capacity" {
		t.Fatalf("unexpected tenant admission output: %+v", byTenant)
	}
	if byTenant.SessionRateLimitPerMin != 2 || byTenant.SessionRateObservedPM != 3 {
		t.Fatalf("expected tenant quota fields in admission output, got %+v", byTenant)
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
