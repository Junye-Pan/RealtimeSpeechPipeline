package sessionfsm

import (
	"testing"
	"time"

	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
)

func TestFSMTransitionsAndTerminalCleanup(t *testing.T) {
	t.Parallel()

	base := time.Unix(100, 0)
	fsm := New(Config{
		DisconnectTimeout: 10 * time.Second,
		Now:               func() time.Time { return base },
	})

	state, err := fsm.Transition(apitransport.SignalConnected)
	if err != nil {
		t.Fatalf("connected transition: %v", err)
	}
	if state != StateConnected {
		t.Fatalf("expected connected state, got %s", state)
	}

	state, err = fsm.Transition(apitransport.SignalDisconnected)
	if err != nil {
		t.Fatalf("disconnected transition: %v", err)
	}
	if state != StateDisconnected {
		t.Fatalf("expected disconnected state, got %s", state)
	}
	if got := fsm.CleanupDeadline(); !got.Equal(base.Add(10 * time.Second)) {
		t.Fatalf("unexpected cleanup deadline: %s", got)
	}

	state, err = fsm.Transition(apitransport.SignalConnected)
	if err != nil {
		t.Fatalf("reconnect transition: %v", err)
	}
	if state != StateConnected {
		t.Fatalf("expected connected state after reconnect, got %s", state)
	}

	state, err = fsm.Transition(apitransport.SignalEnded)
	if err != nil {
		t.Fatalf("ended transition: %v", err)
	}
	if state != StateEnded || !fsm.IsTerminal() {
		t.Fatalf("expected terminal ended state, got state=%s terminal=%v", state, fsm.IsTerminal())
	}

	if _, err := fsm.Transition(apitransport.SignalConnected); err == nil {
		t.Fatalf("expected no further transitions after terminal state")
	}
}

func TestFSMRejectsInvalidTransitions(t *testing.T) {
	t.Parallel()

	fsm := New(Config{})
	if _, err := fsm.Transition(apitransport.SignalReconnecting); err == nil {
		t.Fatalf("expected reconnecting before connected to fail")
	}
	if _, err := fsm.Transition("invalid"); err == nil {
		t.Fatalf("expected invalid signal to fail")
	}
}
