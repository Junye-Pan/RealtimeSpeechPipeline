package session

import (
	"fmt"
	"sync"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

// Manager tracks deterministic turn lifecycle state within a session.
type Manager struct {
	sessionID string

	mu       sync.Mutex
	turns    map[string]controlplane.TurnState
	activeID string
}

// NewManager constructs an empty session lifecycle manager.
func NewManager(sessionID string) (*Manager, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	return &Manager{
		sessionID: sessionID,
		turns:     map[string]controlplane.TurnState{},
	}, nil
}

// SessionID returns manager scope identity.
func (m *Manager) SessionID() string {
	return m.sessionID
}

// ProposeTurn transitions Idle -> Opening for a turn.
func (m *Manager) ProposeTurn(turnID string) (controlplane.TurnTransition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if turnID == "" {
		return controlplane.TurnTransition{}, fmt.Errorf("turn_id is required")
	}
	if m.activeID != "" {
		return controlplane.TurnTransition{}, fmt.Errorf("cannot propose turn %s while turn %s is active", turnID, m.activeID)
	}
	if state, ok := m.turns[turnID]; ok && state != controlplane.TurnIdle {
		return controlplane.TurnTransition{}, fmt.Errorf("turn %s is not idle: %s", turnID, state)
	}

	tr := controlplane.TurnTransition{
		FromState:     controlplane.TurnIdle,
		Trigger:       controlplane.TriggerTurnOpenProposed,
		ToState:       controlplane.TurnOpening,
		Deterministic: true,
	}
	if err := tr.Validate(); err != nil {
		return controlplane.TurnTransition{}, err
	}
	m.turns[turnID] = controlplane.TurnOpening
	return tr, nil
}

// OpenTurn transitions Opening -> Active.
func (m *Manager) OpenTurn(turnID string) (controlplane.TurnTransition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.requireState(turnID, controlplane.TurnOpening); err != nil {
		return controlplane.TurnTransition{}, err
	}
	tr := controlplane.TurnTransition{
		FromState:     controlplane.TurnOpening,
		Trigger:       controlplane.TriggerTurnOpen,
		ToState:       controlplane.TurnActive,
		Deterministic: true,
	}
	if err := tr.Validate(); err != nil {
		return controlplane.TurnTransition{}, err
	}
	m.turns[turnID] = controlplane.TurnActive
	m.activeID = turnID
	return tr, nil
}

// RejectTurn transitions Opening -> Idle for pre-turn rejection/defer/authority outcomes.
func (m *Manager) RejectTurn(turnID string, trigger controlplane.TransitionTrigger) (controlplane.TurnTransition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.requireState(turnID, controlplane.TurnOpening); err != nil {
		return controlplane.TurnTransition{}, err
	}
	if trigger != controlplane.TriggerReject && trigger != controlplane.TriggerDefer &&
		trigger != controlplane.TriggerStaleEpochReject && trigger != controlplane.TriggerDeauthorized {
		return controlplane.TurnTransition{}, fmt.Errorf("invalid opening->idle trigger: %s", trigger)
	}

	tr := controlplane.TurnTransition{
		FromState:     controlplane.TurnOpening,
		Trigger:       trigger,
		ToState:       controlplane.TurnIdle,
		Deterministic: true,
	}
	if err := tr.Validate(); err != nil {
		return controlplane.TurnTransition{}, err
	}
	m.turns[turnID] = controlplane.TurnIdle
	return tr, nil
}

// CommitTurn transitions Active -> Terminal via commit.
func (m *Manager) CommitTurn(turnID string) (controlplane.TurnTransition, error) {
	return m.terminalize(turnID, controlplane.TriggerCommit)
}

// AbortTurn transitions Active -> Terminal via abort.
func (m *Manager) AbortTurn(turnID string) (controlplane.TurnTransition, error) {
	return m.terminalize(turnID, controlplane.TriggerAbort)
}

// CloseTurn transitions Terminal -> Closed.
func (m *Manager) CloseTurn(turnID string) (controlplane.TurnTransition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.requireState(turnID, controlplane.TurnTerminal); err != nil {
		return controlplane.TurnTransition{}, err
	}
	tr := controlplane.TurnTransition{
		FromState:     controlplane.TurnTerminal,
		Trigger:       controlplane.TriggerClose,
		ToState:       controlplane.TurnClosed,
		Deterministic: true,
	}
	if err := tr.Validate(); err != nil {
		return controlplane.TurnTransition{}, err
	}
	m.turns[turnID] = controlplane.TurnClosed
	if m.activeID == turnID {
		m.activeID = ""
	}
	return tr, nil
}

// State returns the current turn state for a known turn.
func (m *Manager) State(turnID string) (controlplane.TurnState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.turns[turnID]
	return state, ok
}

func (m *Manager) terminalize(turnID string, trigger controlplane.TransitionTrigger) (controlplane.TurnTransition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.requireState(turnID, controlplane.TurnActive); err != nil {
		return controlplane.TurnTransition{}, err
	}
	tr := controlplane.TurnTransition{
		FromState:     controlplane.TurnActive,
		Trigger:       trigger,
		ToState:       controlplane.TurnTerminal,
		Deterministic: true,
	}
	if err := tr.Validate(); err != nil {
		return controlplane.TurnTransition{}, err
	}
	m.turns[turnID] = controlplane.TurnTerminal
	return tr, nil
}

func (m *Manager) requireState(turnID string, expected controlplane.TurnState) error {
	if turnID == "" {
		return fmt.Errorf("turn_id is required")
	}
	state, ok := m.turns[turnID]
	if !ok {
		return fmt.Errorf("turn %s is unknown", turnID)
	}
	if state != expected {
		return fmt.Errorf("turn %s expected state %s, got %s", turnID, expected, state)
	}
	return nil
}
