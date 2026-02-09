package buffering

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// DropNoticeInput defines deterministic metadata for ML-001 drop signaling.
type DropNoticeInput struct {
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
	RangeStart           int64
	RangeEnd             int64
	Reason               string
	EmittedBy            string
}

// BuildDropNotice constructs a validated drop_notice control signal.
func BuildDropNotice(in DropNoticeInput) (eventabi.ControlSignal, error) {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EdgeID == "" || in.EventID == "" {
		return eventabi.ControlSignal{}, fmt.Errorf("session_id, pipeline_version, edge_id, and event_id are required")
	}
	if in.TargetLane == "" {
		return eventabi.ControlSignal{}, fmt.Errorf("target_lane is required")
	}

	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}

	reason := in.Reason
	if reason == "" {
		reason = "queue_pressure_drop"
	}

	emittedBy := in.EmittedBy
	if emittedBy == "" {
		emittedBy = "RK-12"
	}

	rangeValue := eventabi.SeqRange{Start: in.RangeStart, End: in.RangeEnd}
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
		TransportSequence:  valueOrZero(in.TransportSequence),
		RuntimeSequence:    nonNegative(in.RuntimeSequence),
		AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
		WallClockMS:        nonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "drop_notice",
		EmittedBy:          emittedBy,
		Reason:             reason,
		SeqRange:           &rangeValue,
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func valueOrZero(v int64) *int64 {
	n := nonNegative(v)
	return &n
}
