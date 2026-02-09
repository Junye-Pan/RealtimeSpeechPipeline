package planresolver

import (
	"reflect"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

func TestCT004ResolvedTurnPlanFrozenFieldsAndProvenance(t *testing.T) {
	t.Parallel()

	resolver := Resolver{}
	input := Input{
		TurnID:             "turn-ct4-1",
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     9,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		AllowedAdaptiveActions: []string{},
	}
	plan, err := resolver.Resolve(input)
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}

	if plan.TurnID == "" || plan.PipelineVersion == "" || plan.PlanHash == "" {
		t.Fatalf("expected frozen identity fields to be populated, got %+v", plan)
	}
	if plan.SnapshotProvenance.RoutingViewSnapshot == "" ||
		plan.SnapshotProvenance.AdmissionPolicySnapshot == "" ||
		plan.SnapshotProvenance.ABICompatibilitySnapshot == "" ||
		plan.SnapshotProvenance.VersionResolutionSnapshot == "" ||
		plan.SnapshotProvenance.PolicyResolutionSnapshot == "" ||
		plan.SnapshotProvenance.ProviderHealthSnapshot == "" {
		t.Fatalf("expected full snapshot provenance to be present, got %+v", plan.SnapshotProvenance)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("resolved plan must validate: %v", err)
	}
}

func TestCT004ResolvedTurnPlanDeterministicIdentity(t *testing.T) {
	t.Parallel()

	resolver := Resolver{}
	input := Input{
		TurnID:             "turn-ct4-2",
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     11,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		AllowedAdaptiveActions: []string{},
	}

	first, err := resolver.Resolve(input)
	if err != nil {
		t.Fatalf("unexpected first resolve error: %v", err)
	}
	second, err := resolver.Resolve(input)
	if err != nil {
		t.Fatalf("unexpected second resolve error: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic resolved plan for identical input")
	}

	modified := input
	modified.AuthorityEpoch = 12
	third, err := resolver.Resolve(modified)
	if err != nil {
		t.Fatalf("unexpected third resolve error: %v", err)
	}
	if first.PlanHash == third.PlanHash {
		t.Fatalf("expected plan hash to change when authority epoch changes")
	}
}
