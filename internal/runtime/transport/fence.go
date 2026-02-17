package transport

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	telemetrycontext "github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry/context"
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
	authority authorityValidator
}

type turnFence interface {
	Accept(sessionID, turnID string) error
	IsFenced(sessionID, turnID string) bool
}

type authorityValidator interface {
	ValidateAuthority(sessionID string, authorityEpoch int64) error
}

type staleAuthorityError interface {
	StaleAuthority() bool
}

// NewOutputFence creates a new deterministic output fence.
func NewOutputFence() *OutputFence {
	return NewOutputFenceWithDependencies(cancellation.NewFence(), nil)
}

// NewOutputFenceWithTurnFence creates an output fence with an explicit cancel fence dependency.
func NewOutputFenceWithTurnFence(fence turnFence) *OutputFence {
	return NewOutputFenceWithDependencies(fence, nil)
}

// NewOutputFenceWithDependencies creates an output fence with explicit cancel and authority dependencies.
func NewOutputFenceWithDependencies(fence turnFence, authority authorityValidator) *OutputFence {
	if fence == nil {
		fence = cancellation.NewFence()
	}
	return &OutputFence{turnFence: fence, authority: authority}
}

// EvaluateOutput applies cancel fencing and emits deterministic control markers.
func (f *OutputFence) EvaluateOutput(in OutputAttempt) (OutputDecision, error) {
	if in.SessionID == "" || in.TurnID == "" || in.PipelineVersion == "" || in.EventID == "" {
		return OutputDecision{}, fmt.Errorf("session_id, turn_id, pipeline_version, and event_id are required")
	}
	correlation, err := telemetrycontext.Resolve(telemetrycontext.ResolveInput{
		SessionID:            in.SessionID,
		TurnID:               in.TurnID,
		EventID:              in.EventID,
		PipelineVersion:      in.PipelineVersion,
		NodeID:               "transport_fence",
		EdgeID:               "transport/egress",
		AuthorityEpoch:       safeNonNegativeTransport(in.AuthorityEpoch),
		Lane:                 eventabi.LaneTelemetry,
		EmittedBy:            "OR-01",
		RuntimeTimestampMS:   safeNonNegativeTransport(in.RuntimeTimestampMS),
		WallClockTimestampMS: safeNonNegativeTransport(in.WallClockTimestampMS),
	})
	if err != nil {
		return OutputDecision{}, err
	}

	if f.authority != nil {
		if err := f.authority.ValidateAuthority(in.SessionID, in.AuthorityEpoch); err != nil {
			reason := "authority_validation_failed"
			if isStaleAuthorityError(err) {
				reason = "authority_epoch_mismatch"
			}
			signal, signalErr := buildOutputSignal(in, "stale_epoch_reject", "RK-24", reason)
			if signalErr != nil {
				return OutputDecision{}, signalErr
			}
			telemetry.DefaultEmitter().EmitLog(
				"output_fence_decision",
				"warn",
				"output rejected due to authority validation failure",
				map[string]string{
					"accepted": "false",
					"signal":   "stale_epoch_reject",
					"reason":   reason,
					"node_id":  "transport_fence",
					"edge_id":  "transport/egress",
				},
				correlation,
			)
			return OutputDecision{Accepted: false, Signal: signal}, nil
		}
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
				"node_id":  "transport_fence",
				"edge_id":  "transport/egress",
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
			"node_id":  "transport_fence",
			"edge_id":  "transport/egress",
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

	signal, err := buildOutputSignal(in, signalName, "RK-22", reason)
	if err != nil {
		return OutputDecision{}, err
	}

	return OutputDecision{Accepted: accepted, Signal: signal}, nil
}

func buildOutputSignal(in OutputAttempt, signalName, emittedBy, reason string) (eventabi.ControlSignal, error) {
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
		EmittedBy:          emittedBy,
		Reason:             reason,
		Scope:              "turn",
	}
	if err := signal.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	normalized, err := runtimeeventabi.ValidateAndNormalizeControlSignals([]eventabi.ControlSignal{signal})
	if err != nil {
		return eventabi.ControlSignal{}, err
	}
	return normalized[0], nil
}

func isStaleAuthorityError(err error) bool {
	var staleErr staleAuthorityError
	return errors.As(err, &staleErr) && staleErr.StaleAuthority()
}

func pointer(v int64) *int64 {
	return &v
}
