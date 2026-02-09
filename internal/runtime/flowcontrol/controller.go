package flowcontrol

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
)

// Mode controls flow-control output semantics.
type Mode string

const (
	ModeSignal Mode = "signal"
	ModeCredit Mode = "credit"
)

// Input captures deterministic flow-control decision context.
type Input struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EdgeID               string
	EventID              string
	TargetLane           eventabi.Lane
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	HighWatermark        bool
	EmitRecovery         bool
	Mode                 Mode
	CreditAmount         int64
}

// Controller emits deterministic RK-14 flow-control signals.
type Controller struct{}

// NewController returns a deterministic flow-control controller.
func NewController() Controller {
	return Controller{}
}

// Evaluate emits flow control signals based on watermark/recovery transitions.
func (Controller) Evaluate(in Input) ([]eventabi.ControlSignal, error) {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EdgeID == "" || in.EventID == "" {
		return nil, fmt.Errorf("session_id, pipeline_version, edge_id, and event_id are required")
	}
	if in.TargetLane == "" {
		return nil, fmt.Errorf("target_lane is required")
	}
	if in.Mode == "" {
		in.Mode = ModeSignal
	}
	if in.Mode != ModeSignal && in.Mode != ModeCredit {
		return nil, fmt.Errorf("unsupported flow control mode: %q", in.Mode)
	}

	signals := make([]eventabi.ControlSignal, 0, 2)
	if in.HighWatermark {
		xoff, err := buildSignal(in, "flow_xoff", "backpressure_asserted", nil)
		if err != nil {
			return nil, err
		}
		signals = append(signals, xoff)
	}

	if in.EmitRecovery {
		if in.Mode == ModeCredit {
			amount := in.CreditAmount
			if amount < 1 {
				amount = 1
			}
			grant, err := buildSignal(in, "credit_grant", "", &amount)
			if err != nil {
				return nil, err
			}
			signals = append(signals, grant)
		} else {
			xon, err := buildSignal(in, "flow_xon", "", nil)
			if err != nil {
				return nil, err
			}
			signals = append(signals, xon)
		}
	}

	if len(signals) == 0 {
		return nil, nil
	}
	normalized, err := runtimeeventabi.ValidateAndNormalizeControlSignals(signals)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func buildSignal(in Input, signal string, reason string, amount *int64) (eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transport := nonNegative(in.TransportSequence)
	control := eventabi.ControlSignal{
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
		RuntimeSequence:    nonNegative(in.RuntimeSequence),
		AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
		WallClockMS:        nonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             signal,
		EmittedBy:          "RK-14",
		Reason:             reason,
		Scope:              scope,
		Amount:             amount,
	}
	if err := control.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return control, nil
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
