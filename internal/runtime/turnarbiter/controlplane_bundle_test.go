package turnarbiter

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/normalizer"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

func TestResolveTurnStartBundleDefaults(t *testing.T) {
	t.Parallel()

	resolver := newControlPlaneBundleResolver()
	bundle, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                "sess-1",
		TurnID:                   "turn-1",
		RequestedPipelineVersion: "pipeline-custom",
		AuthorityEpoch:           3,
	})
	if err != nil {
		t.Fatalf("unexpected resolve bundle error: %v", err)
	}
	if bundle.PipelineVersion != "pipeline-custom" {
		t.Fatalf("expected requested pipeline version to be resolved, got %+v", bundle)
	}
	if bundle.GraphDefinitionRef == "" || bundle.ExecutionProfile == "" {
		t.Fatalf("expected graph/profile defaults in bundle, got %+v", bundle)
	}
	if len(bundle.AllowedAdaptiveActions) == 0 {
		t.Fatalf("expected allowed adaptive actions, got %+v", bundle)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("expected valid bundle, got %v", err)
	}
	if bundle.LeaseTokenID == "" || bundle.LeaseTokenExpiresAtUTC == "" {
		t.Fatalf("expected lease token metadata in default bundle, got %+v", bundle)
	}
}

func TestResolveTurnStartBundleRequiresSessionID(t *testing.T) {
	t.Parallel()

	resolver := newControlPlaneBundleResolver()
	if _, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{}); err == nil {
		t.Fatalf("expected session_id validation error")
	}
}

func TestDefaultSnapshotProvenanceHasAllRefs(t *testing.T) {
	t.Parallel()

	snapshot := defaultSnapshotProvenance()
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("expected default snapshot provenance to validate: %v", err)
	}

	expected := defaultSnapshotProvenance()
	if !reflect.DeepEqual(snapshot, expected) {
		t.Fatalf("expected deterministic snapshot provenance defaults")
	}
}

func TestResolveTurnStartBundleUsesInjectedServices(t *testing.T) {
	t.Parallel()

	var capturedRolloutInput rollout.ResolveVersionInput

	registryService := registry.NewService()
	registryService.Backend = stubRegistryBackend{
		resolveFn: func(version string) (registry.PipelineRecord, error) {
			return registry.PipelineRecord{
				PipelineVersion:    version,
				GraphDefinitionRef: "graph/backend",
				ExecutionProfile:   "simple",
			}, nil
		},
	}

	rolloutService := rollout.NewService()
	rolloutService.Backend = stubRolloutBackend{
		resolveFn: func(in rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
			capturedRolloutInput = in
			return rollout.ResolveVersionOutput{
				PipelineVersion:           "pipeline-rollout-backend",
				VersionResolutionSnapshot: "version-resolution/backend",
			}, nil
		},
	}

	routingService := routingview.NewService()
	routingService.Backend = stubRoutingBackend{
		getFn: func(routingview.Input) (routingview.Snapshot, error) {
			return routingview.Snapshot{
				RoutingViewSnapshot:      "routing-view/backend",
				AdmissionPolicySnapshot:  "admission-policy/backend",
				ABICompatibilitySnapshot: "abi-compat/backend",
			}, nil
		},
	}

	policyService := policy.NewService()
	policyService.Backend = stubPolicyBackend{
		evalFn: func(policy.Input) (policy.Output, error) {
			return policy.Output{
				PolicyResolutionSnapshot: "policy-resolution/backend",
				AllowedAdaptiveActions:   []string{"fallback", "retry"},
				ResolvedPolicy: policy.ResolvedTurnPolicy{
					Budgets: controlplane.Budgets{
						TurnBudgetMS:        8000,
						NodeBudgetMSDefault: 1500,
						PathBudgetMSDefault: 5000,
						EdgeBudgetMSDefault: 800,
					},
					ProviderBindings: map[string]string{
						"stt": "stt-backend",
						"llm": "llm-backend",
						"tts": "tts-backend",
					},
					EdgeBufferPolicies: map[string]controlplane.EdgeBufferPolicy{
						"default": {
							Strategy:                 controlplane.BufferStrategyDrop,
							MaxQueueItems:            48,
							MaxQueueMS:               200,
							MaxQueueBytes:            196608,
							MaxLatencyContributionMS: 90,
							Watermarks: controlplane.EdgeWatermarks{
								QueueItems: &controlplane.WatermarkThreshold{High: 30, Low: 10},
							},
							LaneHandling: controlplane.LaneHandling{
								DataLane:      "drop",
								ControlLane:   "non_blocking_priority",
								TelemetryLane: "best_effort_drop",
							},
							DefaultingSource: "explicit_edge_config",
						},
					},
					NodeExecutionPolicies: map[string]controlplane.NodeExecutionPolicy{
						"provider-stt": {
							ConcurrencyLimit: 1,
							FairnessKey:      "provider-heavy",
						},
					},
					FlowControl: controlplane.FlowControl{
						ModeByLane: controlplane.ModeByLane{
							DataLane:      "signal",
							ControlLane:   "signal",
							TelemetryLane: "signal",
						},
						Watermarks: controlplane.FlowWatermarks{
							DataLane:      controlplane.WatermarkThreshold{High: 60, Low: 20},
							ControlLane:   controlplane.WatermarkThreshold{High: 18, Low: 9},
							TelemetryLane: controlplane.WatermarkThreshold{High: 100, Low: 40},
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
	}

	providerHealthService := providerhealth.NewService()
	providerHealthService.Backend = stubProviderHealthBackend{
		getFn: func(providerhealth.Input) (providerhealth.Output, error) {
			return providerhealth.Output{ProviderHealthSnapshot: "provider-health/backend"}, nil
		},
	}

	resolver := NewControlPlaneBundleResolverWithServices(ControlPlaneBundleServices{
		Registry:       registryService,
		Normalizer:     normalizer.Service{},
		Rollout:        rolloutService,
		RoutingView:    routingService,
		Policy:         policyService,
		ProviderHealth: providerHealthService,
	})

	bundle, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		TenantID:                 "tenant-backend-1",
		SessionID:                "sess-backend-1",
		TurnID:                   "turn-backend-1",
		RequestedPipelineVersion: "pipeline-requested",
		AuthorityEpoch:           2,
	})
	if err != nil {
		t.Fatalf("unexpected backend bundle resolve error: %v", err)
	}
	if bundle.PipelineVersion != "pipeline-rollout-backend" {
		t.Fatalf("expected rollout backend version, got %+v", bundle)
	}
	if bundle.GraphDefinitionRef != "graph/backend" || bundle.ExecutionProfile != "simple" {
		t.Fatalf("expected backend graph/profile from registry-normalizer path, got %+v", bundle)
	}
	if !reflect.DeepEqual(bundle.AllowedAdaptiveActions, []string{"retry", "fallback"}) {
		t.Fatalf("expected normalized backend policy actions, got %+v", bundle.AllowedAdaptiveActions)
	}
	if bundle.SnapshotProvenance.RoutingViewSnapshot != "routing-view/backend" ||
		bundle.SnapshotProvenance.AdmissionPolicySnapshot != "admission-policy/backend" ||
		bundle.SnapshotProvenance.ABICompatibilitySnapshot != "abi-compat/backend" ||
		bundle.SnapshotProvenance.VersionResolutionSnapshot != "version-resolution/backend" ||
		bundle.SnapshotProvenance.PolicyResolutionSnapshot != "policy-resolution/backend" ||
		bundle.SnapshotProvenance.ProviderHealthSnapshot != "provider-health/backend" {
		t.Fatalf("unexpected backend snapshot provenance: %+v", bundle.SnapshotProvenance)
	}
	if bundle.Budgets.TurnBudgetMS != 8000 || bundle.ProviderBindings["llm"] != "llm-backend" {
		t.Fatalf("expected backend policy surfaces in bundle, got %+v", bundle)
	}
	if capturedRolloutInput.TenantID != "tenant-backend-1" {
		t.Fatalf("expected tenant threading to rollout input, got %+v", capturedRolloutInput)
	}
	if policy, ok := bundle.EdgeBufferPolicies["default"]; !ok || policy.MaxQueueItems != 48 {
		t.Fatalf("expected backend edge_buffer_policies in bundle, got %+v", bundle.EdgeBufferPolicies)
	}
	if bundle.FlowControl.Watermarks.ControlLane.High != 18 {
		t.Fatalf("expected backend flow_control in bundle, got %+v", bundle.FlowControl)
	}
	if nodePolicy, ok := bundle.NodeExecutionPolicies["provider-stt"]; !ok || nodePolicy.ConcurrencyLimit != 1 || nodePolicy.FairnessKey != "provider-heavy" {
		t.Fatalf("expected backend node_execution_policies in bundle, got %+v", bundle.NodeExecutionPolicies)
	}
	if !reflect.DeepEqual(bundle.RecordingPolicy.AllowedReplayModes, []string{"replay_decisions"}) {
		t.Fatalf("expected backend recording_policy in bundle, got %+v", bundle.RecordingPolicy)
	}
	if bundle.LeaseTokenID == "" || bundle.LeaseTokenExpiresAtUTC == "" {
		t.Fatalf("expected lease token metadata in resolved bundle, got %+v", bundle)
	}
}

func TestResolveTurnStartBundleRejectsUnsupportedExecutionProfile(t *testing.T) {
	t.Parallel()

	registryService := registry.NewService()
	registryService.Backend = stubRegistryBackend{
		resolveFn: func(version string) (registry.PipelineRecord, error) {
			return registry.PipelineRecord{
				PipelineVersion:    version,
				GraphDefinitionRef: "graph/backend",
				ExecutionProfile:   "advanced",
			}, nil
		},
	}

	resolver := NewControlPlaneBundleResolverWithServices(ControlPlaneBundleServices{
		Registry:       registryService,
		Normalizer:     normalizer.Service{},
		Rollout:        rollout.NewService(),
		RoutingView:    routingview.NewService(),
		Policy:         policy.NewService(),
		ProviderHealth: providerhealth.NewService(),
	})

	_, err := resolver.ResolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                "sess-invalid-profile-1",
		TurnID:                   "turn-invalid-profile-1",
		RequestedPipelineVersion: "pipeline-requested",
	})
	if err == nil {
		t.Fatalf("expected unsupported execution profile error")
	}
	if got := err.Error(); got == "" || !containsAll(got, "normalize pipeline record", "unsupported in MVP") {
		t.Fatalf("expected normalize unsupported-profile error, got %v", err)
	}
}

func TestIsStaleSnapshotResolutionError(t *testing.T) {
	t.Parallel()

	if isStaleSnapshotResolutionError(nil) {
		t.Fatalf("nil error must not be classified as stale")
	}
	if isStaleSnapshotResolutionError(errors.New("backend unavailable")) {
		t.Fatalf("generic errors must not be classified as stale")
	}

	wrapped := errors.Join(staleSnapshotBundleTestError{reason: "snapshot stale"}, errors.New("context"))
	if !isStaleSnapshotResolutionError(wrapped) {
		t.Fatalf("expected stale marker error classification")
	}
}

type stubRegistryBackend struct {
	resolveFn func(version string) (registry.PipelineRecord, error)
}

func (s stubRegistryBackend) ResolvePipelineRecord(version string) (registry.PipelineRecord, error) {
	return s.resolveFn(version)
}

type stubRolloutBackend struct {
	resolveFn func(in rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error)
}

func (s stubRolloutBackend) ResolvePipelineVersion(in rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
	return s.resolveFn(in)
}

type stubRoutingBackend struct {
	getFn func(in routingview.Input) (routingview.Snapshot, error)
}

func (s stubRoutingBackend) GetSnapshot(in routingview.Input) (routingview.Snapshot, error) {
	return s.getFn(in)
}

type stubPolicyBackend struct {
	evalFn func(in policy.Input) (policy.Output, error)
}

func (s stubPolicyBackend) Evaluate(in policy.Input) (policy.Output, error) {
	return s.evalFn(in)
}

type stubProviderHealthBackend struct {
	getFn func(in providerhealth.Input) (providerhealth.Output, error)
}

func (s stubProviderHealthBackend) GetSnapshot(in providerhealth.Input) (providerhealth.Output, error) {
	return s.getFn(in)
}

type staleSnapshotBundleTestError struct {
	reason string
}

func (e staleSnapshotBundleTestError) Error() string {
	if e.reason == "" {
		return "snapshot stale"
	}
	return e.reason
}

func (e staleSnapshotBundleTestError) StaleSnapshot() bool {
	return true
}

func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
