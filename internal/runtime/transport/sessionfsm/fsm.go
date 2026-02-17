package sessionfsm

import (
	"fmt"
	"time"

	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
)

// State is the normalized transport connection state.
type State string

const (
	StateConnected    State = "connected"
	StateReconnecting State = "reconnecting"
	StateDisconnected State = "disconnected"
	StateEnded        State = "ended"
	StateSilence      State = "silence"
	StateStall        State = "stall"
)

// Config controls deterministic lifecycle behavior.
type Config struct {
	DisconnectTimeout time.Duration
	Now               func() time.Time
}

// FSM tracks normalized connection lifecycle transitions.
type FSM struct {
	state             State
	terminal          bool
	cleanupDeadline   time.Time
	disconnectTimeout time.Duration
	now               func() time.Time
	initialized       bool
}

// New returns an FSM with deterministic defaults.
func New(cfg Config) *FSM {
	if cfg.DisconnectTimeout <= 0 {
		cfg.DisconnectTimeout = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &FSM{
		state:             StateDisconnected,
		disconnectTimeout: cfg.DisconnectTimeout,
		now:               cfg.Now,
	}
}

// Transition applies a connection signal and returns the resulting state.
func (f *FSM) Transition(signal apitransport.ConnectionSignal) (State, error) {
	if f.terminal {
		return f.state, fmt.Errorf("session is terminal in state %s", f.state)
	}
	if !isSignalSupported(signal) {
		return f.state, fmt.Errorf("unsupported connection signal %q", signal)
	}

	switch signal {
	case apitransport.SignalConnected:
		f.state = StateConnected
		f.cleanupDeadline = time.Time{}
		f.initialized = true
	case apitransport.SignalReconnecting:
		if !f.initialized {
			return f.state, fmt.Errorf("cannot transition to reconnecting before connected")
		}
		f.state = StateReconnecting
	case apitransport.SignalDisconnected:
		if !f.initialized {
			return f.state, fmt.Errorf("cannot transition to disconnected before connected")
		}
		f.state = StateDisconnected
		f.cleanupDeadline = f.now().Add(f.disconnectTimeout)
	case apitransport.SignalSilence:
		if !f.initialized {
			return f.state, fmt.Errorf("cannot transition to silence before connected")
		}
		f.state = StateSilence
	case apitransport.SignalStall:
		if !f.initialized {
			return f.state, fmt.Errorf("cannot transition to stall before connected")
		}
		f.state = StateStall
	case apitransport.SignalEnded:
		if !f.initialized {
			return f.state, fmt.Errorf("cannot transition to ended before connected")
		}
		f.state = StateEnded
		f.cleanupDeadline = f.now().Add(f.disconnectTimeout)
		f.terminal = true
	}
	return f.state, nil
}

// State returns the current lifecycle state.
func (f *FSM) State() State {
	return f.state
}

// IsTerminal reports whether lifecycle reached terminal state.
func (f *FSM) IsTerminal() bool {
	return f.terminal
}

// CleanupDeadline returns deterministic cleanup deadline once disconnected/ended.
func (f *FSM) CleanupDeadline() time.Time {
	return f.cleanupDeadline
}

func isSignalSupported(signal apitransport.ConnectionSignal) bool {
	switch signal {
	case apitransport.SignalConnected, apitransport.SignalReconnecting, apitransport.SignalDisconnected,
		apitransport.SignalEnded, apitransport.SignalSilence, apitransport.SignalStall:
		return true
	default:
		return false
	}
}
