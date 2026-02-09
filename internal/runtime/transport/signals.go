package transport

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// ConnectionSignalInput defines deterministic transport lifecycle signal context.
type ConnectionSignalInput struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	Signal               string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	Reason               string
}

// BuildConnectionSignal creates validated transport-state control signals.
func BuildConnectionSignal(in ConnectionSignalInput) (eventabi.ControlSignal, error) {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EventID == "" || in.Signal == "" {
		return eventabi.ControlSignal{}, fmt.Errorf("session_id, pipeline_version, event_id, and signal are required")
	}

	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}

	transport := safeNonNegativeTransport(in.TransportSequence)
	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		EventID:            in.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transport,
		RuntimeSequence:    safeNonNegativeTransport(in.RuntimeSequence),
		AuthorityEpoch:     safeNonNegativeTransport(in.AuthorityEpoch),
		RuntimeTimestampMS: safeNonNegativeTransport(in.RuntimeTimestampMS),
		WallClockMS:        safeNonNegativeTransport(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             in.Signal,
		EmittedBy:          "RK-23",
		Reason:             in.Reason,
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}

func safeNonNegativeTransport(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
