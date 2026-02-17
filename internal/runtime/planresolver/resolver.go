package planresolver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	runtimedeterminism "github.com/tiger/realtime-speech-pipeline/internal/runtime/determinism"
)

var ErrMaterializationFailed = errors.New("resolved turn plan materialization failed")

// Input contains deterministic fields used to freeze turn execution.
type Input struct {
	TurnID                 string
	PipelineVersion        string
	GraphDefinitionRef     string
	ExecutionProfile       string
	AuthorityEpoch         int64
	Budgets                controlplane.Budgets
	ProviderBindings       map[string]string
	EdgeBufferPolicies     map[string]controlplane.EdgeBufferPolicy
	NodeExecutionPolicies  map[string]controlplane.NodeExecutionPolicy
	FlowControl            controlplane.FlowControl
	RecordingPolicy        controlplane.RecordingPolicy
	SnapshotProvenance     controlplane.SnapshotProvenance
	FailMaterialization    bool
	AllowedAdaptiveActions []string
}

// Resolver materializes immutable ResolvedTurnPlan artifacts.
type Resolver struct{}

func (Resolver) Resolve(in Input) (controlplane.ResolvedTurnPlan, error) {
	if in.FailMaterialization {
		return controlplane.ResolvedTurnPlan{}, ErrMaterializationFailed
	}
	in.ExecutionProfile = strings.TrimSpace(in.ExecutionProfile)
	if err := in.Validate(); err != nil {
		return controlplane.ResolvedTurnPlan{}, err
	}

	profilePolicy, err := resolvePolicySurface(in)
	if err != nil {
		return controlplane.ResolvedTurnPlan{}, err
	}
	planHash, err := hashResolvedPlan(planIdentity{
		TurnID:                 in.TurnID,
		PipelineVersion:        in.PipelineVersion,
		GraphDefinitionRef:     in.GraphDefinitionRef,
		ExecutionProfile:       in.ExecutionProfile,
		AuthorityEpoch:         in.AuthorityEpoch,
		SnapshotProvenance:     in.SnapshotProvenance,
		AllowedAdaptiveActions: cloneAdaptiveActions(in.AllowedAdaptiveActions),
		Budgets:                profilePolicy.Budgets,
		ProviderBindings:       profilePolicy.ProviderBindings,
		EdgeBufferPolicies:     profilePolicy.EdgeBufferPolicies,
		NodeExecutionPolicies:  profilePolicy.NodeExecutionPolicies,
		FlowControl:            profilePolicy.FlowControl,
		RecordingPolicy:        profilePolicy.RecordingPolicy,
	})
	if err != nil {
		return controlplane.ResolvedTurnPlan{}, err
	}

	determinismCtx, err := runtimedeterminism.NewService().IssueContext(planHash, in.AuthorityEpoch)
	if err != nil {
		return controlplane.ResolvedTurnPlan{}, err
	}

	plan := controlplane.ResolvedTurnPlan{
		TurnID:                 in.TurnID,
		PipelineVersion:        in.PipelineVersion,
		PlanHash:               planHash,
		GraphDefinitionRef:     in.GraphDefinitionRef,
		ExecutionProfile:       in.ExecutionProfile,
		AuthorityEpoch:         in.AuthorityEpoch,
		Budgets:                profilePolicy.Budgets,
		ProviderBindings:       cloneStringMap(profilePolicy.ProviderBindings),
		EdgeBufferPolicies:     cloneEdgeBufferPolicies(profilePolicy.EdgeBufferPolicies),
		NodeExecutionPolicies:  cloneNodeExecutionPolicies(profilePolicy.NodeExecutionPolicies),
		FlowControl:            profilePolicy.FlowControl,
		AllowedAdaptiveActions: cloneAdaptiveActions(in.AllowedAdaptiveActions),
		SnapshotProvenance:     in.SnapshotProvenance,
		RecordingPolicy:        profilePolicy.RecordingPolicy,
		Determinism:            determinismCtx,
	}

	if err := plan.Validate(); err != nil {
		return controlplane.ResolvedTurnPlan{}, err
	}

	return plan, nil
}

func (in Input) Validate() error {
	if in.TurnID == "" || in.PipelineVersion == "" || in.GraphDefinitionRef == "" || in.ExecutionProfile == "" {
		return fmt.Errorf("turn_id, pipeline_version, graph_definition_ref, and execution_profile are required")
	}
	if in.AllowedAdaptiveActions == nil {
		return fmt.Errorf("allowed_adaptive_actions is required")
	}
	if err := in.SnapshotProvenance.Validate(); err != nil {
		return err
	}
	if hasAnyPolicySurfaceOverrides(in) {
		if err := validatePolicySurfaceOverrides(in); err != nil {
			return err
		}
	}
	return nil
}

type executionProfilePolicy struct {
	Budgets               controlplane.Budgets
	ProviderBindings      map[string]string
	EdgeBufferPolicies    map[string]controlplane.EdgeBufferPolicy
	NodeExecutionPolicies map[string]controlplane.NodeExecutionPolicy
	FlowControl           controlplane.FlowControl
	RecordingPolicy       controlplane.RecordingPolicy
}

func resolveExecutionProfilePolicy(profile string) (executionProfilePolicy, error) {
	switch strings.TrimSpace(profile) {
	case "simple", "simple/v1":
		return simpleExecutionProfilePolicy(), nil
	default:
		return executionProfilePolicy{}, fmt.Errorf("unsupported execution_profile %q", profile)
	}
}

func resolvePolicySurface(in Input) (executionProfilePolicy, error) {
	if !hasAnyPolicySurfaceOverrides(in) {
		return resolveExecutionProfilePolicy(in.ExecutionProfile)
	}
	if err := validatePolicySurfaceOverrides(in); err != nil {
		return executionProfilePolicy{}, err
	}
	return executionProfilePolicy{
		Budgets:               in.Budgets,
		ProviderBindings:      cloneStringMap(in.ProviderBindings),
		EdgeBufferPolicies:    cloneEdgeBufferPolicies(in.EdgeBufferPolicies),
		NodeExecutionPolicies: cloneNodeExecutionPolicies(in.NodeExecutionPolicies),
		FlowControl:           in.FlowControl,
		RecordingPolicy:       cloneRecordingPolicy(in.RecordingPolicy),
	}, nil
}

func hasAnyPolicySurfaceOverrides(in Input) bool {
	return in.Budgets != (controlplane.Budgets{}) ||
		len(in.ProviderBindings) > 0 ||
		len(in.EdgeBufferPolicies) > 0 ||
		in.NodeExecutionPolicies != nil ||
		in.FlowControl != (controlplane.FlowControl{}) ||
		!isZeroRecordingPolicy(in.RecordingPolicy)
}

func validatePolicySurfaceOverrides(in Input) error {
	if in.Budgets == (controlplane.Budgets{}) ||
		len(in.ProviderBindings) == 0 ||
		len(in.EdgeBufferPolicies) == 0 ||
		in.NodeExecutionPolicies == nil ||
		in.FlowControl == (controlplane.FlowControl{}) ||
		isZeroRecordingPolicy(in.RecordingPolicy) {
		return fmt.Errorf("policy surface overrides must include budgets, provider_bindings, edge_buffer_policies, node_execution_policies, flow_control, and recording_policy")
	}
	if err := in.Budgets.Validate(); err != nil {
		return err
	}
	for modality, providerID := range in.ProviderBindings {
		if modality == "" || providerID == "" {
			return fmt.Errorf("provider_bindings keys and values must be non-empty")
		}
	}
	for edgeID, policy := range in.EdgeBufferPolicies {
		if edgeID == "" {
			return fmt.Errorf("edge_buffer_policies key cannot be empty")
		}
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	for nodeID, policy := range in.NodeExecutionPolicies {
		if nodeID == "" {
			return fmt.Errorf("node_execution_policies key cannot be empty")
		}
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	if err := in.FlowControl.Validate(); err != nil {
		return err
	}
	if err := in.RecordingPolicy.Validate(); err != nil {
		return err
	}
	return nil
}

func simpleExecutionProfilePolicy() executionProfilePolicy {
	return executionProfilePolicy{
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

type planIdentity struct {
	TurnID                 string
	PipelineVersion        string
	GraphDefinitionRef     string
	ExecutionProfile       string
	AuthorityEpoch         int64
	SnapshotProvenance     controlplane.SnapshotProvenance
	AllowedAdaptiveActions []string
	Budgets                controlplane.Budgets
	ProviderBindings       map[string]string
	EdgeBufferPolicies     map[string]controlplane.EdgeBufferPolicy
	NodeExecutionPolicies  map[string]controlplane.NodeExecutionPolicy
	FlowControl            controlplane.FlowControl
	RecordingPolicy        controlplane.RecordingPolicy
}

func hashResolvedPlan(identity planIdentity) (string, error) {
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
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

func cloneAdaptiveActions(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneNodeExecutionPolicies(in map[string]controlplane.NodeExecutionPolicy) map[string]controlplane.NodeExecutionPolicy {
	if in == nil {
		return nil
	}
	out := make(map[string]controlplane.NodeExecutionPolicy, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRecordingPolicy(in controlplane.RecordingPolicy) controlplane.RecordingPolicy {
	out := in
	out.AllowedReplayModes = append([]string(nil), in.AllowedReplayModes...)
	return out
}

func isZeroRecordingPolicy(in controlplane.RecordingPolicy) bool {
	return in.RecordingLevel == "" && len(in.AllowedReplayModes) == 0
}
