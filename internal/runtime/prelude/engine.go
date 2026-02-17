package prelude

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/identity"
)

// Input captures session-scoped turn-intent proposal data.
type Input struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	Reason               string
}

// Proposal is a non-authoritative turn-open intent.
type Proposal struct {
	Identity identity.Context
	Signal   eventabi.ControlSignal
}

// Engine issues deterministic non-authoritative turn-open intent signals.
type Engine struct {
	identity *identity.Service
}

// NewEngine returns a deterministic prelude engine.
func NewEngine() Engine {
	return Engine{
		identity: identity.NewService(),
	}
}

// ProposeTurn creates a turn_open_proposed signal for arbiter input.
func (e Engine) ProposeTurn(in Input) (Proposal, error) {
	if in.SessionID == "" || in.TurnID == "" {
		return Proposal{}, fmt.Errorf("session_id and turn_id are required")
	}
	if in.PipelineVersion == "" {
		in.PipelineVersion = "pipeline-v1"
	}
	ctx, err := e.identity.NewEventContext(in.SessionID, in.TurnID)
	if err != nil {
		return Proposal{}, err
	}
	if in.EventID != "" {
		ctx.EventID = in.EventID
		if err := identity.ValidateContext(ctx); err != nil {
			return Proposal{}, err
		}
	}

	reason := in.Reason
	if reason == "" {
		reason = "session_turn_intent"
	}
	transport := nonNegative(in.TransportSequence)
	signal := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeSession,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		EventID:            ctx.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transport,
		RuntimeSequence:    nonNegative(in.RuntimeSequence),
		AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
		WallClockMS:        nonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "turn_open_proposed",
		EmittedBy:          "RK-02",
		Reason:             reason,
		Scope:              "turn",
	}
	normalized, err := runtimeeventabi.ValidateAndNormalizeControlSignals([]eventabi.ControlSignal{signal})
	if err != nil {
		return Proposal{}, err
	}

	return Proposal{
		Identity: ctx,
		Signal:   normalized[0],
	}, nil
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
