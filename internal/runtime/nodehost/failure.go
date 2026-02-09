package nodehost

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// NodeFailureInput defines deterministic F2 failure-shaping input.
type NodeFailureInput struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	AllowDegrade         bool
	AllowFallback        bool
}

// NodeFailureResult includes control-lane emissions and terminal recommendation.
type NodeFailureResult struct {
	Signals        []eventabi.ControlSignal
	Terminal       bool
	TerminalReason string
}

// HandleFailure returns deterministic node-timeout/failure handling artifacts.
func HandleFailure(in NodeFailureInput) (NodeFailureResult, error) {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EventID == "" {
		return NodeFailureResult{}, fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}

	warning, err := buildBudgetSignal(in, "budget_warning", "node_budget_threshold_exceeded")
	if err != nil {
		return NodeFailureResult{}, err
	}
	exhausted, err := buildBudgetSignal(in, "budget_exhausted", "node_timeout_or_failure")
	if err != nil {
		return NodeFailureResult{}, err
	}

	result := NodeFailureResult{
		Signals: []eventabi.ControlSignal{warning, exhausted},
	}

	if in.AllowDegrade {
		degrade, err := buildDecisionSignal(in, "degrade", "node_timeout_or_failure")
		if err != nil {
			return NodeFailureResult{}, err
		}
		result.Signals = append(result.Signals, degrade)
		return result, nil
	}
	if in.AllowFallback {
		fallback, err := buildDecisionSignal(in, "fallback", "node_timeout_or_failure")
		if err != nil {
			return NodeFailureResult{}, err
		}
		result.Signals = append(result.Signals, fallback)
		return result, nil
	}

	result.Terminal = true
	result.TerminalReason = "node_timeout_or_failure"
	return result, nil
}

func buildBudgetSignal(in NodeFailureInput, signal, reason string) (eventabi.ControlSignal, error) {
	return buildSignal(in, signal, "RK-17", reason)
}

func buildDecisionSignal(in NodeFailureInput, signal, reason string) (eventabi.ControlSignal, error) {
	return buildSignal(in, signal, "RK-17", reason)
}

func buildSignal(in NodeFailureInput, signal, emitter, reason string) (eventabi.ControlSignal, error) {
	scope := eventabi.ScopeSession
	scopeValue := "session"
	if in.TurnID != "" {
		scope = eventabi.ScopeTurn
		scopeValue = "turn"
	}
	transport := nonNegative(in.TransportSequence)

	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         scope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		EventID:            in.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transport,
		RuntimeSequence:    nonNegative(in.RuntimeSequence),
		AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
		WallClockMS:        nonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             signal,
		EmittedBy:          emitter,
		Reason:             reason,
		Scope:              scopeValue,
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
