package cancellation

import (
	"fmt"
	"strings"
	"sync"
)

// Fence tracks deterministic cancel acceptance for session/turn pairs.
type Fence struct {
	mu       sync.Mutex
	canceled map[string]bool
}

// NewFence returns an empty cancellation fence.
func NewFence() *Fence {
	return &Fence{canceled: map[string]bool{}}
}

// Accept marks a turn as cancellation-fenced.
func (f *Fence) Accept(sessionID, turnID string) error {
	key, err := turnKey(sessionID, turnID)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.canceled[key] = true
	return nil
}

// IsFenced returns true when cancellation has been accepted for the turn.
func (f *Fence) IsFenced(sessionID, turnID string) bool {
	key, err := turnKey(sessionID, turnID)
	if err != nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.canceled[key]
}

func turnKey(sessionID, turnID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return "", fmt.Errorf("session_id and turn_id are required")
	}
	return sessionID + "/" + turnID, nil
}
