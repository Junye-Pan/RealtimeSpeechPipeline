package planresolver

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
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

	if in.TurnID == "" || in.PipelineVersion == "" {
		return controlplane.ResolvedTurnPlan{}, fmt.Errorf("turn_id and pipeline_version are required")
	}

	if in.GraphDefinitionRef == "" {
		in.GraphDefinitionRef = "graph/default"
	}
	if in.ExecutionProfile == "" {
		in.ExecutionProfile = "simple"
	}

	if in.AllowedAdaptiveActions == nil {
		in.AllowedAdaptiveActions = []string{}
	}

	determinismCtx, err := runtimedeterminism.NewService().IssueContext(
		hashPlanIdentity(in.TurnID, in.PipelineVersion, in.GraphDefinitionRef, in.ExecutionProfile, in.AuthorityEpoch),
		in.AuthorityEpoch,
	)
	if err != nil {
		return controlplane.ResolvedTurnPlan{}, err
	}

	plan := controlplane.ResolvedTurnPlan{
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		PlanHash:           hashPlanIdentity(in.TurnID, in.PipelineVersion, in.GraphDefinitionRef, in.ExecutionProfile, in.AuthorityEpoch),
		GraphDefinitionRef: in.GraphDefinitionRef,
		ExecutionProfile:   in.ExecutionProfile,
		AuthorityEpoch:     in.AuthorityEpoch,
		Budgets: controlplane.Budgets{
			TurnBudgetMS:        5000,
			NodeBudgetMSDefault: 1500,
			PathBudgetMSDefault: 3000,
			EdgeBudgetMSDefault: 500,
		},
		ProviderBindings: map[string]string{"stt": "default-stt", "llm": "default-llm", "tts": "default-tts"},
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
		AllowedAdaptiveActions: in.AllowedAdaptiveActions,
		SnapshotProvenance:     in.SnapshotProvenance,
		RecordingPolicy: controlplane.RecordingPolicy{
			RecordingLevel:     "L0",
			AllowedReplayModes: []string{"replay_decisions"},
		},
		Determinism: determinismCtx,
	}

	if err := plan.Validate(); err != nil {
		return controlplane.ResolvedTurnPlan{}, err
	}

	return plan, nil
}

func hashPlanIdentity(turnID, pipelineVersion, graphRef, profile string, epoch int64) string {
	s := fmt.Sprintf("%s|%s|%s|%s|%d", turnID, pipelineVersion, graphRef, profile, epoch)
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
