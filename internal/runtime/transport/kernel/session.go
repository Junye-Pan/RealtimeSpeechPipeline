package kernel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/authority"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/egress"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/ingress"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/routing"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/sessionfsm"
)

// Config controls transport kernel session defaults.
type Config struct {
	DisconnectTimeout time.Duration
	DefaultDataClass  eventabi.PayloadClass
}

// Session wires shared transport lifecycle/ingress/egress/routing behavior.
type Session struct {
	mu        sync.RWMutex
	bootstrap apitransport.SessionBootstrap
	guard     authority.Guard
	fsm       *sessionfsm.FSM
	router    *routing.Client
	egress    egress.Broker
	cfg       Config
}

// NewSession creates a transport-kernel session from control-plane bootstrap data.
func NewSession(cfg Config, bootstrap apitransport.SessionBootstrap, guard authority.Guard, fence *runtimetransport.OutputFence) (*Session, error) {
	if cfg.DisconnectTimeout <= 0 {
		cfg.DisconnectTimeout = 30 * time.Second
	}
	if cfg.DefaultDataClass == "" {
		cfg.DefaultDataClass = eventabi.PayloadAudioRaw
	}
	if err := guard.ValidateBootstrap(context.Background(), bootstrap); err != nil {
		return nil, err
	}

	return &Session{
		bootstrap: bootstrap,
		guard:     guard,
		fsm:       sessionfsm.New(sessionfsm.Config{DisconnectTimeout: cfg.DisconnectTimeout}),
		router:    routing.NewClient(),
		egress:    egress.NewBroker(fence),
		cfg:       cfg,
	}, nil
}

// Bootstrap returns current bootstrap snapshot.
func (s *Session) Bootstrap() apitransport.SessionBootstrap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bootstrap
}

// OnConnection applies a normalized transport lifecycle signal.
func (s *Session) OnConnection(signal apitransport.ConnectionSignal) (sessionfsm.State, error) {
	return s.fsm.Transition(signal)
}

// NormalizeIngress validates authority and maps ingress records into ABI-compliant events.
func (s *Session) NormalizeIngress(record eventabi.EventRecord, sourceCodec string) (eventabi.EventRecord, error) {
	s.mu.RLock()
	bootstrap := s.bootstrap
	s.mu.RUnlock()

	observedEpoch := bootstrap.AuthorityEpoch
	if record.AuthorityEpoch != nil {
		observedEpoch = *record.AuthorityEpoch
	}
	if err := s.guard.ValidateIngress(bootstrap.AuthorityEpoch, observedEpoch); err != nil {
		return eventabi.EventRecord{}, err
	}
	return ingress.Normalize(ingress.NormalizeInput{
		Record:           record,
		DefaultDataClass: s.cfg.DefaultDataClass,
		SourceCodec:      sourceCodec,
	})
}

// DeliverEgress validates authority and applies output fencing for one egress attempt.
func (s *Session) DeliverEgress(attempt runtimetransport.OutputAttempt, payloadBytes int) (runtimetransport.OutputDecision, error) {
	s.mu.RLock()
	bootstrap := s.bootstrap
	s.mu.RUnlock()

	if err := s.guard.ValidateEgress(bootstrap.AuthorityEpoch, attempt.AuthorityEpoch); err != nil {
		return runtimetransport.OutputDecision{}, err
	}
	return s.egress.Deliver(egress.Delivery{Attempt: attempt, PayloadBytes: payloadBytes})
}

// ApplyRoutingUpdate applies routing view updates and tracks lease epoch advancement.
func (s *Session) ApplyRoutingUpdate(view routing.View) (bool, error) {
	changed, err := s.router.Apply(view)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if view.LeaseEpoch < 0 {
		return false, fmt.Errorf("routing lease_epoch must be >=0")
	}
	if view.LeaseEpoch > 0 {
		s.mu.Lock()
		s.bootstrap.AuthorityEpoch = view.LeaseEpoch
		s.mu.Unlock()
	}
	return true, nil
}

// RoutingView returns the latest routing view when available.
func (s *Session) RoutingView() (routing.View, bool) {
	return s.router.Current()
}

// CleanupDeadline returns lifecycle cleanup deadline for disconnected/ended sessions.
func (s *Session) CleanupDeadline() time.Time {
	return s.fsm.CleanupDeadline()
}
