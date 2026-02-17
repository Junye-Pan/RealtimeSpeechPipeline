package policy

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

const defaultPolicyResolutionSnapshot = "policy-resolution/v1"

var allowedActionOrder = []string{"retry", "provider_switch", "degrade", "fallback"}

// ResolvedTurnPolicy is the CP-materialized runtime policy surface for turn-plan freeze.
type ResolvedTurnPolicy struct {
	Budgets               controlplane.Budgets
	ProviderBindings      map[string]string
	EdgeBufferPolicies    map[string]controlplane.EdgeBufferPolicy
	NodeExecutionPolicies map[string]controlplane.NodeExecutionPolicy
	FlowControl           controlplane.FlowControl
	RecordingPolicy       controlplane.RecordingPolicy
}

// Input models CP-04 policy evaluation context.
type Input struct {
	TenantID               string
	SessionID              string
	TurnID                 string
	PipelineVersion        string
	ProviderHealthSnapshot string
}

// Output is the policy evaluation artifact consumed by runtime turn-start.
type Output struct {
	PolicyResolutionSnapshot string
	AllowedAdaptiveActions   []string
	ResolvedPolicy           ResolvedTurnPolicy
}

// Backend evaluates policy from a snapshot-fed control-plane source.
type Backend interface {
	Evaluate(in Input) (Output, error)
}

// Service returns deterministic CP-04 policy outputs.
type Service struct {
	DefaultPolicyResolutionSnapshot string
	DefaultAllowedAdaptiveActions   []string
	DefaultResolvedPolicy           ResolvedTurnPolicy
	Backend                         Backend
}

// NewService returns baseline CP-04 deterministic policy defaults.
func NewService() Service {
	return Service{
		DefaultPolicyResolutionSnapshot: defaultPolicyResolutionSnapshot,
		DefaultAllowedAdaptiveActions:   []string{"retry", "provider_switch", "fallback"},
		DefaultResolvedPolicy:           defaultResolvedTurnPolicy(),
	}
}

// Evaluate resolves policy snapshot and pre-authorized adaptive actions.
func (s Service) Evaluate(in Input) (Output, error) {
	if in.SessionID == "" {
		return Output{}, fmt.Errorf("session_id is required")
	}

	if s.Backend != nil {
		out, err := s.Backend.Evaluate(in)
		if err != nil {
			return Output{}, fmt.Errorf("evaluate policy backend: %w", err)
		}
		return s.normalizeOutput(out)
	}

	return s.normalizeOutput(Output{})
}

func (s Service) normalizeOutput(out Output) (Output, error) {
	snapshot := s.DefaultPolicyResolutionSnapshot
	if snapshot == "" {
		snapshot = defaultPolicyResolutionSnapshot
	}
	if out.PolicyResolutionSnapshot != "" {
		snapshot = out.PolicyResolutionSnapshot
	}

	actions := s.DefaultAllowedAdaptiveActions
	if len(out.AllowedAdaptiveActions) > 0 {
		actions = out.AllowedAdaptiveActions
	}
	normalizedActions := normalizeAllowedAdaptiveActions(actions)

	defaultPolicy := s.DefaultResolvedPolicy
	if isZeroResolvedTurnPolicy(defaultPolicy) {
		defaultPolicy = defaultResolvedTurnPolicy()
	}
	normalizedPolicy, err := normalizeResolvedTurnPolicy(out.ResolvedPolicy, defaultPolicy)
	if err != nil {
		return Output{}, err
	}

	return Output{
		PolicyResolutionSnapshot: snapshot,
		AllowedAdaptiveActions:   normalizedActions,
		ResolvedPolicy:           normalizedPolicy,
	}, nil
}

func normalizeResolvedTurnPolicy(candidate ResolvedTurnPolicy, defaults ResolvedTurnPolicy) (ResolvedTurnPolicy, error) {
	out := ResolvedTurnPolicy{
		Budgets:               defaults.Budgets,
		ProviderBindings:      cloneStringMap(defaults.ProviderBindings),
		EdgeBufferPolicies:    cloneEdgeBufferPolicies(defaults.EdgeBufferPolicies),
		NodeExecutionPolicies: cloneNodeExecutionPolicies(defaults.NodeExecutionPolicies),
		FlowControl:           defaults.FlowControl,
		RecordingPolicy:       cloneRecordingPolicy(defaults.RecordingPolicy),
	}

	if candidate.Budgets != (controlplane.Budgets{}) {
		out.Budgets = candidate.Budgets
	}
	if len(candidate.ProviderBindings) > 0 {
		out.ProviderBindings = cloneStringMap(candidate.ProviderBindings)
	}
	if len(candidate.EdgeBufferPolicies) > 0 {
		out.EdgeBufferPolicies = cloneEdgeBufferPolicies(candidate.EdgeBufferPolicies)
	}
	if candidate.NodeExecutionPolicies != nil {
		out.NodeExecutionPolicies = cloneNodeExecutionPolicies(candidate.NodeExecutionPolicies)
	}
	if candidate.FlowControl != (controlplane.FlowControl{}) {
		out.FlowControl = candidate.FlowControl
	}
	if !isZeroRecordingPolicy(candidate.RecordingPolicy) {
		out.RecordingPolicy = cloneRecordingPolicy(candidate.RecordingPolicy)
	}

	if err := out.Budgets.Validate(); err != nil {
		return ResolvedTurnPolicy{}, err
	}
	if len(out.ProviderBindings) == 0 {
		return ResolvedTurnPolicy{}, fmt.Errorf("provider_bindings must be non-empty")
	}
	for modality, providerID := range out.ProviderBindings {
		if modality == "" || providerID == "" {
			return ResolvedTurnPolicy{}, fmt.Errorf("provider_bindings keys and values must be non-empty")
		}
	}
	if len(out.EdgeBufferPolicies) == 0 {
		return ResolvedTurnPolicy{}, fmt.Errorf("edge_buffer_policies must be non-empty")
	}
	for edgeID, policy := range out.EdgeBufferPolicies {
		if edgeID == "" {
			return ResolvedTurnPolicy{}, fmt.Errorf("edge_buffer_policies key cannot be empty")
		}
		if err := policy.Validate(); err != nil {
			return ResolvedTurnPolicy{}, err
		}
	}
	for nodeID, policy := range out.NodeExecutionPolicies {
		if nodeID == "" {
			return ResolvedTurnPolicy{}, fmt.Errorf("node_execution_policies key cannot be empty")
		}
		if err := policy.Validate(); err != nil {
			return ResolvedTurnPolicy{}, err
		}
	}
	if err := out.FlowControl.Validate(); err != nil {
		return ResolvedTurnPolicy{}, err
	}
	if err := out.RecordingPolicy.Validate(); err != nil {
		return ResolvedTurnPolicy{}, err
	}

	return out, nil
}

func defaultResolvedTurnPolicy() ResolvedTurnPolicy {
	return ResolvedTurnPolicy{
		Budgets: controlplane.Budgets{
			TurnBudgetMS:        5000,
			NodeBudgetMSDefault: 1500,
			PathBudgetMSDefault: 3000,
			EdgeBudgetMSDefault: 500,
		},
		ProviderBindings: map[string]string{
			"stt": "default-stt",
			"llm": "default-llm",
			"tts": "default-tts",
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
		NodeExecutionPolicies: map[string]controlplane.NodeExecutionPolicy{},
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
			AllowedReplayModes: []string{string(obs.ReplayModeReplayDecisions)},
		},
	}
}

func isZeroResolvedTurnPolicy(policy ResolvedTurnPolicy) bool {
	return policy.Budgets == (controlplane.Budgets{}) &&
		len(policy.ProviderBindings) == 0 &&
		len(policy.EdgeBufferPolicies) == 0 &&
		len(policy.NodeExecutionPolicies) == 0 &&
		policy.FlowControl == (controlplane.FlowControl{}) &&
		isZeroRecordingPolicy(policy.RecordingPolicy)
}

func isZeroRecordingPolicy(policy controlplane.RecordingPolicy) bool {
	return policy.RecordingLevel == "" && len(policy.AllowedReplayModes) == 0
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneEdgeBufferPolicies(in map[string]controlplane.EdgeBufferPolicy) map[string]controlplane.EdgeBufferPolicy {
	out := make(map[string]controlplane.EdgeBufferPolicy, len(in))
	for key, value := range in {
		cloned := value
		if value.Watermarks.QueueItems != nil {
			queueItems := *value.Watermarks.QueueItems
			cloned.Watermarks.QueueItems = &queueItems
		}
		if value.Watermarks.QueueMS != nil {
			queueMS := *value.Watermarks.QueueMS
			cloned.Watermarks.QueueMS = &queueMS
		}
		if value.SyncDropPolicy != nil {
			syncDropPolicy := *value.SyncDropPolicy
			cloned.SyncDropPolicy = &syncDropPolicy
		}
		out[key] = cloned
	}
	return out
}

func cloneRecordingPolicy(in controlplane.RecordingPolicy) controlplane.RecordingPolicy {
	out := in
	out.AllowedReplayModes = append([]string(nil), in.AllowedReplayModes...)
	return out
}

func cloneResolvedTurnPolicy(in ResolvedTurnPolicy) ResolvedTurnPolicy {
	return ResolvedTurnPolicy{
		Budgets:               in.Budgets,
		ProviderBindings:      cloneStringMap(in.ProviderBindings),
		EdgeBufferPolicies:    cloneEdgeBufferPolicies(in.EdgeBufferPolicies),
		NodeExecutionPolicies: cloneNodeExecutionPolicies(in.NodeExecutionPolicies),
		FlowControl:           in.FlowControl,
		RecordingPolicy:       cloneRecordingPolicy(in.RecordingPolicy),
	}
}

func cloneNodeExecutionPolicies(in map[string]controlplane.NodeExecutionPolicy) map[string]controlplane.NodeExecutionPolicy {
	if in == nil {
		return nil
	}
	out := make(map[string]controlplane.NodeExecutionPolicy, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func normalizeAllowedAdaptiveActions(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(in))
	for _, action := range in {
		seen[action] = struct{}{}
	}

	out := make([]string, 0, len(allowedActionOrder))
	for _, action := range allowedActionOrder {
		if _, ok := seen[action]; ok {
			out = append(out, action)
		}
	}
	return out
}
