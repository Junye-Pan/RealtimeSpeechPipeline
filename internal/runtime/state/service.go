package state

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// StaleAuthorityError indicates an operation was attempted with an older authority epoch.
type StaleAuthorityError struct {
	SessionID     string
	ExpectedEpoch int64
	ProvidedEpoch int64
}

func (e StaleAuthorityError) Error() string {
	return fmt.Sprintf("stale authority epoch for session %q: expected >= %d, provided %d", e.SessionID, e.ExpectedEpoch, e.ProvidedEpoch)
}

// StaleAuthority marks this error as an authority staleness rejection.
func (e StaleAuthorityError) StaleAuthority() bool {
	return true
}

// IsStaleAuthority reports whether err is a stale-authority rejection.
func IsStaleAuthority(err error) bool {
	var staleErr interface{ StaleAuthority() bool }
	return errors.As(err, &staleErr) && staleErr.StaleAuthority()
}

// Service stores runtime-local turn/session state with epoch gating and dedupe registries.
//
// Runtime compute remains stateless-by-design for durable state. This service is intentionally
// scoped to turn-ephemeral and session-hot caches plus idempotency registries used by runtime paths.
type Service struct {
	mu sync.RWMutex

	sessionEpoch  map[string]int64
	sessionHot    map[string]map[string]any
	turnEphemeral map[string]map[string]map[string]any

	idempotencyKeys map[string]map[string]struct{}
	invocationKeys  map[string]map[string]struct{}
}

// NewService constructs an empty state service.
func NewService() *Service {
	return &Service{
		sessionEpoch:    make(map[string]int64),
		sessionHot:      make(map[string]map[string]any),
		turnEphemeral:   make(map[string]map[string]map[string]any),
		idempotencyKeys: make(map[string]map[string]struct{}),
		invocationKeys:  make(map[string]map[string]struct{}),
	}
}

// ValidateAuthority enforces monotonic authority epoch progression per session.
func (s *Service) ValidateAuthority(sessionID string, authorityEpoch int64) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if authorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >= 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.sessionEpoch[sessionID]
	if !ok {
		s.sessionEpoch[sessionID] = authorityEpoch
		return nil
	}
	if authorityEpoch < current {
		return StaleAuthorityError{
			SessionID:     sessionID,
			ExpectedEpoch: current,
			ProvidedEpoch: authorityEpoch,
		}
	}
	s.sessionEpoch[sessionID] = authorityEpoch
	return nil
}

// CurrentAuthorityEpoch returns the current tracked authority epoch for a session.
func (s *Service) CurrentAuthorityEpoch(sessionID string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	epoch, ok := s.sessionEpoch[strings.TrimSpace(sessionID)]
	return epoch, ok
}

// PutSessionHot writes session-hot state after authority validation.
func (s *Service) PutSessionHot(sessionID, key string, authorityEpoch int64, value any) error {
	if err := s.ValidateAuthority(sessionID, authorityEpoch); err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessionHot[sessionID]; !ok {
		s.sessionHot[sessionID] = make(map[string]any)
	}
	s.sessionHot[sessionID][key] = value
	return nil
}

// GetSessionHot reads a session-hot state value.
func (s *Service) GetSessionHot(sessionID, key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session := s.sessionHot[strings.TrimSpace(sessionID)]
	if session == nil {
		return nil, false
	}
	value, ok := session[strings.TrimSpace(key)]
	return value, ok
}

// ResetSessionHot clears session-hot state for the session after authority validation.
func (s *Service) ResetSessionHot(sessionID string, authorityEpoch int64) error {
	if err := s.ValidateAuthority(sessionID, authorityEpoch); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionHot, sessionID)
	return nil
}

// PutTurnEphemeral writes turn-ephemeral state after authority validation.
func (s *Service) PutTurnEphemeral(sessionID, turnID, key string, authorityEpoch int64, value any) error {
	if err := s.ValidateAuthority(sessionID, authorityEpoch); err != nil {
		return err
	}
	turnID = strings.TrimSpace(turnID)
	key = strings.TrimSpace(key)
	if turnID == "" {
		return fmt.Errorf("turn_id is required")
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.turnEphemeral[sessionID]; !ok {
		s.turnEphemeral[sessionID] = make(map[string]map[string]any)
	}
	if _, ok := s.turnEphemeral[sessionID][turnID]; !ok {
		s.turnEphemeral[sessionID][turnID] = make(map[string]any)
	}
	s.turnEphemeral[sessionID][turnID][key] = value
	return nil
}

// GetTurnEphemeral reads a turn-ephemeral state value.
func (s *Service) GetTurnEphemeral(sessionID, turnID, key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	turns := s.turnEphemeral[strings.TrimSpace(sessionID)]
	if turns == nil {
		return nil, false
	}
	turn := turns[strings.TrimSpace(turnID)]
	if turn == nil {
		return nil, false
	}
	value, ok := turn[strings.TrimSpace(key)]
	return value, ok
}

// ClearTurnEphemeral clears state for one turn.
func (s *Service) ClearTurnEphemeral(sessionID, turnID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	turns := s.turnEphemeral[strings.TrimSpace(sessionID)]
	if turns == nil {
		return
	}
	delete(turns, strings.TrimSpace(turnID))
	if len(turns) == 0 {
		delete(s.turnEphemeral, strings.TrimSpace(sessionID))
	}
}

// RegisterIdempotencyKey registers a request key; false indicates duplicate.
func (s *Service) RegisterIdempotencyKey(sessionID, key string, authorityEpoch int64) (bool, error) {
	return s.registerKey(sessionID, key, authorityEpoch, s.idempotencyKeys)
}

// RegisterInvocationKey registers a provider invocation key; false indicates duplicate.
func (s *Service) RegisterInvocationKey(sessionID, invocationID string, authorityEpoch int64) (bool, error) {
	return s.registerKey(sessionID, invocationID, authorityEpoch, s.invocationKeys)
}

func (s *Service) registerKey(sessionID, key string, authorityEpoch int64, registry map[string]map[string]struct{}) (bool, error) {
	if err := s.ValidateAuthority(sessionID, authorityEpoch); err != nil {
		return false, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, fmt.Errorf("key is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := registry[sessionID]; !ok {
		registry[sessionID] = make(map[string]struct{})
	}
	if _, exists := registry[sessionID][key]; exists {
		return false, nil
	}
	registry[sessionID][key] = struct{}{}
	return true, nil
}
