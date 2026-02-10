package turnarbiter

import (
	"errors"
	"reflect"
	"testing"

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

	registryService := registry.NewService()
	registryService.Backend = stubRegistryBackend{
		resolveFn: func(version string) (registry.PipelineRecord, error) {
			return registry.PipelineRecord{
				PipelineVersion:    version,
				GraphDefinitionRef: "graph/backend",
				ExecutionProfile:   "profile/backend",
			}, nil
		},
	}

	rolloutService := rollout.NewService()
	rolloutService.Backend = stubRolloutBackend{
		resolveFn: func(rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
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
	if bundle.GraphDefinitionRef != "graph/backend" || bundle.ExecutionProfile != "profile/backend" {
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
