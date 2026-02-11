package telemetry

import (
	"context"
	"sync"
)

// MemorySink is a deterministic in-memory sink used by tests.
type MemorySink struct {
	mu     sync.Mutex
	events []Event
}

// NewMemorySink returns an empty in-memory sink.
func NewMemorySink() *MemorySink {
	return &MemorySink{events: make([]Event, 0, 64)}
}

// Export appends an event in memory.
func (s *MemorySink) Export(_ context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

// Events returns a copy of all exported events.
func (s *MemorySink) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, len(s.events))
	copy(out, s.events)
	return out
}
