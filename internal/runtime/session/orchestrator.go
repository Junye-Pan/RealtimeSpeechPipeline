package session

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

// Orchestrator runs turnarbiter decisions under session-owned lifecycle state.
type Orchestrator struct {
	manager *Manager
	arbiter turnarbiter.Arbiter
}

// NewOrchestrator constructs a session lifecycle orchestrator for one session.
func NewOrchestrator(sessionID string, arbiter turnarbiter.Arbiter) (*Orchestrator, error) {
	manager, err := NewManager(sessionID)
	if err != nil {
		return nil, err
	}
	if arbiter == (turnarbiter.Arbiter{}) {
		arbiter = turnarbiter.New()
	}
	return &Orchestrator{
		manager: manager,
		arbiter: arbiter,
	}, nil
}

// SessionID returns the owned session identifier.
func (o *Orchestrator) SessionID() string {
	return o.manager.SessionID()
}

// State returns known turn state from the session-owned registry.
func (o *Orchestrator) State(turnID string) (controlplane.TurnState, bool) {
	return o.manager.State(turnID)
}

// HandleTurnOpen runs pre-turn arbiter logic under session-owned lifecycle transitions.
func (o *Orchestrator) HandleTurnOpen(in turnarbiter.OpenRequest) (turnarbiter.OpenResult, error) {
	if err := o.validateSession(in.SessionID); err != nil {
		return turnarbiter.OpenResult{}, err
	}

	if _, err := o.manager.ProposeTurn(in.TurnID); err != nil {
		return turnarbiter.OpenResult{}, err
	}

	out, err := o.arbiter.HandleTurnOpenProposed(in)
	if err != nil {
		// Keep session lifecycle recoverable on open-path errors.
		_, _ = o.manager.RejectTurn(in.TurnID, controlplane.TriggerDefer)
		return turnarbiter.OpenResult{}, err
	}

	switch out.State {
	case controlplane.TurnActive:
		if _, err := o.manager.OpenTurn(in.TurnID); err != nil {
			return turnarbiter.OpenResult{}, err
		}
	case controlplane.TurnIdle:
		trigger, err := openIdleTrigger(out.Decision)
		if err != nil {
			return turnarbiter.OpenResult{}, err
		}
		if _, err := o.manager.RejectTurn(in.TurnID, trigger); err != nil {
			return turnarbiter.OpenResult{}, err
		}
	default:
		return turnarbiter.OpenResult{}, fmt.Errorf("unexpected open result state %s", out.State)
	}

	return out, nil
}

// HandleActive runs active-turn arbiter logic under session-owned terminal sequencing.
func (o *Orchestrator) HandleActive(in turnarbiter.ActiveInput) (turnarbiter.ActiveResult, error) {
	if err := o.validateSession(in.SessionID); err != nil {
		return turnarbiter.ActiveResult{}, err
	}

	out, err := o.arbiter.HandleActive(in)
	if err != nil {
		return turnarbiter.ActiveResult{}, err
	}

	if out.State != controlplane.TurnClosed {
		return out, nil
	}

	terminalTrigger, err := activeTerminalTrigger(out)
	if err != nil {
		return turnarbiter.ActiveResult{}, err
	}
	switch terminalTrigger {
	case controlplane.TriggerCommit:
		if _, err := o.manager.CommitTurn(in.TurnID); err != nil {
			return turnarbiter.ActiveResult{}, err
		}
	case controlplane.TriggerAbort:
		if _, err := o.manager.AbortTurn(in.TurnID); err != nil {
			return turnarbiter.ActiveResult{}, err
		}
	default:
		return turnarbiter.ActiveResult{}, fmt.Errorf("unsupported terminal trigger %s", terminalTrigger)
	}
	if _, err := o.manager.CloseTurn(in.TurnID); err != nil {
		return turnarbiter.ActiveResult{}, err
	}

	return out, nil
}

func (o *Orchestrator) validateSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if sessionID != o.manager.SessionID() {
		return fmt.Errorf("session_id %s does not match orchestrator session %s", sessionID, o.manager.SessionID())
	}
	return nil
}

func openIdleTrigger(decision *controlplane.DecisionOutcome) (controlplane.TransitionTrigger, error) {
	if decision == nil {
		return "", fmt.Errorf("open result in Idle state must include decision outcome")
	}
	switch decision.OutcomeKind {
	case controlplane.OutcomeReject:
		return controlplane.TriggerReject, nil
	case controlplane.OutcomeDefer:
		return controlplane.TriggerDefer, nil
	case controlplane.OutcomeStaleEpochReject:
		return controlplane.TriggerStaleEpochReject, nil
	case controlplane.OutcomeDeauthorized:
		return controlplane.TriggerDeauthorized, nil
	default:
		return "", fmt.Errorf("unsupported idle decision outcome %s", decision.OutcomeKind)
	}
}

func activeTerminalTrigger(out turnarbiter.ActiveResult) (controlplane.TransitionTrigger, error) {
	for _, tr := range out.Transitions {
		if tr.FromState != controlplane.TurnActive || tr.ToState != controlplane.TurnTerminal {
			continue
		}
		if tr.Trigger == controlplane.TriggerCommit || tr.Trigger == controlplane.TriggerAbort {
			return tr.Trigger, nil
		}
	}
	return "", fmt.Errorf("closed active result missing terminal transition")
}
