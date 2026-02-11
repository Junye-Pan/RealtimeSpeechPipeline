package transport

import (
	"fmt"
	"strconv"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/cancellation"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
)

// OutputAttempt captures an egress output decision point.
type OutputAttempt struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	CancelAccepted       bool
}

// OutputDecision reports whether output was accepted or fenced.
type OutputDecision struct {
	Accepted bool
	Signal   eventabi.ControlSignal
}

// OutputFence applies deterministic post-cancel egress fencing.
type OutputFence struct {
	turnFence turnFence
}

type turnFence interface {
	Accept(sessionID, turnID string) error
	IsFenced(sessionID, turnID string) bool
}

// NewOutputFence creates a new deterministic output fence.
func NewOutputFence() *OutputFence {
	return NewOutputFenceWithTurnFence(cancellation.NewFence())
}

// NewOutputFenceWithTurnFence creates an output fence with an explicit cancel fence dependency.
func NewOutputFenceWithTurnFence(fence turnFence) *OutputFence {
	if fence == nil {
		fence = cancellation.NewFence()
	}
	return &OutputFence{turnFence: fence}
}

// EvaluateOutput applies cancel fencing and emits deterministic control markers.
func (f *OutputFence) EvaluateOutput(in OutputAttempt) (OutputDecision, error) {
	if in.SessionID == "" || in.TurnID == "" || in.PipelineVersion == "" || in.EventID == "" {
		return OutputDecision{}, fmt.Errorf("session_id, turn_id, pipeline_version, and event_id are required")
	}
	correlation := telemetry.Correlation{
		SessionID:            in.SessionID,
		TurnID:               in.TurnID,
		EventID:              in.EventID,
		PipelineVersion:      in.PipelineVersion,
		AuthorityEpoch:       safeNonNegativeTransport(in.AuthorityEpoch),
		Lane:                 string(eventabi.LaneTelemetry),
		EmittedBy:            "OR-01",
		RuntimeTimestampMS:   safeNonNegativeTransport(in.RuntimeTimestampMS),
		WallClockTimestampMS: safeNonNegativeTransport(in.WallClockTimestampMS),
	}

	if in.CancelAccepted {
		if err := f.turnFence.Accept(in.SessionID, in.TurnID); err != nil {
			return OutputDecision{}, err
		}
	}
	canceled := f.turnFence.IsFenced(in.SessionID, in.TurnID)

	signalName := "output_accepted"
	reason := "output_accepted"
	accepted := true
	if canceled {
		signalName = "playback_cancelled"
		reason = "cancel_fence_applied"
		accepted = false
	}
	logSeverity := "info"
	if !accepted {
		logSeverity = "warn"
	}
	if in.CancelAccepted {
		telemetry.DefaultEmitter().EmitMetric(
			telemetry.MetricCancelLatencyMS,
			0,
			"ms",
			map[string]string{
				"accepted": strconv.FormatBool(accepted),
				"scope":    "turn",
			},
			correlation,
		)
	}
	telemetry.DefaultEmitter().EmitLog(
		"output_fence_decision",
		logSeverity,
		"output fence decision emitted",
		map[string]string{
			"accepted": strconv.FormatBool(accepted),
			"signal":   signalName,
			"reason":   reason,
		},
		correlation,
	)
	telemetry.DefaultEmitter().EmitSpan(
		"node_span",
		"node_span",
		safeNonNegativeTransport(in.RuntimeTimestampMS),
		safeNonNegativeTransport(in.RuntimeTimestampMS)+1,
		map[string]string{
			"node_type": "transport_fence",
			"accepted":  strconv.FormatBool(accepted),
		},
		correlation,
	)

	signal := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeTurn,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		EventID:            in.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  pointer(safeNonNegativeTransport(in.TransportSequence)),
		RuntimeSequence:    safeNonNegativeTransport(in.RuntimeSequence),
		AuthorityEpoch:     safeNonNegativeTransport(in.AuthorityEpoch),
		RuntimeTimestampMS: safeNonNegativeTransport(in.RuntimeTimestampMS),
		WallClockMS:        safeNonNegativeTransport(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             signalName,
		EmittedBy:          "RK-22",
		Reason:             reason,
		Scope:              "turn",
	}
	if err := signal.Validate(); err != nil {
		return OutputDecision{}, err
	}
	normalized, err := runtimeeventabi.ValidateAndNormalizeControlSignals([]eventabi.ControlSignal{signal})
	if err != nil {
		return OutputDecision{}, err
	}
	signal = normalized[0]

	return OutputDecision{Accepted: accepted, Signal: signal}, nil
}

func pointer(v int64) *int64 {
	return &v
}
