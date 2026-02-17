package telephony

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/authority"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/kernel"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/routing"
)

// Config controls transport-kernel defaults for telephony adapter sessions.
type Config struct {
	DisconnectTimeout time.Duration
	DefaultDataClass  eventabi.PayloadClass
}

// Adapter implements shared transport contract behavior for telephony sessions.
type Adapter struct {
	cfg   Config
	guard authority.Guard
	fence *runtimetransport.OutputFence

	mu      sync.RWMutex
	session *kernel.Session
}

// NewAdapter creates a telephony adapter.
func NewAdapter(cfg Config, guard authority.Guard, fence *runtimetransport.OutputFence) *Adapter {
	if cfg.DisconnectTimeout <= 0 {
		cfg.DisconnectTimeout = 30 * time.Second
	}
	if cfg.DefaultDataClass == "" {
		cfg.DefaultDataClass = eventabi.PayloadAudioRaw
	}
	return &Adapter{cfg: cfg, guard: guard, fence: fence}
}

// Kind returns adapter transport kind.
func (a *Adapter) Kind() apitransport.TransportKind {
	return apitransport.TransportTelephony
}

// Capabilities returns deterministic adapter capabilities.
func (a *Adapter) Capabilities() apitransport.AdapterCapabilities {
	return apitransport.AdapterCapabilities{
		TransportKind: apitransport.TransportTelephony,
		SupportedConnectionSignals: []apitransport.ConnectionSignal{
			apitransport.SignalConnected,
			apitransport.SignalDisconnected,
			apitransport.SignalEnded,
			apitransport.SignalSilence,
			apitransport.SignalStall,
		},
		SupportsIngressAudio:       true,
		SupportsIngressControl:     true,
		SupportsEgressAudio:        true,
		SupportsReconnectSemantics: false,
	}
}

// Open validates bootstrap and initializes transport-kernel session.
func (a *Adapter) Open(ctx context.Context, bootstrap apitransport.SessionBootstrap) error {
	if bootstrap.TransportKind != apitransport.TransportTelephony {
		return fmt.Errorf("telephony adapter requires transport_kind=telephony")
	}
	session, err := kernel.NewSession(kernel.Config{
		DisconnectTimeout: a.cfg.DisconnectTimeout,
		DefaultDataClass:  a.cfg.DefaultDataClass,
	}, bootstrap, a.guard, a.fence)
	if err != nil {
		return err
	}
	if _, err := session.OnConnection(apitransport.SignalConnected); err != nil {
		return err
	}
	a.mu.Lock()
	a.session = session
	a.mu.Unlock()
	return nil
}

// ConnectionSignal applies lifecycle signals to the active session.
func (a *Adapter) ConnectionSignal(signal apitransport.ConnectionSignal) error {
	session, err := a.currentSession()
	if err != nil {
		return err
	}
	_, err = session.OnConnection(signal)
	return err
}

// HandleIngress normalizes ingress events through transport kernel.
func (a *Adapter) HandleIngress(record eventabi.EventRecord, codec string) (eventabi.EventRecord, error) {
	session, err := a.currentSession()
	if err != nil {
		return eventabi.EventRecord{}, err
	}
	return session.NormalizeIngress(record, codec)
}

// HandleEgress applies authority + cancel fencing to egress output.
func (a *Adapter) HandleEgress(attempt runtimetransport.OutputAttempt, payloadBytes int) (runtimetransport.OutputDecision, error) {
	session, err := a.currentSession()
	if err != nil {
		return runtimetransport.OutputDecision{}, err
	}
	return session.DeliverEgress(attempt, payloadBytes)
}

// OnRoutingUpdate applies a routing snapshot update.
func (a *Adapter) OnRoutingUpdate(view routing.View) (bool, error) {
	session, err := a.currentSession()
	if err != nil {
		return false, err
	}
	return session.ApplyRoutingUpdate(view)
}

func (a *Adapter) currentSession() (*kernel.Session, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.session == nil {
		return nil, fmt.Errorf("telephony session is not open")
	}
	return a.session, nil
}
