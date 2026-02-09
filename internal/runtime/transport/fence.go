package transport

import (
	"fmt"
	"sync"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
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
	mu       sync.Mutex
	canceled map[string]bool
}

// NewOutputFence creates a new deterministic output fence.
func NewOutputFence() *OutputFence {
	return &OutputFence{canceled: make(map[string]bool)}
}

// EvaluateOutput applies cancel fencing and emits deterministic control markers.
func (f *OutputFence) EvaluateOutput(in OutputAttempt) (OutputDecision, error) {
	if in.SessionID == "" || in.TurnID == "" || in.PipelineVersion == "" || in.EventID == "" {
		return OutputDecision{}, fmt.Errorf("session_id, turn_id, pipeline_version, and event_id are required")
	}

	key := in.SessionID + "/" + in.TurnID
	f.mu.Lock()
	if in.CancelAccepted {
		f.canceled[key] = true
	}
	canceled := f.canceled[key]
	f.mu.Unlock()

	signalName := "output_accepted"
	reason := "output_accepted"
	accepted := true
	if canceled {
		signalName = "playback_cancelled"
		reason = "cancel_fence_applied"
		accepted = false
	}

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

	return OutputDecision{Accepted: accepted, Signal: signal}, nil
}

func pointer(v int64) *int64 {
	return &v
}
