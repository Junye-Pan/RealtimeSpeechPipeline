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

func TestCT004ResolvedTurnPlanRequiresCompleteTurnStartInput(t *testing.T) {
	t.Parallel()

	base := Input{
		TurnID:             "turn-ct4-required",
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     2,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		AllowedAdaptiveActions: []string{"retry"},
	}

	tests := []struct {
		name  string
		input Input
	}{
		{
			name: "missing graph_definition_ref",
			input: func() Input {
				in := base
				in.GraphDefinitionRef = ""
				return in
			}(),
		},
		{
			name: "missing execution_profile",
			input: func() Input {
				in := base
				in.ExecutionProfile = ""
				return in
			}(),
		},
		{
			name: "nil adaptive actions",
			input: func() Input {
				in := base
				in.AllowedAdaptiveActions = nil
				return in
			}(),
		},
		{
			name: "incomplete snapshot provenance",
			input: func() Input {
				in := base
				in.SnapshotProvenance.ProviderHealthSnapshot = ""
				return in
			}(),
		},
		{
			name: "unsupported execution profile",
			input: func() Input {
				in := base
				in.ExecutionProfile = "advanced"
				return in
			}(),
		},
	}

	resolver := Resolver{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := resolver.Resolve(tt.input); err == nil {
				t.Fatalf("expected resolver input validation failure")
			}
		})
	}
}

func TestCT004ResolvedTurnPlanHashIncludesFrozenPolicyInputs(t *testing.T) {
	t.Parallel()

	resolver := Resolver{}
	base := Input{
		TurnID:             "turn-ct4-hash-1",
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     4,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		AllowedAdaptiveActions: []string{"retry"},
	}

	first, err := resolver.Resolve(base)
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}

	changedSnapshot := base
	changedSnapshot.SnapshotProvenance.ProviderHealthSnapshot = "provider-health/v2"
	second, err := resolver.Resolve(changedSnapshot)
	if err != nil {
		t.Fatalf("unexpected resolve with changed snapshot: %v", err)
	}
	if first.PlanHash == second.PlanHash {
		t.Fatalf("expected plan hash to change when snapshot provenance changes")
	}

	changedActions := base
	changedActions.AllowedAdaptiveActions = []string{"retry", "fallback"}
	third, err := resolver.Resolve(changedActions)
	if err != nil {
		t.Fatalf("unexpected resolve with changed actions: %v", err)
	}
	if first.PlanHash == third.PlanHash {
		t.Fatalf("expected plan hash to change when adaptive actions change")
	}
}

func TestCT004ResolvedTurnPlanUsesCPMaterializedPolicySurfaces(t *testing.T) {
	t.Parallel()

	input := Input{
		TurnID:             "turn-ct4-cp-policy-1",
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     6,
		Budgets: controlplane.Budgets{
			TurnBudgetMS:        7200,
			NodeBudgetMSDefault: 1600,
			PathBudgetMSDefault: 4200,
			EdgeBudgetMSDefault: 800,
		},
		ProviderBindings: map[string]string{
			"stt": "stt-cp",
			"llm": "llm-cp",
			"tts": "tts-cp",
		},
		EdgeBufferPolicies: map[string]controlplane.EdgeBufferPolicy{
			"default": {
				Strategy:                 controlplane.BufferStrategyDrop,
				MaxQueueItems:            33,
				MaxQueueMS:               220,
				MaxQueueBytes:            180000,
				MaxLatencyContributionMS: 80,
				Watermarks: controlplane.EdgeWatermarks{
					QueueItems: &controlplane.WatermarkThreshold{High: 24, Low: 10},
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
				DataLane:      controlplane.WatermarkThreshold{High: 64, Low: 16},
				ControlLane:   controlplane.WatermarkThreshold{High: 12, Low: 4},
				TelemetryLane: controlplane.WatermarkThreshold{High: 96, Low: 24},
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
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		AllowedAdaptiveActions: []string{"retry", "fallback"},
	}

	plan, err := Resolver{}.Resolve(input)
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}

	if plan.Budgets != input.Budgets {
		t.Fatalf("expected CP budgets in plan, got %+v", plan.Budgets)
	}
	if !reflect.DeepEqual(plan.ProviderBindings, input.ProviderBindings) {
		t.Fatalf("expected CP provider bindings in plan, got %+v", plan.ProviderBindings)
	}
	if !reflect.DeepEqual(plan.EdgeBufferPolicies, input.EdgeBufferPolicies) {
		t.Fatalf("expected CP edge buffer policies in plan, got %+v", plan.EdgeBufferPolicies)
	}
	if !reflect.DeepEqual(plan.NodeExecutionPolicies, input.NodeExecutionPolicies) {
		t.Fatalf("expected CP node execution policies in plan, got %+v", plan.NodeExecutionPolicies)
	}
	if plan.FlowControl != input.FlowControl {
		t.Fatalf("expected CP flow control in plan, got %+v", plan.FlowControl)
	}
	if !reflect.DeepEqual(plan.RecordingPolicy, input.RecordingPolicy) {
		t.Fatalf("expected CP recording policy in plan, got %+v", plan.RecordingPolicy)
	}
}

func TestCT004ResolvedTurnPlanRejectsPartialCPPolicySurfaceOverrides(t *testing.T) {
	t.Parallel()

	base := Input{
		TurnID:             "turn-ct4-cp-policy-partial",
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     6,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		AllowedAdaptiveActions: []string{"retry"},
	}

	in := base
	in.Budgets = controlplane.Budgets{
		TurnBudgetMS:        6000,
		NodeBudgetMSDefault: 1200,
		PathBudgetMSDefault: 3000,
		EdgeBudgetMSDefault: 400,
	}
	if _, err := (Resolver{}).Resolve(in); err == nil {
		t.Fatalf("expected partial CP policy surface override to fail")
	}
}
