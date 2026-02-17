package policy

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

func TestEvaluate(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.DefaultAllowedAdaptiveActions = []string{"fallback", "retry", "retry", "provider_switch", "invalid"}

	out, err := service.Evaluate(Input{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("unexpected policy evaluation error: %v", err)
	}
	if out.PolicyResolutionSnapshot == "" {
		t.Fatalf("expected non-empty policy snapshot")
	}
	expectedActions := []string{"retry", "provider_switch", "fallback"}
	if !reflect.DeepEqual(out.AllowedAdaptiveActions, expectedActions) {
		t.Fatalf("expected normalized allowed actions %v, got %v", expectedActions, out.AllowedAdaptiveActions)
	}
	if err := out.ResolvedPolicy.Budgets.Validate(); err != nil {
		t.Fatalf("expected default resolved policy budgets to validate, got %v", err)
	}
	if len(out.ResolvedPolicy.ProviderBindings) == 0 || len(out.ResolvedPolicy.EdgeBufferPolicies) == 0 {
		t.Fatalf("expected default resolved policy to include provider and edge policies, got %+v", out.ResolvedPolicy)
	}
	if out.ResolvedPolicy.NodeExecutionPolicies == nil {
		t.Fatalf("expected default resolved policy node execution policies map to be initialized")
	}
	if err := out.ResolvedPolicy.FlowControl.Validate(); err != nil {
		t.Fatalf("expected default flow control to validate, got %v", err)
	}
	if err := out.ResolvedPolicy.RecordingPolicy.Validate(); err != nil {
		t.Fatalf("expected default recording policy to validate, got %v", err)
	}
}

func TestEvaluateRequiresSessionID(t *testing.T) {
	t.Parallel()

	if _, err := NewService().Evaluate(Input{}); err == nil {
		t.Fatalf("expected session_id validation error")
	}
}

func TestEvaluateUsesBackendWhenConfigured(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubPolicyBackend{
		evalFn: func(Input) (Output, error) {
			return Output{
				PolicyResolutionSnapshot: "policy-resolution/backend",
				AllowedAdaptiveActions:   []string{"degrade", "fallback", "retry"},
				ResolvedPolicy: ResolvedTurnPolicy{
					Budgets: controlplane.Budgets{
						TurnBudgetMS:        9000,
						NodeBudgetMSDefault: 1900,
						PathBudgetMSDefault: 5900,
						EdgeBudgetMSDefault: 900,
					},
					ProviderBindings: map[string]string{
						"stt": "stt-backend",
						"llm": "llm-backend",
						"tts": "tts-backend",
					},
					EdgeBufferPolicies: map[string]controlplane.EdgeBufferPolicy{
						"default": {
							Strategy:                 controlplane.BufferStrategyDrop,
							MaxQueueItems:            77,
							MaxQueueMS:               400,
							MaxQueueBytes:            300000,
							MaxLatencyContributionMS: 140,
							Watermarks: controlplane.EdgeWatermarks{
								QueueItems: &controlplane.WatermarkThreshold{High: 50, Low: 20},
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
							DataLane:      controlplane.WatermarkThreshold{High: 70, Low: 35},
							ControlLane:   controlplane.WatermarkThreshold{High: 25, Low: 12},
							TelemetryLane: controlplane.WatermarkThreshold{High: 120, Low: 60},
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

	out, err := service.Evaluate(Input{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("unexpected backend evaluation error: %v", err)
	}
	expectedActions := []string{"retry", "degrade", "fallback"}
	if out.PolicyResolutionSnapshot != "policy-resolution/backend" || !reflect.DeepEqual(out.AllowedAdaptiveActions, expectedActions) {
		t.Fatalf("unexpected backend policy output: %+v", out)
	}
	if out.ResolvedPolicy.Budgets.TurnBudgetMS != 9000 || out.ResolvedPolicy.ProviderBindings["llm"] != "llm-backend" {
		t.Fatalf("expected backend resolved policy to propagate, got %+v", out.ResolvedPolicy)
	}
	if policy, ok := out.ResolvedPolicy.NodeExecutionPolicies["provider-heavy"]; !ok || policy.ConcurrencyLimit != 1 || policy.FairnessKey != "provider-heavy" {
		t.Fatalf("expected backend node execution policy to propagate, got %+v", out.ResolvedPolicy.NodeExecutionPolicies)
	}
}

func TestEvaluateBackendError(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubPolicyBackend{
		evalFn: func(Input) (Output, error) {
			return Output{}, errors.New("backend unavailable")
		},
	}

	if _, err := service.Evaluate(Input{SessionID: "sess-1"}); err == nil {
		t.Fatalf("expected backend error")
	}
}

func TestEvaluateRejectsInvalidBackendPolicySurface(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubPolicyBackend{
		evalFn: func(Input) (Output, error) {
			return Output{
				PolicyResolutionSnapshot: "policy-resolution/backend",
				AllowedAdaptiveActions:   []string{"retry"},
				ResolvedPolicy: ResolvedTurnPolicy{
					Budgets: controlplane.Budgets{
						TurnBudgetMS:        1000,
						NodeBudgetMSDefault: 500,
						PathBudgetMSDefault: 1000,
						EdgeBudgetMSDefault: 100,
					},
					ProviderBindings: map[string]string{
						"stt": "",
					},
				},
			}, nil
		},
	}

	if _, err := service.Evaluate(Input{SessionID: "sess-1"}); err == nil {
		t.Fatalf("expected invalid backend policy surface to fail")
	}
}

func TestEvaluateRejectsInvalidBackendNodeExecutionPolicy(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubPolicyBackend{
		evalFn: func(Input) (Output, error) {
			return Output{
				PolicyResolutionSnapshot: "policy-resolution/backend",
				AllowedAdaptiveActions:   []string{"retry"},
				ResolvedPolicy: ResolvedTurnPolicy{
					Budgets: controlplane.Budgets{
						TurnBudgetMS:        1000,
						NodeBudgetMSDefault: 500,
						PathBudgetMSDefault: 1000,
						EdgeBudgetMSDefault: 100,
					},
					ProviderBindings: map[string]string{
						"stt": "stt-backend",
						"llm": "llm-backend",
						"tts": "tts-backend",
					},
					EdgeBufferPolicies: map[string]controlplane.EdgeBufferPolicy{
						"default": {
							Strategy:                 controlplane.BufferStrategyDrop,
							MaxQueueItems:            10,
							MaxQueueMS:               20,
							MaxQueueBytes:            5000,
							MaxLatencyContributionMS: 10,
							Watermarks: controlplane.EdgeWatermarks{
								QueueItems: &controlplane.WatermarkThreshold{High: 8, Low: 4},
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
							ConcurrencyLimit: -1,
						},
					},
					FlowControl: controlplane.FlowControl{
						ModeByLane: controlplane.ModeByLane{
							DataLane:      "signal",
							ControlLane:   "signal",
							TelemetryLane: "signal",
						},
						Watermarks: controlplane.FlowWatermarks{
							DataLane:      controlplane.WatermarkThreshold{High: 10, Low: 5},
							ControlLane:   controlplane.WatermarkThreshold{High: 10, Low: 5},
							TelemetryLane: controlplane.WatermarkThreshold{High: 10, Low: 5},
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

	if _, err := service.Evaluate(Input{SessionID: "sess-1"}); err == nil {
		t.Fatalf("expected invalid backend node_execution_policy to fail")
	}
}

type stubPolicyBackend struct {
	evalFn func(in Input) (Output, error)
}

func (s stubPolicyBackend) Evaluate(in Input) (Output, error) {
	return s.evalFn(in)
}
