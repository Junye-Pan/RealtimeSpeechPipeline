package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// Context is the runtime correlation and idempotency envelope.
type Context struct {
	SessionID      string
	TurnID         string
	EventID        string
	IdempotencyKey string
	CorrelationID  string
}

// Service generates deterministic identity/correlation context.
type Service struct {
	mu  sync.Mutex
	seq map[string]int64
}

// NewService returns a deterministic identity service.
func NewService() *Service {
	return &Service{
		seq: map[string]int64{},
	}
}

// NewEventContext issues a deterministic event context for a session/turn pair.
func (s *Service) NewEventContext(sessionID, turnID string) (Context, error) {
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" {
		return Context{}, fmt.Errorf("session_id is required")
	}
	if turnID == "" {
		turnID = "session"
	}

	key := sessionID + "/" + turnID
	s.mu.Lock()
	s.seq[key]++
	ordinal := s.seq[key]
	s.mu.Unlock()

	eventID := fmt.Sprintf("evt/%s/%s/%06d", sanitize(sessionID), sanitize(turnID), ordinal)
	correlationID := fmt.Sprintf("corr/%s/%s", sanitize(sessionID), sanitize(turnID))
	idempotencyKey := hashID(key + "/" + eventID)
	ctx := Context{
		SessionID:      sessionID,
		TurnID:         turnID,
		EventID:        eventID,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  correlationID,
	}
	if err := ValidateContext(ctx); err != nil {
		return Context{}, err
	}
	return ctx, nil
}

// ValidateContext validates identity context completeness.
func ValidateContext(ctx Context) error {
	if strings.TrimSpace(ctx.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	if strings.TrimSpace(ctx.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(ctx.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	if strings.TrimSpace(ctx.CorrelationID) == "" {
		return fmt.Errorf("correlation_id is required")
	}
	return nil
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, " ", "_")
	if value == "" {
		return "unknown"
	}
	return value
}

func hashID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
