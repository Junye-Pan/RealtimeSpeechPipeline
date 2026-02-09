package executionpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Task is one deterministic execution pool unit.
type Task struct {
	ID  string
	Run func() error
}

// Stats reports execution pool counters.
type Stats struct {
	Submitted  int64
	Completed  int64
	Rejected   int64
	InFlight   int64
	QueueDepth int64
}

// Manager is a bounded single-worker FIFO execution pool.
type Manager struct {
	queue     chan Task
	wg        sync.WaitGroup
	submitted atomic.Int64
	completed atomic.Int64
	rejected  atomic.Int64
	inFlight  atomic.Int64
	closed    atomic.Bool
}

// NewManager creates a deterministic FIFO manager.
func NewManager(capacity int) *Manager {
	if capacity < 1 {
		capacity = 64
	}
	m := &Manager{
		queue: make(chan Task, capacity),
	}
	m.wg.Add(1)
	go m.worker()
	return m
}

// Submit enqueues a task or returns error when saturated/closed.
func (m *Manager) Submit(task Task) error {
	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}
	if task.Run == nil {
		return fmt.Errorf("task run func is required")
	}
	if m.closed.Load() {
		m.rejected.Add(1)
		return fmt.Errorf("execution pool is closed")
	}
	select {
	case m.queue <- task:
		m.submitted.Add(1)
		return nil
	default:
		m.rejected.Add(1)
		return fmt.Errorf("execution pool queue is full")
	}
}

// Drain waits until queue/in-flight is empty, then closes worker.
func (m *Manager) Drain(ctx context.Context) error {
	for {
		if len(m.queue) == 0 && m.inFlight.Load() == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	if m.closed.CompareAndSwap(false, true) {
		close(m.queue)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.wg.Wait()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// Stats returns a snapshot of pool counters.
func (m *Manager) Stats() Stats {
	return Stats{
		Submitted:  m.submitted.Load(),
		Completed:  m.completed.Load(),
		Rejected:   m.rejected.Load(),
		InFlight:   m.inFlight.Load(),
		QueueDepth: int64(len(m.queue)),
	}
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for task := range m.queue {
		m.inFlight.Add(1)
		_ = task.Run()
		m.completed.Add(1)
		m.inFlight.Add(-1)
	}
}
