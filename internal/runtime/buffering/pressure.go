package buffering

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
)

// PressureInput defines deterministic F4 edge-pressure context.
type PressureInput struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EdgeID               string
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	TargetLane           eventabi.Lane
	DropRangeStart       int64
	DropRangeEnd         int64
	HighWatermark        bool
	EmitRecovery         bool
	Shed                 bool
}

// PressureResult exposes deterministic flow/drop/shed outputs.
type PressureResult struct {
	Signals     []eventabi.ControlSignal
	ShedOutcome *controlplane.DecisionOutcome
}

// HandleEdgePressure evaluates edge-pressure behavior for F4.
func HandleEdgePressure(admission localadmission.Evaluator, in PressureInput) (PressureResult, error) {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EdgeID == "" || in.EventID == "" {
		return PressureResult{}, fmt.Errorf("session_id, pipeline_version, edge_id, and event_id are required")
	}
	if in.TargetLane == "" {
		return PressureResult{}, fmt.Errorf("target_lane is required")
	}

	result := PressureResult{}
	if in.HighWatermark {
		watermark, err := buildEdgeSignal(in, "watermark", "RK-13", "edge_watermark_high")
		if err != nil {
			return PressureResult{}, err
		}
		xoff, err := buildEdgeSignal(in, "flow_xoff", "RK-14", "backpressure_asserted")
		if err != nil {
			return PressureResult{}, err
		}
		drop, err := BuildDropNotice(DropNoticeInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			PipelineVersion:      in.PipelineVersion,
			EdgeID:               in.EdgeID,
			EventID:              in.EventID,
			TransportSequence:    in.TransportSequence,
			RuntimeSequence:      in.RuntimeSequence,
			AuthorityEpoch:       in.AuthorityEpoch,
			RuntimeTimestampMS:   in.RuntimeTimestampMS,
			WallClockTimestampMS: in.WallClockTimestampMS,
			TargetLane:           in.TargetLane,
			RangeStart:           in.DropRangeStart,
			RangeEnd:             in.DropRangeEnd,
			Reason:               "queue_pressure_drop",
			EmittedBy:            "RK-12",
		})
		if err != nil {
			return PressureResult{}, err
		}
		result.Signals = append(result.Signals, watermark, xoff, drop)
	}

	if in.EmitRecovery {
		xon, err := buildEdgeSignal(in, "flow_xon", "RK-14", "")
		if err != nil {
			return PressureResult{}, err
		}
		result.Signals = append(result.Signals, xon)
	}

	if in.Shed {
		out := admission.EvaluateSchedulingPoint(localadmission.SchedulingPointInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			EventID:              in.EventID,
			RuntimeTimestampMS:   in.RuntimeTimestampMS,
			WallClockTimestampMS: in.WallClockTimestampMS,
			Scope:                controlplane.ScopeEdgeEnqueue,
			Shed:                 true,
			Reason:               "scheduling_point_shed",
		})
		if out.Outcome != nil {
			if err := out.Outcome.Validate(); err != nil {
				return PressureResult{}, err
			}
			result.ShedOutcome = out.Outcome
		}
	}

	return result, nil
}

// SyncLossInput defines deterministic F5 sync-domain loss handling.
type SyncLossInput struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	SyncDomain           string
	DiscontinuityID      string
	UseAtomicDrop        bool
	EdgeID               string
	TargetLane           eventabi.Lane
	DropRangeStart       int64
	DropRangeEnd         int64
}

// HandleSyncLoss emits either discontinuity or deterministic atomic-drop marker.
func HandleSyncLoss(in SyncLossInput) (eventabi.ControlSignal, error) {
	if in.UseAtomicDrop {
		return BuildDropNotice(DropNoticeInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			PipelineVersion:      in.PipelineVersion,
			EdgeID:               in.EdgeID,
			EventID:              in.EventID,
			TransportSequence:    in.TransportSequence,
			RuntimeSequence:      in.RuntimeSequence,
			AuthorityEpoch:       in.AuthorityEpoch,
			RuntimeTimestampMS:   in.RuntimeTimestampMS,
			WallClockTimestampMS: in.WallClockTimestampMS,
			TargetLane:           in.TargetLane,
			RangeStart:           in.DropRangeStart,
			RangeEnd:             in.DropRangeEnd,
			Reason:               "sync_atomic_drop",
			EmittedBy:            "RK-12",
		})
	}

	if in.SessionID == "" || in.PipelineVersion == "" || in.EventID == "" {
		return eventabi.ControlSignal{}, fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	if in.SyncDomain == "" || in.DiscontinuityID == "" {
		return eventabi.ControlSignal{}, fmt.Errorf("sync_domain and discontinuity_id are required")
	}

	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}

	transport := safeNonNegative(in.TransportSequence)
	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		SyncDomain:         in.SyncDomain,
		DiscontinuityID:    in.DiscontinuityID,
		EventID:            in.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transport,
		RuntimeSequence:    safeNonNegative(in.RuntimeSequence),
		AuthorityEpoch:     safeNonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: safeNonNegative(in.RuntimeTimestampMS),
		WallClockMS:        safeNonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "discontinuity",
		EmittedBy:          "RK-15",
		Reason:             "sync_domain_partial_loss",
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}

func buildEdgeSignal(in PressureInput, signal, emitter, reason string) (eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transport := safeNonNegative(in.TransportSequence)

	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		EdgeID:             in.EdgeID,
		EventID:            in.EventID,
		Lane:               eventabi.LaneControl,
		TargetLane:         in.TargetLane,
		TransportSequence:  &transport,
		RuntimeSequence:    safeNonNegative(in.RuntimeSequence),
		AuthorityEpoch:     safeNonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: safeNonNegative(in.RuntimeTimestampMS),
		WallClockMS:        safeNonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             signal,
		EmittedBy:          emitter,
		Reason:             reason,
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}

func safeNonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
