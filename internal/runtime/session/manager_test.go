package session

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

func TestManagerLifecycleHappyPath(t *testing.T) {
	t.Parallel()

	m, err := NewManager("sess-1")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	tr, err := m.ProposeTurn("turn-1")
	if err != nil || tr.Trigger != controlplane.TriggerTurnOpenProposed {
		t.Fatalf("propose turn failed: tr=%+v err=%v", tr, err)
	}
	tr, err = m.OpenTurn("turn-1")
	if err != nil || tr.Trigger != controlplane.TriggerTurnOpen {
		t.Fatalf("open turn failed: tr=%+v err=%v", tr, err)
	}
	tr, err = m.CommitTurn("turn-1")
	if err != nil || tr.Trigger != controlplane.TriggerCommit {
		t.Fatalf("commit turn failed: tr=%+v err=%v", tr, err)
	}
	tr, err = m.CloseTurn("turn-1")
	if err != nil || tr.Trigger != controlplane.TriggerClose {
		t.Fatalf("close turn failed: tr=%+v err=%v", tr, err)
	}

	state, ok := m.State("turn-1")
	if !ok || state != controlplane.TurnClosed {
		t.Fatalf("expected closed state, got state=%s ok=%t", state, ok)
	}
}

func TestManagerRejectFromOpening(t *testing.T) {
	t.Parallel()

	m, err := NewManager("sess-2")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := m.ProposeTurn("turn-2"); err != nil {
		t.Fatalf("propose turn failed: %v", err)
	}
	tr, err := m.RejectTurn("turn-2", controlplane.TriggerReject)
	if err != nil || tr.ToState != controlplane.TurnIdle {
		t.Fatalf("reject turn failed: tr=%+v err=%v", tr, err)
	}
	state, ok := m.State("turn-2")
	if !ok || state != controlplane.TurnIdle {
		t.Fatalf("expected idle state after reject, got state=%s ok=%t", state, ok)
	}
}

func TestManagerRejectInvalidTransitions(t *testing.T) {
	t.Parallel()

	m, err := NewManager("sess-3")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if _, err := m.OpenTurn("turn-3"); err == nil {
		t.Fatalf("expected opening unknown turn to fail")
	}
	if _, err := m.ProposeTurn("turn-3"); err != nil {
		t.Fatalf("propose turn failed: %v", err)
	}
	if _, err := m.CommitTurn("turn-3"); err == nil {
		t.Fatalf("expected commit from opening to fail")
	}
	if _, err := m.RejectTurn("turn-3", controlplane.TriggerCommit); err == nil {
		t.Fatalf("expected invalid reject trigger to fail")
	}
}

func TestManagerSingleActiveTurn(t *testing.T) {
	t.Parallel()

	m, err := NewManager("sess-4")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := m.ProposeTurn("turn-a"); err != nil {
		t.Fatalf("propose turn-a failed: %v", err)
	}
	if _, err := m.OpenTurn("turn-a"); err != nil {
		t.Fatalf("open turn-a failed: %v", err)
	}
	if _, err := m.ProposeTurn("turn-b"); err == nil {
		t.Fatalf("expected proposing turn-b while turn-a active to fail")
	}
	if _, err := m.AbortTurn("turn-a"); err != nil {
		t.Fatalf("abort turn-a failed: %v", err)
	}
	if _, err := m.CloseTurn("turn-a"); err != nil {
		t.Fatalf("close turn-a failed: %v", err)
	}
	if _, err := m.ProposeTurn("turn-b"); err != nil {
		t.Fatalf("expected turn-b proposal after close to succeed, got %v", err)
	}
}
