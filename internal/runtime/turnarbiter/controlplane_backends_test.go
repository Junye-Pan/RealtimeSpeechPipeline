package turnarbiter

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/distribution"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
)

func TestNewControlPlaneBundleResolverWithBackends(t *testing.T) {
	t.Parallel()

	resolver := NewControlPlaneBundleResolverWithBackends(ControlPlaneBackends{
		Registry: stubRegistryBackend{
			resolveFn: func(version string) (registry.PipelineRecord, error) {
				return registry.PipelineRecord{
					PipelineVersion:    version,
					GraphDefinitionRef: "graph/backend-wired",
					ExecutionProfile:   "simple",
				}, nil
			},
		},
		Rollout: stubRolloutBackend{
			resolveFn: func(rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
				return rollout.ResolveVersionOutput{
					PipelineVersion:           "pipeline/backend-wired",
					VersionResolutionSnapshot: "version-resolution/backend-wired",
				}, nil
			},
		},
		RoutingView: stubRoutingBackend{
			getFn: func(routingview.Input) (routingview.Snapshot, error) {
				return routingview.Snapshot{
					RoutingViewSnapshot:      "routing-view/backend-wired",
					AdmissionPolicySnapshot:  "admission-policy/backend-wired",
					ABICompatibilitySnapshot: "abi-compat/backend-wired",
				}, nil
			},
		},
		Policy: stubPolicyBackend{
			evalFn: func(policy.Input) (policy.Output, error) {
				return policy.Output{
					PolicyResolutionSnapshot: "policy-resolution/backend-wired",
					AllowedAdaptiveActions:   []string{"fallback", "retry"},
				}, nil
			},
		},
		ProviderHealth: stubProviderHealthBackend{
			getFn: func(providerhealth.Input) (providerhealth.Output, error) {
				return providerhealth.Output{ProviderHealthSnapshot: "provider-health/backend-wired"}, nil
			},
		},
		GraphCompiler: stubGraphCompilerBackend{
			compileFn: func(graphcompiler.Input) (graphcompiler.Output, error) {
				return graphcompiler.Output{
					GraphDefinitionRef:       "graph/backend-compiled",
					GraphCompilationSnapshot: "graph-compiler/backend-wired",
					GraphFingerprint:         "graph-fingerprint/backend-wired",
				}, nil
			},
		},
		Admission: stubAdmissionBackend{
			evalFn: func(admission.Input) (admission.Output, error) {
				return admission.Output{
					AdmissionPolicySnapshot: "admission-policy/backend-wired",
					OutcomeKind:             controlplane.OutcomeAdmit,
					Scope:                   controlplane.ScopeSession,
					Reason:                  "cp_admission_allowed",
				}, nil
			},
		},
		Lease: stubLeaseBackend{
			resolveFn: func(lease.Input) (lease.Output, error) {
				return lease.Output{
					LeaseResolutionSnapshot: "lease-resolution/backend-wired",
					AuthorityEpoch:          9,
					AuthorityEpochValid:     boolPtr(true),
					AuthorityAuthorized:     boolPtr(true),
					Reason:                  "lease_authorized",
				}, nil
			},
		},
	})

	bundle, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                "sess-backend-wired-1",
		TurnID:                   "turn-backend-wired-1",
		RequestedPipelineVersion: "pipeline-requested",
		AuthorityEpoch:           3,
	})
	if err != nil {
		t.Fatalf("unexpected backend-wired resolver error: %v", err)
	}
	if bundle.PipelineVersion != "pipeline/backend-wired" ||
		bundle.GraphDefinitionRef != "graph/backend-compiled" ||
		bundle.ExecutionProfile != "simple" {
		t.Fatalf("unexpected backend-wired bundle fields: %+v", bundle)
	}
	if !reflect.DeepEqual(bundle.AllowedAdaptiveActions, []string{"retry", "fallback"}) {
		t.Fatalf("expected normalized backend adaptive actions, got %+v", bundle.AllowedAdaptiveActions)
	}
	if bundle.SnapshotProvenance.RoutingViewSnapshot != "routing-view/backend-wired" ||
		bundle.SnapshotProvenance.AdmissionPolicySnapshot != "admission-policy/backend-wired" ||
		bundle.SnapshotProvenance.ABICompatibilitySnapshot != "abi-compat/backend-wired" ||
		bundle.SnapshotProvenance.VersionResolutionSnapshot != "version-resolution/backend-wired" ||
		bundle.SnapshotProvenance.PolicyResolutionSnapshot != "policy-resolution/backend-wired" ||
		bundle.SnapshotProvenance.ProviderHealthSnapshot != "provider-health/backend-wired" {
		t.Fatalf("unexpected backend-wired bundle provenance: %+v", bundle.SnapshotProvenance)
	}
	if !bundle.HasCPAdmissionDecision || bundle.CPAdmissionOutcomeKind != controlplane.OutcomeAdmit || bundle.CPAdmissionReason != "cp_admission_allowed" {
		t.Fatalf("expected CP admission decision metadata from backend, got %+v", bundle)
	}
	if !bundle.HasLeaseDecision || !bundle.LeaseAuthorityValid || !bundle.LeaseAuthorityGranted || bundle.LeaseAuthorityEpoch != 9 {
		t.Fatalf("expected lease decision metadata from backend, got %+v", bundle)
	}
	if bundle.LeaseTokenID == "" || bundle.LeaseTokenExpiresAtUTC == "" {
		t.Fatalf("expected lease token metadata in turn-start bundle, got %+v", bundle)
	}
}

func TestNewControlPlaneBundleResolverWithBackendsThreadsTenantIDToAdmission(t *testing.T) {
	t.Parallel()

	var capturedTenantID string
	resolver := NewControlPlaneBundleResolverWithBackends(ControlPlaneBackends{
		Admission: stubAdmissionBackend{
			evalFn: func(in admission.Input) (admission.Output, error) {
				capturedTenantID = in.TenantID
				return admission.Output{
					AdmissionPolicySnapshot: "admission-policy/tenant",
					OutcomeKind:             controlplane.OutcomeAdmit,
					Reason:                  admission.ReasonAllowed,
				}, nil
			},
		},
	})

	bundle, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		TenantID:                 "tenant-backend-1",
		SessionID:                "sess-tenant-backend-1",
		TurnID:                   "turn-tenant-backend-1",
		RequestedPipelineVersion: "pipeline-requested",
		AuthorityEpoch:           5,
	})
	if err != nil {
		t.Fatalf("unexpected resolver error: %v", err)
	}
	if capturedTenantID != "tenant-backend-1" {
		t.Fatalf("expected tenant_id threading to admission backend, got %q", capturedTenantID)
	}
	if !bundle.HasCPAdmissionDecision || bundle.CPAdmissionOutcomeKind != controlplane.OutcomeAdmit {
		t.Fatalf("expected admit CP admission decision, got %+v", bundle)
	}
	if bundle.CPAdmissionScope != controlplane.ScopeTenant {
		t.Fatalf("expected tenant-scoped CP admission when tenant_id is set, got %+v", bundle)
	}
}

func TestNewControlPlaneBundleResolverWithBackendsThreadsTenantIDToPolicy(t *testing.T) {
	t.Parallel()

	var capturedTenantID string
	resolver := NewControlPlaneBundleResolverWithBackends(ControlPlaneBackends{
		Policy: stubPolicyBackend{
			evalFn: func(in policy.Input) (policy.Output, error) {
				capturedTenantID = in.TenantID
				return policy.Output{
					PolicyResolutionSnapshot: "policy-resolution/tenant",
					AllowedAdaptiveActions:   []string{"retry"},
					ResolvedPolicy: policy.ResolvedTurnPolicy{
						Budgets: controlplane.Budgets{
							TurnBudgetMS:        5000,
							NodeBudgetMSDefault: 1500,
							PathBudgetMSDefault: 3000,
							EdgeBudgetMSDefault: 500,
						},
						ProviderBindings: map[string]string{
							"stt": "stt-default",
							"llm": "llm-default",
							"tts": "tts-default",
						},
						EdgeBufferPolicies: map[string]controlplane.EdgeBufferPolicy{
							"default": {
								Strategy:                 controlplane.BufferStrategyDrop,
								MaxQueueItems:            64,
								MaxQueueMS:               300,
								MaxQueueBytes:            262144,
								MaxLatencyContributionMS: 120,
								Watermarks: controlplane.EdgeWatermarks{
									QueueItems: &controlplane.WatermarkThreshold{High: 48, Low: 24},
								},
								LaneHandling: controlplane.LaneHandling{
									DataLane:      "drop",
									ControlLane:   "non_blocking_priority",
									TelemetryLane: "best_effort_drop",
								},
								DefaultingSource: "execution_profile_default",
							},
						},
						NodeExecutionPolicies: map[string]controlplane.NodeExecutionPolicy{
							"provider-heavy": {
								ConcurrencyLimit: 1,
								FairnessKey:      "tenant-policy-heavy",
							},
						},
						FlowControl: controlplane.FlowControl{
							ModeByLane: controlplane.ModeByLane{
								DataLane:      "signal",
								ControlLane:   "signal",
								TelemetryLane: "signal",
							},
							Watermarks: controlplane.FlowWatermarks{
								DataLane:      controlplane.WatermarkThreshold{High: 100, Low: 50},
								ControlLane:   controlplane.WatermarkThreshold{High: 20, Low: 10},
								TelemetryLane: controlplane.WatermarkThreshold{High: 200, Low: 100},
							},
							SheddingStrategyByLane: controlplane.SheddingStrategyByLane{
								DataLane:      "drop",
								ControlLane:   "none",
								TelemetryLane: "sample",
							},
						},
						RecordingPolicy: controlplane.RecordingPolicy{
							RecordingLevel:     "L0",
							AllowedReplayModes: []string{"replay_decisions"},
						},
					},
				}, nil
			},
		},
	})

	bundle, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		TenantID:                 "tenant-policy-1",
		SessionID:                "sess-policy-backend-1",
		TurnID:                   "turn-policy-backend-1",
		RequestedPipelineVersion: "pipeline-requested",
		AuthorityEpoch:           5,
	})
	if err != nil {
		t.Fatalf("unexpected resolver error: %v", err)
	}
	if capturedTenantID != "tenant-policy-1" {
		t.Fatalf("expected tenant_id threading to policy backend, got %q", capturedTenantID)
	}
	if policyOut, ok := bundle.NodeExecutionPolicies["provider-heavy"]; !ok || policyOut.FairnessKey != "tenant-policy-heavy" {
		t.Fatalf("expected tenant policy node_execution_policies in turn-start bundle, got %+v", bundle.NodeExecutionPolicies)
	}
}

func TestNewControlPlaneBundleResolverWithBackendsFallsBackPerService(t *testing.T) {
	t.Parallel()

	resolver := NewControlPlaneBundleResolverWithBackends(ControlPlaneBackends{
		Registry: stubRegistryBackend{
			resolveFn: func(string) (registry.PipelineRecord, error) {
				return registry.PipelineRecord{}, errors.New("registry backend unavailable")
			},
		},
		Rollout: stubRolloutBackend{
			resolveFn: func(rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
				return rollout.ResolveVersionOutput{
					PipelineVersion:           "pipeline/partial-backend",
					VersionResolutionSnapshot: "version-resolution/partial-backend",
				}, nil
			},
		},
		RoutingView: stubRoutingBackend{
			getFn: func(routingview.Input) (routingview.Snapshot, error) {
				return routingview.Snapshot{
					RoutingViewSnapshot:      "routing-view/partial-backend",
					AdmissionPolicySnapshot:  "admission-policy/partial-backend",
					ABICompatibilitySnapshot: "abi-compat/partial-backend",
				}, nil
			},
		},
		Policy: stubPolicyBackend{
			evalFn: func(policy.Input) (policy.Output, error) {
				return policy.Output{
					PolicyResolutionSnapshot: "policy-resolution/partial-backend",
					AllowedAdaptiveActions:   []string{"fallback", "retry"},
				}, nil
			},
		},
		ProviderHealth: stubProviderHealthBackend{
			getFn: func(providerhealth.Input) (providerhealth.Output, error) {
				return providerhealth.Output{ProviderHealthSnapshot: "provider-health/partial-backend"}, nil
			},
		},
		GraphCompiler: stubGraphCompilerBackend{
			compileFn: func(graphcompiler.Input) (graphcompiler.Output, error) {
				return graphcompiler.Output{}, errors.New("graph compiler backend unavailable")
			},
		},
		Admission: stubAdmissionBackend{
			evalFn: func(admission.Input) (admission.Output, error) {
				return admission.Output{}, errors.New("admission backend unavailable")
			},
		},
		Lease: stubLeaseBackend{
			resolveFn: func(lease.Input) (lease.Output, error) {
				return lease.Output{}, errors.New("lease backend unavailable")
			},
		},
	})

	bundle, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                "sess-partial-backend-1",
		TurnID:                   "turn-partial-backend-1",
		RequestedPipelineVersion: "pipeline-requested",
		AuthorityEpoch:           3,
	})
	if err != nil {
		t.Fatalf("unexpected partial-backend resolver error: %v", err)
	}
	if bundle.GraphDefinitionRef != registry.DefaultGraphDefinitionRef || bundle.ExecutionProfile != registry.DefaultExecutionProfile {
		t.Fatalf("expected registry fallback defaults after backend error, got %+v", bundle)
	}
	if bundle.PipelineVersion != "pipeline/partial-backend" {
		t.Fatalf("expected rollout backend version to remain active, got %+v", bundle)
	}
	if bundle.SnapshotProvenance.RoutingViewSnapshot != "routing-view/partial-backend" ||
		bundle.SnapshotProvenance.AdmissionPolicySnapshot != "admission-policy/partial-backend" ||
		bundle.SnapshotProvenance.ABICompatibilitySnapshot != "abi-compat/partial-backend" ||
		bundle.SnapshotProvenance.VersionResolutionSnapshot != "version-resolution/partial-backend" ||
		bundle.SnapshotProvenance.PolicyResolutionSnapshot != "policy-resolution/partial-backend" ||
		bundle.SnapshotProvenance.ProviderHealthSnapshot != "provider-health/partial-backend" {
		t.Fatalf("unexpected partial-backend snapshot provenance: %+v", bundle.SnapshotProvenance)
	}
	if !bundle.HasCPAdmissionDecision || bundle.CPAdmissionOutcomeKind != controlplane.OutcomeAdmit || bundle.CPAdmissionReason == "" {
		t.Fatalf("expected admission fallback defaults after backend error, got %+v", bundle)
	}
	if !bundle.HasLeaseDecision || !bundle.LeaseAuthorityValid || !bundle.LeaseAuthorityGranted {
		t.Fatalf("expected lease fallback defaults after backend error, got %+v", bundle)
	}
	if bundle.LeaseTokenID == "" || bundle.LeaseTokenExpiresAtUTC == "" {
		t.Fatalf("expected fallback lease token metadata after backend error, got %+v", bundle)
	}
}

func TestNewControlPlaneBundleResolverWithBackendsStrictModePropagatesNonStaleErrors(t *testing.T) {
	t.Parallel()

	resolver := NewControlPlaneBundleResolverWithBackends(ControlPlaneBackends{
		FallbackMode: BackendFallbackModeStrict,
		Registry: stubRegistryBackend{
			resolveFn: func(string) (registry.PipelineRecord, error) {
				return registry.PipelineRecord{}, errors.New("registry backend unavailable")
			},
		},
	})

	_, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                "sess-strict-backend-1",
		TurnID:                   "turn-strict-backend-1",
		RequestedPipelineVersion: "pipeline-requested",
		AuthorityEpoch:           1,
	})
	if err == nil {
		t.Fatalf("expected strict-mode backend error propagation")
	}
	if !strings.Contains(err.Error(), "registry backend unavailable") {
		t.Fatalf("expected strict-mode resolver to include backend error, got %v", err)
	}
}

func TestNewControlPlaneBundleResolverWithBackendsPropagatesStaleErrors(t *testing.T) {
	t.Parallel()

	resolver := NewControlPlaneBundleResolverWithBackends(ControlPlaneBackends{
		Registry: stubRegistryBackend{
			resolveFn: func(version string) (registry.PipelineRecord, error) {
				return registry.PipelineRecord{
					PipelineVersion:    version,
					GraphDefinitionRef: "graph/backend",
					ExecutionProfile:   "simple",
				}, nil
			},
		},
		Rollout: stubRolloutBackend{
			resolveFn: func(rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
				return rollout.ResolveVersionOutput{PipelineVersion: "pipeline/backend"}, nil
			},
		},
		RoutingView: stubRoutingBackend{
			getFn: func(routingview.Input) (routingview.Snapshot, error) {
				return routingview.Snapshot{}, staleSnapshotStubError{reason: "routing snapshot stale"}
			},
		},
	})

	_, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                "sess-stale-1",
		TurnID:                   "turn-stale-1",
		RequestedPipelineVersion: "pipeline-requested",
		AuthorityEpoch:           3,
	})
	if err == nil {
		t.Fatalf("expected stale backend resolution error")
	}
	if !isStaleSnapshotResolutionError(err) {
		t.Fatalf("expected stale snapshot error classification, got %v", err)
	}
}

func TestNewWithControlPlaneBackendsFromDistributionFile(t *testing.T) {
	t.Parallel()

	artifactPath := writeDistributionFixture(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "records": {
      "pipeline-requested": {
        "pipeline_version": "pipeline-requested",
        "graph_definition_ref": "graph/distribution",
        "execution_profile": "simple"
      }
    }
  },
  "rollout": {
    "by_requested_version": {
      "pipeline-requested": "pipeline-distribution"
    },
    "version_resolution_snapshot": "version-resolution/distribution"
  },
  "routing_view": {
    "default": {
      "routing_view_snapshot": "routing-view/distribution",
      "admission_policy_snapshot": "admission-policy/distribution",
      "abi_compatibility_snapshot": "abi-compat/distribution"
    }
  },
  "policy": {
    "default": {
      "policy_resolution_snapshot": "policy-resolution/distribution",
      "allowed_adaptive_actions": ["fallback", "retry"]
    }
  },
  "provider_health": {
    "default": {
      "provider_health_snapshot": "provider-health/distribution"
    }
  }
}`)

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 8})
	arbiter, err := NewWithControlPlaneBackendsFromDistributionFile(&recorder, artifactPath)
	if err != nil {
		t.Fatalf("expected distribution-backed arbiter, got %v", err)
	}

	open, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-distribution-1",
		TurnID:               "turn-distribution-1",
		EventID:              "evt-distribution-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested",
		AuthorityEpoch:       3,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected open error with distribution backends: %v", err)
	}
	if open.State != controlplane.TurnActive || open.Plan == nil {
		t.Fatalf("expected active state and resolved plan, got %+v", open)
	}
	if open.Plan.PipelineVersion != "pipeline-distribution" || open.Plan.GraphDefinitionRef != "graph/distribution" {
		t.Fatalf("unexpected distribution-backed plan fields: %+v", open.Plan)
	}
	if !reflect.DeepEqual(open.Plan.AllowedAdaptiveActions, []string{"retry", "fallback"}) {
		t.Fatalf("expected normalized policy actions from distribution backend, got %+v", open.Plan.AllowedAdaptiveActions)
	}
}

func TestNewWithControlPlaneBackendsFromDistributionFileAppliesTenantAdmissionOverride(t *testing.T) {
	t.Parallel()

	artifactPath := writeDistributionFixture(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "records": {
      "pipeline-requested": {
        "pipeline_version": "pipeline-requested",
        "graph_definition_ref": "graph/distribution",
        "execution_profile": "simple"
      }
    }
  },
  "rollout": {
    "by_requested_version": {
      "pipeline-requested": "pipeline-distribution"
    },
    "version_resolution_snapshot": "version-resolution/distribution"
  },
  "routing_view": {
    "default": {
      "routing_view_snapshot": "routing-view/distribution",
      "admission_policy_snapshot": "admission-policy/distribution",
      "abi_compatibility_snapshot": "abi-compat/distribution"
    }
  },
  "policy": {
    "default": {
      "policy_resolution_snapshot": "policy-resolution/distribution",
      "allowed_adaptive_actions": ["fallback", "retry"]
    }
  },
  "provider_health": {
    "default": {
      "provider_health_snapshot": "provider-health/distribution"
    }
  },
  "admission": {
    "default": {
      "admission_policy_snapshot": "admission-policy/default",
      "outcome_kind": "admit",
      "scope": "session",
      "reason": "cp_admission_allowed"
    },
    "by_tenant": {
      "tenant-gold": {
        "admission_policy_snapshot": "admission-policy/tenant-gold",
        "outcome_kind": "defer",
        "scope": "tenant",
        "reason": "cp_admission_defer_capacity"
      }
    }
  }
}`)

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 8})
	arbiter, err := NewWithControlPlaneBackendsFromDistributionFile(&recorder, artifactPath)
	if err != nil {
		t.Fatalf("expected distribution-backed arbiter, got %v", err)
	}

	open, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		TenantID:             "tenant-gold",
		SessionID:            "sess-distribution-tenant-1",
		TurnID:               "turn-distribution-tenant-1",
		EventID:              "evt-distribution-tenant-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested",
		AuthorityEpoch:       3,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected open error with distribution backends: %v", err)
	}
	if open.State != controlplane.TurnIdle {
		t.Fatalf("expected tenant admission defer to keep turn idle, got %s", open.State)
	}
	if open.Plan != nil {
		t.Fatalf("expected no plan when CP admission defers turn, got %+v", open.Plan)
	}
	if open.Decision == nil ||
		open.Decision.OutcomeKind != controlplane.OutcomeDefer ||
		open.Decision.Scope != controlplane.ScopeTenant ||
		open.Decision.Reason != "cp_admission_defer_capacity" {
		t.Fatalf("expected tenant-scoped CP admission defer decision, got %+v", open.Decision)
	}
}

func TestNewControlPlaneBackendsFromDistributionEnv(t *testing.T) {
	artifactPath := writeDistributionFixture(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {"records": {"pipeline-v1": {"graph_definition_ref": "graph/default", "execution_profile": "simple"}}},
  "rollout": {"default_pipeline_version": "pipeline-v1"},
  "routing_view": {"default": {"routing_view_snapshot": "routing-view/v1", "admission_policy_snapshot": "admission-policy/v1", "abi_compatibility_snapshot": "abi-compat/v1"}},
  "policy": {"default": {"policy_resolution_snapshot": "policy-resolution/v1", "allowed_adaptive_actions": ["retry"]}},
  "provider_health": {"default": {"provider_health_snapshot": "provider-health/v1"}}
}`)
	t.Setenv(ControlPlaneDistributionPathEnv, artifactPath)

	backends, err := NewControlPlaneBackendsFromDistributionEnv()
	if err != nil {
		t.Fatalf("expected distribution env backends, got %v", err)
	}
	if backends.Registry == nil || backends.Rollout == nil || backends.RoutingView == nil || backends.Policy == nil || backends.ProviderHealth == nil || backends.GraphCompiler == nil || backends.Admission == nil || backends.Lease == nil {
		t.Fatalf("expected all distribution env backends to be initialized")
	}
	if backends.FallbackMode != BackendFallbackModeAvailability {
		t.Fatalf("expected availability fallback mode by default, got %s", backends.FallbackMode)
	}
}

func TestNewControlPlaneBackendsFromDistributionEnvStrictMode(t *testing.T) {
	artifactPath := writeDistributionFixture(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {"records": {"pipeline-v1": {"graph_definition_ref": "graph/default", "execution_profile": "simple"}}}
}`)
	t.Setenv(ControlPlaneDistributionPathEnv, artifactPath)
	t.Setenv(ControlPlaneBackendFallbackModeEnv, string(BackendFallbackModeStrict))

	backends, err := NewControlPlaneBackendsFromDistributionEnv()
	if err != nil {
		t.Fatalf("expected strict fallback mode config to load, got %v", err)
	}
	if backends.FallbackMode != BackendFallbackModeStrict {
		t.Fatalf("expected strict fallback mode, got %s", backends.FallbackMode)
	}
}

func TestNewControlPlaneBackendsFromDistributionEnvInvalidFallbackMode(t *testing.T) {
	t.Setenv(ControlPlaneBackendFallbackModeEnv, "unknown")
	_, err := NewControlPlaneBackendsFromDistributionEnv()
	if err == nil {
		t.Fatalf("expected invalid fallback mode error")
	}
	if !strings.Contains(err.Error(), ControlPlaneBackendFallbackModeEnv) {
		t.Fatalf("expected env-specific fallback mode validation error, got %v", err)
	}
}

func TestNewWithControlPlaneBackendsFromDistributionHTTP(t *testing.T) {
	t.Parallel()

	server := newDistributionHTTPFixtureServer(t, http.StatusOK, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "records": {
      "pipeline-requested": {
        "pipeline_version": "pipeline-requested",
        "graph_definition_ref": "graph/distribution-http",
        "execution_profile": "simple"
      }
    }
  },
  "rollout": {
    "by_requested_version": {
      "pipeline-requested": "pipeline-distribution-http"
    },
    "version_resolution_snapshot": "version-resolution/distribution-http"
  },
  "routing_view": {
    "default": {
      "routing_view_snapshot": "routing-view/distribution-http",
      "admission_policy_snapshot": "admission-policy/distribution-http",
      "abi_compatibility_snapshot": "abi-compat/distribution-http"
    }
  },
  "policy": {
    "default": {
      "policy_resolution_snapshot": "policy-resolution/distribution-http",
      "allowed_adaptive_actions": ["fallback", "retry"]
    }
  },
  "provider_health": {
    "default": {
      "provider_health_snapshot": "provider-health/distribution-http"
    }
  }
}`)
	defer server.Close()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 8})
	arbiter, err := NewWithControlPlaneBackendsFromDistributionHTTP(&recorder, server.URL)
	if err != nil {
		t.Fatalf("expected http distribution-backed arbiter, got %v", err)
	}

	open, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-distribution-http-1",
		TurnID:               "turn-distribution-http-1",
		EventID:              "evt-distribution-http-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested",
		AuthorityEpoch:       3,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected open error with http distribution backends: %v", err)
	}
	if open.State != controlplane.TurnActive || open.Plan == nil {
		t.Fatalf("expected active state and resolved plan, got %+v", open)
	}
	if open.Plan.PipelineVersion != "pipeline-distribution-http" || open.Plan.GraphDefinitionRef != "graph/distribution-http" {
		t.Fatalf("unexpected http distribution-backed plan fields: %+v", open.Plan)
	}
	if !reflect.DeepEqual(open.Plan.AllowedAdaptiveActions, []string{"retry", "fallback"}) {
		t.Fatalf("expected normalized policy actions from http distribution backend, got %+v", open.Plan.AllowedAdaptiveActions)
	}
}

func TestNewControlPlaneBackendsFromDistributionEnvPrefersHTTPWhenConfigured(t *testing.T) {
	server := newDistributionHTTPFixtureServer(t, http.StatusOK, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "records": {
      "pipeline-http-env": {
        "pipeline_version": "pipeline-http-env",
        "graph_definition_ref": "graph/http-env",
        "execution_profile": "simple"
      }
    }
  }
}`)
	defer server.Close()

	t.Setenv(ControlPlaneDistributionHTTPURLEnv, server.URL)
	t.Setenv(ControlPlaneDistributionPathEnv, filepath.Join(t.TempDir(), "missing-distribution.json"))

	backends, err := NewControlPlaneBackendsFromDistributionEnv()
	if err != nil {
		t.Fatalf("expected distribution env to prefer http backends when configured, got %v", err)
	}
	record, err := backends.Registry.ResolvePipelineRecord("pipeline-http-env")
	if err != nil {
		t.Fatalf("expected registry backend resolution from http source, got %v", err)
	}
	if record.GraphDefinitionRef != "graph/http-env" {
		t.Fatalf("unexpected registry record resolved from http source: %+v", record)
	}
}

func TestNewControlPlaneBackendsFromDistributionEnvUsesHTTPOrderedFailoverURLs(t *testing.T) {
	degraded := newDistributionHTTPFixtureServer(t, http.StatusServiceUnavailable, `{"error":"unavailable"}`)
	defer degraded.Close()

	healthy := newDistributionHTTPFixtureServer(t, http.StatusOK, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "records": {
      "pipeline-http-failover": {
        "pipeline_version": "pipeline-http-failover",
        "graph_definition_ref": "graph/http-failover",
        "execution_profile": "simple"
      }
    }
  }
}`)
	defer healthy.Close()

	t.Setenv(ControlPlaneDistributionHTTPURLsEnv, degraded.URL+","+healthy.URL)
	t.Setenv(distribution.EnvHTTPAdapterRetryMaxAttempts, "1")
	t.Setenv(ControlPlaneDistributionPathEnv, filepath.Join(t.TempDir(), "missing-distribution.json"))

	backends, err := NewControlPlaneBackendsFromDistributionEnv()
	if err != nil {
		t.Fatalf("expected ordered HTTP failover chain to load, got %v", err)
	}
	record, err := backends.Registry.ResolvePipelineRecord("pipeline-http-failover")
	if err != nil {
		t.Fatalf("expected registry resolution from failover chain, got %v", err)
	}
	if record.GraphDefinitionRef != "graph/http-failover" {
		t.Fatalf("unexpected registry record from failover chain: %+v", record)
	}
}

func TestNewControlPlaneBackendsFromDistributionEnvAllHTTPEndpointsStale(t *testing.T) {
	stalePayload := `{"schema_version":"cp-snapshot-distribution/v1","stale":true}`
	staleA := newDistributionHTTPFixtureServer(t, http.StatusOK, stalePayload)
	defer staleA.Close()
	staleB := newDistributionHTTPFixtureServer(t, http.StatusOK, stalePayload)
	defer staleB.Close()

	t.Setenv(ControlPlaneDistributionHTTPURLsEnv, staleA.URL+","+staleB.URL)
	t.Setenv(distribution.EnvHTTPAdapterRetryMaxAttempts, "1")

	_, err := NewControlPlaneBackendsFromDistributionEnv()
	if err == nil {
		t.Fatalf("expected stale HTTP endpoint chain to fail")
	}
	var backendErr distribution.BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected wrapped distribution backend error, got %T", err)
	}
	if backendErr.Code != distribution.ErrorCodeSnapshotStale {
		t.Fatalf("expected stale snapshot classification, got %s", backendErr.Code)
	}
}

type staleSnapshotStubError struct {
	reason string
}

func (e staleSnapshotStubError) Error() string {
	if e.reason == "" {
		return "snapshot stale"
	}
	return e.reason
}

func (e staleSnapshotStubError) StaleSnapshot() bool {
	return true
}

func writeDistributionFixture(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cp-distribution.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write distribution fixture: %v", err)
	}
	return path
}

func newDistributionHTTPFixtureServer(t *testing.T, statusCode int, payload string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
}

type stubGraphCompilerBackend struct {
	compileFn func(in graphcompiler.Input) (graphcompiler.Output, error)
}

func (s stubGraphCompilerBackend) Compile(in graphcompiler.Input) (graphcompiler.Output, error) {
	return s.compileFn(in)
}

type stubAdmissionBackend struct {
	evalFn func(in admission.Input) (admission.Output, error)
}

func (s stubAdmissionBackend) Evaluate(in admission.Input) (admission.Output, error) {
	return s.evalFn(in)
}

type stubLeaseBackend struct {
	resolveFn func(in lease.Input) (lease.Output, error)
}

func (s stubLeaseBackend) Resolve(in lease.Input) (lease.Output, error) {
	return s.resolveFn(in)
}

func boolPtr(v bool) *bool {
	out := v
	return &out
}
