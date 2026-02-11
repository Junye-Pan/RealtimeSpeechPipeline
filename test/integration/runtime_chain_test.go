package integration_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	cpadmission "github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	cpgraphcompiler "github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	cplease "github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	cppolicy "github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	cpproviderhealth "github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	cpregistry "github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	cprollout "github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	cproutingview "github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/executor"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/prelude"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestGuardPlanResolverTurnArbiterChain(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-1",
		TurnID:               "turn-integration-1",
		EventID:              "evt-open-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       11,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnActive {
		t.Fatalf("expected turn to become Active, got %s", open.State)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-1",
		TurnID:               "turn-integration-1",
		EventID:              "evt-active-1",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   150,
		WallClockTimestampMS: 150,
		AuthorityEpoch:       11,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("active path failed: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected turn to close, got %s", active.State)
	}
	if len(active.Transitions) != 2 {
		t.Fatalf("expected terminal transitions, got %d", len(active.Transitions))
	}
}

func TestCPBundleProvenancePropagatesThroughOpenAndTerminalBaseline(t *testing.T) {
	t.Parallel()

	customSnapshot := controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/integration-custom",
		AdmissionPolicySnapshot:   "admission-policy/integration-custom",
		ABICompatibilitySnapshot:  "abi-compat/integration-custom",
		VersionResolutionSnapshot: "version-resolution/integration-custom",
		PolicyResolutionSnapshot:  "policy-resolution/integration-custom",
		ProviderHealthSnapshot:    "provider-health/integration-custom",
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16, AttemptCapacity: 16})
	arbiter := turnarbiter.NewWithDependencies(&recorder, integrationTurnStartBundleResolver{
		bundle: turnarbiter.TurnStartBundle{
			PipelineVersion:        "pipeline-integration-custom",
			GraphDefinitionRef:     "graph/integration-custom",
			ExecutionProfile:       "simple",
			AllowedAdaptiveActions: []string{"retry", "fallback"},
			SnapshotProvenance:     customSnapshot,
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-bundle-1",
		TurnID:               "turn-integration-bundle-1",
		EventID:              "evt-open-bundle-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-request-ignored",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.Plan == nil {
		t.Fatalf("expected resolved plan")
	}
	if open.Plan.PipelineVersion != "pipeline-integration-custom" || open.Plan.GraphDefinitionRef != "graph/integration-custom" {
		t.Fatalf("expected custom pipeline/graph from CP bundle, got %+v", open.Plan)
	}
	if !reflect.DeepEqual(open.Plan.SnapshotProvenance, customSnapshot) {
		t.Fatalf("expected custom snapshot provenance in resolved plan, got %+v", open.Plan.SnapshotProvenance)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-bundle-1",
		TurnID:               "turn-integration-bundle-1",
		EventID:              "evt-active-bundle-1",
		RuntimeTimestampMS:   130,
		WallClockTimestampMS: 130,
		AuthorityEpoch:       5,
		RuntimeSequence:      7,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("active path failed: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected closed state after terminal success, got %s", active.State)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one baseline entry, got %d", len(entries))
	}
	if entries[0].PipelineVersion != "pipeline-integration-custom" {
		t.Fatalf("expected baseline pipeline version from custom bundle, got %+v", entries[0])
	}
	if !reflect.DeepEqual(entries[0].SnapshotProvenance, customSnapshot) {
		t.Fatalf("expected baseline snapshot provenance from custom bundle, got %+v", entries[0].SnapshotProvenance)
	}
}

func TestCPBackendBundleProvenancePropagatesThroughOpenAndTerminalBaseline(t *testing.T) {
	t.Parallel()

	customSnapshot := controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/integration-backend",
		AdmissionPolicySnapshot:   "admission-policy/integration-backend",
		ABICompatibilitySnapshot:  "abi-compat/integration-backend",
		VersionResolutionSnapshot: "version-resolution/integration-backend",
		PolicyResolutionSnapshot:  "policy-resolution/integration-backend",
		ProviderHealthSnapshot:    "provider-health/integration-backend",
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16, AttemptCapacity: 16})
	arbiter := turnarbiter.NewWithControlPlaneBackends(&recorder, turnarbiter.ControlPlaneBackends{
		Registry: integrationRegistryBackend{
			resolveFn: func(version string) (cpregistry.PipelineRecord, error) {
				return cpregistry.PipelineRecord{
					PipelineVersion:    version,
					GraphDefinitionRef: "graph/integration-backend",
					ExecutionProfile:   "simple",
				}, nil
			},
		},
		Rollout: integrationRolloutBackend{
			resolveFn: func(cprollout.ResolveVersionInput) (cprollout.ResolveVersionOutput, error) {
				return cprollout.ResolveVersionOutput{
					PipelineVersion:           "pipeline-integration-backend",
					VersionResolutionSnapshot: customSnapshot.VersionResolutionSnapshot,
				}, nil
			},
		},
		RoutingView: integrationRoutingBackend{
			getFn: func(cproutingview.Input) (cproutingview.Snapshot, error) {
				return cproutingview.Snapshot{
					RoutingViewSnapshot:      customSnapshot.RoutingViewSnapshot,
					AdmissionPolicySnapshot:  customSnapshot.AdmissionPolicySnapshot,
					ABICompatibilitySnapshot: customSnapshot.ABICompatibilitySnapshot,
				}, nil
			},
		},
		Policy: integrationPolicyBackend{
			evalFn: func(cppolicy.Input) (cppolicy.Output, error) {
				return cppolicy.Output{
					PolicyResolutionSnapshot: customSnapshot.PolicyResolutionSnapshot,
					AllowedAdaptiveActions:   []string{"fallback", "retry"},
				}, nil
			},
		},
		ProviderHealth: integrationProviderHealthBackend{
			getFn: func(cpproviderhealth.Input) (cpproviderhealth.Output, error) {
				return cpproviderhealth.Output{ProviderHealthSnapshot: customSnapshot.ProviderHealthSnapshot}, nil
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-backend-bundle-1",
		TurnID:               "turn-integration-backend-bundle-1",
		EventID:              "evt-open-backend-bundle-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested-ignored",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.Plan == nil {
		t.Fatalf("expected resolved plan")
	}
	if open.Plan.PipelineVersion != "pipeline-integration-backend" || open.Plan.GraphDefinitionRef != "graph/integration-backend" {
		t.Fatalf("expected backend pipeline/graph in resolved plan, got %+v", open.Plan)
	}
	if !reflect.DeepEqual(open.Plan.SnapshotProvenance, customSnapshot) {
		t.Fatalf("expected backend snapshot provenance in resolved plan, got %+v", open.Plan.SnapshotProvenance)
	}
	if !reflect.DeepEqual(open.Plan.AllowedAdaptiveActions, []string{"retry", "fallback"}) {
		t.Fatalf("expected normalized backend adaptive actions in plan, got %+v", open.Plan.AllowedAdaptiveActions)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-backend-bundle-1",
		TurnID:               "turn-integration-backend-bundle-1",
		EventID:              "evt-active-backend-bundle-1",
		RuntimeTimestampMS:   140,
		WallClockTimestampMS: 140,
		AuthorityEpoch:       5,
		RuntimeSequence:      9,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("active path failed: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected closed state after terminal success, got %s", active.State)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one baseline entry, got %d", len(entries))
	}
	if entries[0].PipelineVersion != "pipeline-integration-backend" {
		t.Fatalf("expected backend pipeline version in baseline, got %+v", entries[0])
	}
	if !reflect.DeepEqual(entries[0].SnapshotProvenance, customSnapshot) {
		t.Fatalf("expected backend snapshot provenance in baseline, got %+v", entries[0].SnapshotProvenance)
	}
}

func TestCPHTTPDistributionBackendsPropagateThroughOpenAndTerminalBaseline(t *testing.T) {
	t.Parallel()

	server := newIntegrationDistributionHTTPServer(t, http.StatusOK, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "default_pipeline_version": "pipeline-distribution-http-integration",
    "records": {
      "pipeline-requested": {
        "pipeline_version": "pipeline-requested",
        "graph_definition_ref": "graph/distribution-http-integration",
        "execution_profile": "simple"
      },
      "pipeline-distribution-http-integration": {
        "pipeline_version": "pipeline-distribution-http-integration",
        "graph_definition_ref": "graph/distribution-http-integration",
        "execution_profile": "simple"
      }
    }
  },
  "rollout": {
    "default_pipeline_version": "pipeline-distribution-http-integration",
    "by_requested_version": {
      "pipeline-requested": "pipeline-distribution-http-integration"
    },
    "version_resolution_snapshot": "version-resolution/distribution-http-integration"
  },
  "routing_view": {
    "default": {
      "routing_view_snapshot": "routing-view/distribution-http-integration",
      "admission_policy_snapshot": "admission-policy/distribution-http-integration",
      "abi_compatibility_snapshot": "abi-compat/distribution-http-integration"
    }
  },
  "policy": {
    "default": {
      "policy_resolution_snapshot": "policy-resolution/distribution-http-integration",
      "allowed_adaptive_actions": ["fallback", "retry"]
    }
  },
  "provider_health": {
    "default": {
      "provider_health_snapshot": "provider-health/distribution-http-integration"
    }
  }
}`)
	defer server.Close()

	expectedSnapshot := controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/distribution-http-integration",
		AdmissionPolicySnapshot:   "admission-policy/distribution-http-integration",
		ABICompatibilitySnapshot:  "abi-compat/distribution-http-integration",
		VersionResolutionSnapshot: "version-resolution/distribution-http-integration",
		PolicyResolutionSnapshot:  "policy-resolution/distribution-http-integration",
		ProviderHealthSnapshot:    "provider-health/distribution-http-integration",
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16, AttemptCapacity: 16})
	arbiter, err := turnarbiter.NewWithControlPlaneBackendsFromDistributionHTTP(&recorder, server.URL)
	if err != nil {
		t.Fatalf("expected http distribution-backed arbiter, got %v", err)
	}

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-http-distribution-1",
		TurnID:               "turn-integration-http-distribution-1",
		EventID:              "evt-open-http-distribution-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.Plan == nil {
		t.Fatalf("expected resolved plan")
	}
	if open.Plan.PipelineVersion != "pipeline-distribution-http-integration" || open.Plan.GraphDefinitionRef != "graph/distribution-http-integration" {
		t.Fatalf("expected http distribution pipeline/graph in resolved plan, got %+v", open.Plan)
	}
	if !reflect.DeepEqual(open.Plan.SnapshotProvenance, expectedSnapshot) {
		t.Fatalf("expected http distribution snapshot provenance in resolved plan, got %+v", open.Plan.SnapshotProvenance)
	}
	if !reflect.DeepEqual(open.Plan.AllowedAdaptiveActions, []string{"retry", "fallback"}) {
		t.Fatalf("expected normalized policy actions in plan, got %+v", open.Plan.AllowedAdaptiveActions)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-http-distribution-1",
		TurnID:               "turn-integration-http-distribution-1",
		EventID:              "evt-active-http-distribution-1",
		RuntimeTimestampMS:   140,
		WallClockTimestampMS: 140,
		AuthorityEpoch:       5,
		RuntimeSequence:      9,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("active path failed: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected closed state after terminal success, got %s", active.State)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one baseline entry, got %d", len(entries))
	}
	if entries[0].PipelineVersion != "pipeline-distribution-http-integration" {
		t.Fatalf("expected backend pipeline version in baseline, got %+v", entries[0])
	}
	if !reflect.DeepEqual(entries[0].SnapshotProvenance, expectedSnapshot) {
		t.Fatalf("expected backend snapshot provenance in baseline, got %+v", entries[0].SnapshotProvenance)
	}
}

func TestCPBackendPartialAvailabilityFallsBackDeterministically(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16, AttemptCapacity: 16})
	arbiter := turnarbiter.NewWithControlPlaneBackends(&recorder, turnarbiter.ControlPlaneBackends{
		Registry: integrationRegistryBackend{
			resolveFn: func(version string) (cpregistry.PipelineRecord, error) {
				return cpregistry.PipelineRecord{
					PipelineVersion:    version,
					GraphDefinitionRef: "graph/integration-partial",
					ExecutionProfile:   "simple",
				}, nil
			},
		},
		Rollout: integrationRolloutBackend{
			resolveFn: func(cprollout.ResolveVersionInput) (cprollout.ResolveVersionOutput, error) {
				return cprollout.ResolveVersionOutput{
					PipelineVersion:           "pipeline-integration-partial",
					VersionResolutionSnapshot: "version-resolution/integration-partial",
				}, nil
			},
		},
		RoutingView: integrationRoutingBackend{
			getFn: func(cproutingview.Input) (cproutingview.Snapshot, error) {
				return cproutingview.Snapshot{}, errors.New("routing backend unavailable")
			},
		},
		Policy: integrationPolicyBackend{
			evalFn: func(cppolicy.Input) (cppolicy.Output, error) {
				return cppolicy.Output{
					PolicyResolutionSnapshot: "policy-resolution/integration-partial",
					AllowedAdaptiveActions:   []string{"fallback", "retry"},
				}, nil
			},
		},
		ProviderHealth: integrationProviderHealthBackend{
			getFn: func(cpproviderhealth.Input) (cpproviderhealth.Output, error) {
				return cpproviderhealth.Output{ProviderHealthSnapshot: "provider-health/integration-partial"}, nil
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-partial-backend-1",
		TurnID:               "turn-integration-partial-backend-1",
		EventID:              "evt-open-partial-backend-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnActive || open.Plan == nil {
		t.Fatalf("expected active turn with resolved plan under partial backend availability, got %+v", open)
	}
	if open.Plan.PipelineVersion != "pipeline-integration-partial" || open.Plan.GraphDefinitionRef != "graph/integration-partial" {
		t.Fatalf("expected rollout/registry backend values in plan, got %+v", open.Plan)
	}
	if !reflect.DeepEqual(open.Plan.AllowedAdaptiveActions, []string{"retry", "fallback"}) {
		t.Fatalf("expected normalized policy actions under partial availability, got %+v", open.Plan.AllowedAdaptiveActions)
	}

	expectedProvenance := controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/v1",
		AdmissionPolicySnapshot:   "admission-policy/v1",
		ABICompatibilitySnapshot:  "abi-compat/v1",
		VersionResolutionSnapshot: "version-resolution/integration-partial",
		PolicyResolutionSnapshot:  "policy-resolution/integration-partial",
		ProviderHealthSnapshot:    "provider-health/integration-partial",
	}
	if !reflect.DeepEqual(open.Plan.SnapshotProvenance, expectedProvenance) {
		t.Fatalf("expected routing fallback defaults with other backend snapshots preserved, got %+v", open.Plan.SnapshotProvenance)
	}
}

func TestCPBackendRolloutPolicyProviderHealthFailuresFallBackDeterministically(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16, AttemptCapacity: 16})
	arbiter := turnarbiter.NewWithControlPlaneBackends(&recorder, turnarbiter.ControlPlaneBackends{
		Registry: integrationRegistryBackend{
			resolveFn: func(string) (cpregistry.PipelineRecord, error) {
				return cpregistry.PipelineRecord{
					PipelineVersion:    "pipeline-integration-rollout-fallback",
					GraphDefinitionRef: "graph/integration-rollout-fallback",
					ExecutionProfile:   "simple",
				}, nil
			},
		},
		Rollout: integrationRolloutBackend{
			resolveFn: func(cprollout.ResolveVersionInput) (cprollout.ResolveVersionOutput, error) {
				return cprollout.ResolveVersionOutput{}, errors.New("rollout backend unavailable")
			},
		},
		RoutingView: integrationRoutingBackend{
			getFn: func(cproutingview.Input) (cproutingview.Snapshot, error) {
				return cproutingview.Snapshot{
					RoutingViewSnapshot:      "routing-view/integration-rollout-fallback",
					AdmissionPolicySnapshot:  "admission-policy/integration-rollout-fallback",
					ABICompatibilitySnapshot: "abi-compat/integration-rollout-fallback",
				}, nil
			},
		},
		Policy: integrationPolicyBackend{
			evalFn: func(cppolicy.Input) (cppolicy.Output, error) {
				return cppolicy.Output{}, errors.New("policy backend unavailable")
			},
		},
		ProviderHealth: integrationProviderHealthBackend{
			getFn: func(cpproviderhealth.Input) (cpproviderhealth.Output, error) {
				return cpproviderhealth.Output{}, errors.New("provider health backend unavailable")
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-rollout-fallback-1",
		TurnID:               "turn-integration-rollout-fallback-1",
		EventID:              "evt-open-rollout-fallback-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnActive || open.Plan == nil {
		t.Fatalf("expected active turn with resolved plan under rollout/policy/provider-health failure, got %+v", open)
	}
	if open.Plan.PipelineVersion != "pipeline-integration-rollout-fallback" || open.Plan.GraphDefinitionRef != "graph/integration-rollout-fallback" {
		t.Fatalf("expected registry pipeline/graph values under rollout failure fallback, got %+v", open.Plan)
	}
	if !reflect.DeepEqual(open.Plan.AllowedAdaptiveActions, []string{"retry", "provider_switch", "fallback"}) {
		t.Fatalf("expected policy fallback adaptive actions under backend failure, got %+v", open.Plan.AllowedAdaptiveActions)
	}

	expectedProvenance := controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/integration-rollout-fallback",
		AdmissionPolicySnapshot:   "admission-policy/integration-rollout-fallback",
		ABICompatibilitySnapshot:  "abi-compat/integration-rollout-fallback",
		VersionResolutionSnapshot: "version-resolution/v1",
		PolicyResolutionSnapshot:  "policy-resolution/v1",
		ProviderHealthSnapshot:    "provider-health/v1",
	}
	if !reflect.DeepEqual(open.Plan.SnapshotProvenance, expectedProvenance) {
		t.Fatalf("expected rollout/policy/provider-health fallback snapshots, got %+v", open.Plan.SnapshotProvenance)
	}
}

func TestCPBackendGraphCompilerOutputPropagatesIntoResolvedPlan(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.NewWithControlPlaneBackends(nil, turnarbiter.ControlPlaneBackends{
		GraphCompiler: integrationGraphCompilerBackend{
			compileFn: func(cpgraphcompiler.Input) (cpgraphcompiler.Output, error) {
				return cpgraphcompiler.Output{
					GraphDefinitionRef:       "graph/compiled-integration",
					GraphCompilationSnapshot: "graph-compiler/integration",
					GraphFingerprint:         "graph-fingerprint/integration",
				}, nil
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-graph-compiler-1",
		TurnID:               "turn-integration-graph-compiler-1",
		EventID:              "evt-open-graph-compiler-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnActive || open.Plan == nil {
		t.Fatalf("expected active state with resolved plan, got %+v", open)
	}
	if open.Plan.GraphDefinitionRef != "graph/compiled-integration" {
		t.Fatalf("expected CP-03 compiled graph definition in plan, got %+v", open.Plan)
	}
}

func TestCPBackendAdmissionRejectTriggersDeterministicPreTurnDecision(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.NewWithControlPlaneBackends(nil, turnarbiter.ControlPlaneBackends{
		Admission: integrationAdmissionBackend{
			evalFn: func(cpadmission.Input) (cpadmission.Output, error) {
				return cpadmission.Output{
					AdmissionPolicySnapshot: "admission-policy/integration",
					OutcomeKind:             controlplane.OutcomeReject,
					Scope:                   controlplane.ScopeSession,
					Reason:                  cpadmission.ReasonRejectPolicy,
				}, nil
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-admission-1",
		TurnID:               "turn-integration-admission-1",
		EventID:              "evt-open-admission-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnIdle || open.Decision == nil {
		t.Fatalf("expected deterministic pre-turn reject decision, got %+v", open)
	}
	if open.Decision.OutcomeKind != controlplane.OutcomeReject || open.Decision.EmittedBy != controlplane.EmitterCP05 || open.Decision.Reason != cpadmission.ReasonRejectPolicy {
		t.Fatalf("unexpected CP-05 decision outcome: %+v", open.Decision)
	}
}

func TestCPBackendLeaseDecisionTriggersAuthorityRejection(t *testing.T) {
	t.Parallel()

	authorized := false
	epochValid := true

	arbiter := turnarbiter.NewWithControlPlaneBackends(nil, turnarbiter.ControlPlaneBackends{
		Lease: integrationLeaseBackend{
			resolveFn: func(cplease.Input) (cplease.Output, error) {
				return cplease.Output{
					LeaseResolutionSnapshot: "lease-resolution/integration",
					AuthorityEpoch:          5,
					AuthorityEpochValid:     &epochValid,
					AuthorityAuthorized:     &authorized,
					Reason:                  cplease.ReasonLeaseDeauthorized,
				}, nil
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-lease-1",
		TurnID:               "turn-integration-lease-1",
		EventID:              "evt-open-lease-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnIdle || open.Decision == nil {
		t.Fatalf("expected deterministic pre-turn deauthorized decision, got %+v", open)
	}
	if open.Decision.OutcomeKind != controlplane.OutcomeDeauthorized || open.Decision.EmittedBy != controlplane.EmitterRK24 {
		t.Fatalf("unexpected CP-07 lease authority gate decision: %+v", open.Decision)
	}
}

func TestCPBackendStaleSnapshotFetchTriggersDeterministicPreTurnHandling(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.NewWithControlPlaneBackends(nil, turnarbiter.ControlPlaneBackends{
		Registry: integrationRegistryBackend{
			resolveFn: func(version string) (cpregistry.PipelineRecord, error) {
				return cpregistry.PipelineRecord{
					PipelineVersion:    version,
					GraphDefinitionRef: "graph/integration-stale",
					ExecutionProfile:   "simple",
				}, nil
			},
		},
		Rollout: integrationRolloutBackend{
			resolveFn: func(cprollout.ResolveVersionInput) (cprollout.ResolveVersionOutput, error) {
				return cprollout.ResolveVersionOutput{PipelineVersion: "pipeline-integration-stale"}, nil
			},
		},
		RoutingView: integrationRoutingBackend{
			getFn: func(cproutingview.Input) (cproutingview.Snapshot, error) {
				return cproutingview.Snapshot{}, integrationStaleSnapshotError{reason: "routing snapshot stale"}
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-stale-backend-1",
		TurnID:               "turn-integration-stale-backend-1",
		EventID:              "evt-open-stale-backend-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-requested",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnIdle || open.Decision == nil {
		t.Fatalf("expected deterministic pre-turn decision on stale snapshot fetch failure, got %+v", open)
	}
	if open.Decision.OutcomeKind != controlplane.OutcomeDefer || open.Decision.Reason != "turn_start_bundle_resolution_failed" {
		t.Fatalf("unexpected stale snapshot pre-turn decision: %+v", open.Decision)
	}
}

func TestCPBackendUnsupportedExecutionProfileTriggersDeterministicPreTurnHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		planFailurePolicy controlplane.OutcomeKind
		wantOutcome       controlplane.OutcomeKind
	}{
		{
			name:        "default defer",
			wantOutcome: controlplane.OutcomeDefer,
		},
		{
			name:              "explicit reject",
			planFailurePolicy: controlplane.OutcomeReject,
			wantOutcome:       controlplane.OutcomeReject,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			arbiter := turnarbiter.NewWithControlPlaneBackends(nil, turnarbiter.ControlPlaneBackends{
				Registry: integrationRegistryBackend{
					resolveFn: func(version string) (cpregistry.PipelineRecord, error) {
						return cpregistry.PipelineRecord{
							PipelineVersion:    version,
							GraphDefinitionRef: "graph/integration-invalid-profile",
							ExecutionProfile:   "advanced",
						}, nil
					},
				},
			})

			open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
				SessionID:            "sess-integration-invalid-profile-1",
				TurnID:               "turn-integration-invalid-profile-1",
				EventID:              "evt-open-invalid-profile-1",
				RuntimeTimestampMS:   100,
				WallClockTimestampMS: 100,
				PipelineVersion:      "pipeline-requested",
				AuthorityEpoch:       5,
				SnapshotValid:        true,
				AuthorityEpochValid:  true,
				AuthorityAuthorized:  true,
				PlanFailurePolicy:    tc.planFailurePolicy,
			})
			if err != nil {
				t.Fatalf("open path failed: %v", err)
			}
			if open.State != controlplane.TurnIdle || open.Decision == nil {
				t.Fatalf("expected deterministic pre-turn decision on unsupported execution profile, got %+v", open)
			}
			if open.Decision.OutcomeKind != tc.wantOutcome || open.Decision.Reason != "turn_start_bundle_resolution_failed" {
				t.Fatalf("unexpected unsupported-profile pre-turn decision: %+v", open.Decision)
			}
		})
	}
}

func TestCPBundleResolutionFailureRemainsDeterministicPreTurn(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.NewWithDependencies(nil, integrationTurnStartBundleResolver{err: errors.New("bundle unavailable")})
	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-bundle-fail-1",
		TurnID:               "turn-integration-bundle-fail-1",
		EventID:              "evt-open-bundle-fail-1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		AuthorityEpoch:       2,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
		PlanFailurePolicy:    controlplane.OutcomeReject,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnIdle || open.Decision == nil {
		t.Fatalf("expected deterministic pre-turn decision on bundle failure, got %+v", open)
	}
	if open.Decision.Reason != "turn_start_bundle_resolution_failed" || open.Decision.OutcomeKind != controlplane.OutcomeReject {
		t.Fatalf("unexpected bundle failure decision: %+v", open.Decision)
	}
}

func TestGuardRejectsStaleAuthorityBeforeOpen(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-2",
		TurnID:               "turn-integration-2",
		EventID:              "evt-open-2",
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       22,
		SnapshotValid:        true,
		AuthorityEpochValid:  false,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if open.State != controlplane.TurnIdle {
		t.Fatalf("expected pre-turn rejection to stay Idle, got %s", open.State)
	}
	if open.Decision == nil || open.Decision.OutcomeKind != controlplane.OutcomeStaleEpochReject {
		t.Fatalf("expected stale_epoch_reject outcome, got %+v", open.Decision)
	}
}

func TestSchedulingPointShedDoesNotForceTerminalLifecycle(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	scheduler := executor.NewScheduler(localadmission.Evaluator{})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-3",
		TurnID:               "turn-integration-3",
		EventID:              "evt-open-3",
		RuntimeTimestampMS:   300,
		WallClockTimestampMS: 300,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       33,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnActive {
		t.Fatalf("expected Active after open, got %s", open.State)
	}

	decision, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-integration-3",
		TurnID:               "turn-integration-3",
		EventID:              "evt-dispatch-3",
		RuntimeTimestampMS:   320,
		WallClockTimestampMS: 320,
		Shed:                 true,
	})
	if err != nil {
		t.Fatalf("scheduling decision failed: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected shed outcome at scheduling point")
	}
	if decision.Outcome == nil || decision.Outcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected shed decision outcome, got %+v", decision.Outcome)
	}
	if decision.ControlSignal == nil {
		t.Fatalf("expected shed control signal")
	}
	if decision.ControlSignal.Signal != "shed" || decision.ControlSignal.EmittedBy != "RK-25" {
		t.Fatalf("unexpected shed control signal: %+v", decision.ControlSignal)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-3",
		TurnID:               "turn-integration-3",
		EventID:              "evt-active-3",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   330,
		WallClockTimestampMS: 330,
		AuthorityEpoch:       33,
	})
	if err != nil {
		t.Fatalf("active lifecycle handling failed: %v", err)
	}
	if active.State != controlplane.TurnActive {
		t.Fatalf("scheduling shed alone must not terminalize turn, got %s", active.State)
	}
	if len(active.Transitions) != 0 {
		t.Fatalf("expected no lifecycle transitions from scheduling shed alone")
	}
}

func TestProviderInvocationEvidenceThreadedIntoTerminalBaseline(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16})
	arbiter := turnarbiter.NewWithRecorder(&recorder)
	scheduler := executor.NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invocation.NewController(catalog))

	decision, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-integration-provider-1",
		TurnID:               "turn-integration-provider-1",
		EventID:              "evt-dispatch-provider-1",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		ProviderInvocation: &executor.ProviderInvocationInput{
			Modality:          contracts.ModalitySTT,
			PreferredProvider: "stt-a",
		},
	})
	if err != nil {
		t.Fatalf("unexpected scheduling/provider error: %v", err)
	}
	if !decision.Allowed || decision.Provider == nil {
		t.Fatalf("expected allowed provider decision, got %+v", decision)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:                  "sess-integration-provider-1",
		TurnID:                     "turn-integration-provider-1",
		EventID:                    "evt-active-provider-1",
		PipelineVersion:            "pipeline-v1",
		RuntimeTimestampMS:         120,
		WallClockTimestampMS:       120,
		AuthorityEpoch:             3,
		RuntimeSequence:            2,
		TerminalSuccessReady:       true,
		ProviderInvocationOutcomes: []timeline.InvocationOutcomeEvidence{decision.Provider.ToInvocationOutcomeEvidence()},
	})
	if err != nil {
		t.Fatalf("unexpected active handling error: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected terminal close state, got %s", active.State)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 baseline entry, got %d", len(entries))
	}
	if len(entries[0].InvocationOutcomes) != 1 {
		t.Fatalf("expected 1 invocation outcome evidence, got %d", len(entries[0].InvocationOutcomes))
	}
	if entries[0].InvocationOutcomes[0].OutcomeClass != "success" {
		t.Fatalf("expected success invocation outcome class, got %+v", entries[0].InvocationOutcomes[0])
	}
}

func TestExecutePlanPersistsAttemptEvidenceAndTerminalBaseline(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:       contracts.OutcomeOverload,
					Retryable:   false,
					CircuitOpen: true,
					Reason:      "provider_overload",
				}, nil
			},
		},
		contracts.StaticAdapter{
			ID:   "stt-b",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16, AttemptCapacity: 16})
	arbiter := turnarbiter.NewWithRecorder(&recorder)
	scheduler := executor.NewSchedulerWithProviderInvokerAndAttemptAppender(localadmission.Evaluator{}, invocation.NewController(catalog), &recorder)

	trace, err := scheduler.ExecutePlan(
		executor.SchedulingInput{
			SessionID:            "sess-integration-provider-plan-1",
			TurnID:               "turn-integration-provider-plan-1",
			EventID:              "evt-dispatch-provider-plan-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    10,
			RuntimeSequence:      11,
			AuthorityEpoch:       3,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		executor.ExecutionPlan{
			Nodes: []executor.NodeSpec{
				{
					NodeID:   "provider-stt",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
					Provider: &executor.ProviderInvocationInput{
						Modality:               contracts.ModalitySTT,
						PreferredProvider:      "stt-a",
						AllowedAdaptiveActions: []string{"provider_switch"},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected completed execution trace, got %+v", trace)
	}
	if len(trace.Nodes) != 1 || trace.Nodes[0].Decision.Provider == nil {
		t.Fatalf("expected one provider node decision, got %+v", trace.Nodes)
	}

	attempts := recorder.ProviderAttemptEntries()
	if len(attempts) != 2 {
		t.Fatalf("expected 2 provider attempt entries, got %d", len(attempts))
	}
	if attempts[0].ProviderID != "stt-a" || attempts[1].ProviderID != "stt-b" {
		t.Fatalf("unexpected provider attempt ordering: %+v", attempts)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:                  "sess-integration-provider-plan-1",
		TurnID:                     "turn-integration-provider-plan-1",
		EventID:                    "evt-active-provider-plan-1",
		PipelineVersion:            "pipeline-v1",
		RuntimeTimestampMS:         120,
		WallClockTimestampMS:       120,
		AuthorityEpoch:             3,
		RuntimeSequence:            2,
		TerminalSuccessReady:       true,
		ProviderInvocationOutcomes: []timeline.InvocationOutcomeEvidence{trace.Nodes[0].Decision.Provider.ToInvocationOutcomeEvidence()},
	})
	if err != nil {
		t.Fatalf("unexpected active handling error: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected terminal close state, got %s", active.State)
	}
}

func TestExecutePlanPromotesAttemptEvidenceIntoTerminalBaseline(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{
					Class:       contracts.OutcomeOverload,
					Retryable:   false,
					CircuitOpen: true,
					Reason:      "provider_overload",
				}, nil
			},
		},
		contracts.StaticAdapter{
			ID:   "stt-b",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16, AttemptCapacity: 16})
	arbiter := turnarbiter.NewWithRecorder(&recorder)
	scheduler := executor.NewSchedulerWithProviderInvokerAndAttemptAppender(localadmission.Evaluator{}, invocation.NewController(catalog), &recorder)

	trace, err := scheduler.ExecutePlan(
		executor.SchedulingInput{
			SessionID:            "sess-integration-provider-promote-1",
			TurnID:               "turn-integration-provider-promote-1",
			EventID:              "evt-dispatch-provider-promote-1",
			PipelineVersion:      "pipeline-v1",
			TransportSequence:    10,
			RuntimeSequence:      11,
			AuthorityEpoch:       3,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		executor.ExecutionPlan{
			Nodes: []executor.NodeSpec{
				{
					NodeID:   "provider-stt",
					NodeType: "provider",
					Lane:     eventabi.LaneData,
					Provider: &executor.ProviderInvocationInput{
						Modality:               contracts.ModalitySTT,
						PreferredProvider:      "stt-a",
						AllowedAdaptiveActions: []string{"provider_switch"},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected execute plan error: %v", err)
	}
	if !trace.Completed {
		t.Fatalf("expected completed execution trace, got %+v", trace)
	}
	if len(recorder.ProviderAttemptEntries()) != 2 {
		t.Fatalf("expected 2 persisted attempts, got %d", len(recorder.ProviderAttemptEntries()))
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-provider-promote-1",
		TurnID:               "turn-integration-provider-promote-1",
		EventID:              "evt-active-provider-promote-1",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   120,
		WallClockTimestampMS: 120,
		AuthorityEpoch:       3,
		RuntimeSequence:      2,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("unexpected active handling error: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected terminal close state, got %s", active.State)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 baseline entry, got %d", len(entries))
	}
	if len(entries[0].InvocationOutcomes) != 1 {
		t.Fatalf("expected synthesized invocation outcome in baseline, got %+v", entries[0].InvocationOutcomes)
	}
	outcome := entries[0].InvocationOutcomes[0]
	if outcome.ProviderID != "stt-b" || outcome.OutcomeClass != "success" || outcome.AttemptCount != 2 {
		t.Fatalf("unexpected promoted invocation outcome: %+v", outcome)
	}
	if outcome.FinalAttemptLatencyMS != 1 || outcome.TotalInvocationLatencyMS != 1 {
		t.Fatalf("expected promoted latency fields to equal 1ms, got %+v", outcome)
	}
}

func TestCancellationFenceRejectsLateOutputDeterministically(t *testing.T) {
	t.Parallel()

	fence := transport.NewOutputFence()
	beforeCancel, err := fence.EvaluateOutput(transport.OutputAttempt{
		SessionID:            "sess-int-cancel-1",
		TurnID:               "turn-int-cancel-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-int-cancel-before",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected pre-cancel output error: %v", err)
	}
	if !beforeCancel.Accepted || beforeCancel.Signal.Signal != "output_accepted" {
		t.Fatalf("expected output_accepted before cancel, got %+v", beforeCancel)
	}

	cancelAccepted, err := fence.EvaluateOutput(transport.OutputAttempt{
		SessionID:            "sess-int-cancel-1",
		TurnID:               "turn-int-cancel-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-int-cancel-accept",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected cancel acceptance error: %v", err)
	}
	if cancelAccepted.Accepted || cancelAccepted.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected cancel fence output rejection, got %+v", cancelAccepted)
	}

	lateOutput, err := fence.EvaluateOutput(transport.OutputAttempt{
		SessionID:            "sess-int-cancel-1",
		TurnID:               "turn-int-cancel-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-int-cancel-late",
		TransportSequence:    3,
		RuntimeSequence:      3,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   102,
		WallClockTimestampMS: 102,
	})
	if err != nil {
		t.Fatalf("unexpected late output error: %v", err)
	}
	if lateOutput.Accepted || lateOutput.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected deterministic post-cancel rejection, got %+v", lateOutput)
	}
}

func TestPreludeProposalFeedsTurnOpenPath(t *testing.T) {
	t.Parallel()

	preludeEngine := prelude.NewEngine()
	proposal, err := preludeEngine.ProposeTurn(prelude.Input{
		SessionID:            "sess-prelude-chain-1",
		TurnID:               "turn-prelude-chain-1",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected prelude proposal error: %v", err)
	}
	if proposal.Signal.Signal != "turn_open_proposed" {
		t.Fatalf("expected turn_open_proposed signal, got %+v", proposal.Signal)
	}

	arbiter := turnarbiter.New()
	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            proposal.Signal.SessionID,
		TurnID:               proposal.Signal.TurnID,
		EventID:              proposal.Signal.EventID,
		RuntimeTimestampMS:   proposal.Signal.RuntimeTimestampMS,
		WallClockTimestampMS: proposal.Signal.WallClockMS,
		PipelineVersion:      proposal.Signal.PipelineVersion,
		AuthorityEpoch:       proposal.Signal.AuthorityEpoch,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected arbiter open error: %v", err)
	}
	if open.State != controlplane.TurnActive {
		t.Fatalf("expected active turn after prelude proposal, got %s", open.State)
	}
}

type integrationTurnStartBundleResolver struct {
	bundle turnarbiter.TurnStartBundle
	err    error
}

func (s integrationTurnStartBundleResolver) ResolveTurnStartBundle(_ turnarbiter.TurnStartBundleInput) (turnarbiter.TurnStartBundle, error) {
	if s.err != nil {
		return turnarbiter.TurnStartBundle{}, s.err
	}
	return s.bundle, nil
}

type integrationRegistryBackend struct {
	resolveFn func(version string) (cpregistry.PipelineRecord, error)
}

func (s integrationRegistryBackend) ResolvePipelineRecord(version string) (cpregistry.PipelineRecord, error) {
	return s.resolveFn(version)
}

type integrationRolloutBackend struct {
	resolveFn func(in cprollout.ResolveVersionInput) (cprollout.ResolveVersionOutput, error)
}

func (s integrationRolloutBackend) ResolvePipelineVersion(in cprollout.ResolveVersionInput) (cprollout.ResolveVersionOutput, error) {
	return s.resolveFn(in)
}

type integrationRoutingBackend struct {
	getFn func(in cproutingview.Input) (cproutingview.Snapshot, error)
}

func (s integrationRoutingBackend) GetSnapshot(in cproutingview.Input) (cproutingview.Snapshot, error) {
	return s.getFn(in)
}

type integrationPolicyBackend struct {
	evalFn func(in cppolicy.Input) (cppolicy.Output, error)
}

func (s integrationPolicyBackend) Evaluate(in cppolicy.Input) (cppolicy.Output, error) {
	return s.evalFn(in)
}

type integrationProviderHealthBackend struct {
	getFn func(in cpproviderhealth.Input) (cpproviderhealth.Output, error)
}

func (s integrationProviderHealthBackend) GetSnapshot(in cpproviderhealth.Input) (cpproviderhealth.Output, error) {
	return s.getFn(in)
}

type integrationGraphCompilerBackend struct {
	compileFn func(in cpgraphcompiler.Input) (cpgraphcompiler.Output, error)
}

func (s integrationGraphCompilerBackend) Compile(in cpgraphcompiler.Input) (cpgraphcompiler.Output, error) {
	return s.compileFn(in)
}

type integrationAdmissionBackend struct {
	evalFn func(in cpadmission.Input) (cpadmission.Output, error)
}

func (s integrationAdmissionBackend) Evaluate(in cpadmission.Input) (cpadmission.Output, error) {
	return s.evalFn(in)
}

type integrationLeaseBackend struct {
	resolveFn func(in cplease.Input) (cplease.Output, error)
}

func (s integrationLeaseBackend) Resolve(in cplease.Input) (cplease.Output, error) {
	return s.resolveFn(in)
}

type integrationStaleSnapshotError struct {
	reason string
}

func (e integrationStaleSnapshotError) Error() string {
	if e.reason == "" {
		return "snapshot stale"
	}
	return e.reason
}

func (e integrationStaleSnapshotError) StaleSnapshot() bool {
	return true
}

func newIntegrationDistributionHTTPServer(t *testing.T, statusCode int, payload string) *httptest.Server {
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
