package executionpool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Task is one deterministic execution pool unit.
type Task struct {
	ID             string
	Run            func() error
	FairnessKey    string
	MaxOutstanding int
}

var (
	// ErrTaskIDRequired is returned when a task is missing an ID.
	ErrTaskIDRequired = errors.New("task id is required")
	// ErrTaskRunRequired is returned when a task is missing a run function.
	ErrTaskRunRequired = errors.New("task run func is required")
	// ErrClosed indicates the execution pool no longer accepts submissions.
	ErrClosed = errors.New("execution pool is closed")
	// ErrQueueFull indicates the execution pool queue is saturated.
	ErrQueueFull = errors.New("execution pool queue is full")
	// ErrNodeConcurrencyExceeded indicates a per-key concurrency/outstanding limit was exceeded.
	ErrNodeConcurrencyExceeded = errors.New("execution pool node concurrency limit exceeded")
)

// Stats reports execution pool counters.
type Stats struct {
	Submitted             int64
	Completed             int64
	Rejected              int64
	RejectedByConcurrency int64
	InFlight              int64
	QueueDepth            int64
}

// Manager is a bounded single-worker FIFO execution pool.
type Manager struct {
	queue                 chan Task
	wg                    sync.WaitGroup
	submitted             atomic.Int64
	completed             atomic.Int64
	rejected              atomic.Int64
	rejectedByConcurrency atomic.Int64
	inFlight              atomic.Int64
	closed                atomic.Bool
	mu                    sync.Mutex
	outstandingByKey      map[string]int
}

// NewManager creates a deterministic FIFO manager.
func NewManager(capacity int) *Manager {
	if capacity < 1 {
		capacity = 64
	}
	m := &Manager{
		queue:            make(chan Task, capacity),
		outstandingByKey: make(map[string]int),
	}
	m.wg.Add(1)
	go m.worker()
	return m
}

// Submit enqueues a task or returns error when saturated/closed.
func (m *Manager) Submit(task Task) error {
	if task.ID == "" {
		return fmt.Errorf("%w", ErrTaskIDRequired)
	}
	if task.Run == nil {
		return fmt.Errorf("%w", ErrTaskRunRequired)
	}
	if task.MaxOutstanding < 0 {
		m.rejected.Add(1)
		return fmt.Errorf("max outstanding must be >= 0")
	}
	if m.closed.Load() {
		m.rejected.Add(1)
		return fmt.Errorf("%w", ErrClosed)
	}

	outstandingReserved := false
	outstandingKey := ""
	if task.MaxOutstanding > 0 {
		outstandingKey = outstandingKeyForTask(task)
		if !m.reserveOutstanding(outstandingKey, task.MaxOutstanding) {
			m.rejected.Add(1)
			m.rejectedByConcurrency.Add(1)
			return fmt.Errorf("%w", ErrNodeConcurrencyExceeded)
		}
		outstandingReserved = true
	}

	select {
	case m.queue <- task:
		m.submitted.Add(1)
		return nil
	default:
		if outstandingReserved {
			m.releaseOutstanding(outstandingKey)
		}
		m.rejected.Add(1)
		return fmt.Errorf("%w", ErrQueueFull)
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
		Submitted:             m.submitted.Load(),
		Completed:             m.completed.Load(),
		Rejected:              m.rejected.Load(),
		RejectedByConcurrency: m.rejectedByConcurrency.Load(),
		InFlight:              m.inFlight.Load(),
		QueueDepth:            int64(len(m.queue)),
	}
}

func (m *Manager) worker() {
	defer m.wg.Done()
	for task := range m.queue {
		m.inFlight.Add(1)
		_ = task.Run()
		if task.MaxOutstanding > 0 {
			m.releaseOutstanding(outstandingKeyForTask(task))
		}
		m.completed.Add(1)
		m.inFlight.Add(-1)
	}
}

func (m *Manager) reserveOutstanding(key string, maxOutstanding int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.outstandingByKey[key] >= maxOutstanding {
		return false
	}
	m.outstandingByKey[key]++
	return true
}

func (m *Manager) releaseOutstanding(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.outstandingByKey[key]
	if current <= 1 {
		delete(m.outstandingByKey, key)
		return
	}
	m.outstandingByKey[key] = current - 1
}

func outstandingKeyForTask(task Task) string {
	if key := strings.TrimSpace(task.FairnessKey); key != "" {
		return key
	}
	return task.ID
}
