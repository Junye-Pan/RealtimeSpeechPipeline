package executionpool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestManagerFIFOAndDrain(t *testing.T) {
	t.Parallel()

	manager := NewManager(4)
	var mu sync.Mutex
	order := make([]int, 0, 3)
	for i := 1; i <= 3; i++ {
		i := i
		if err := manager.Submit(Task{
			ID: "task",
			Run: func() error {
				mu.Lock()
				order = append(order, i)
				mu.Unlock()
				return nil
			},
		}); err != nil {
			t.Fatalf("unexpected submit error: %v", err)
		}
	}
	if err := manager.Drain(context.Background()); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("expected FIFO order [1,2,3], got %+v", order)
	}
	stats := manager.Stats()
	if stats.Submitted != 3 || stats.Completed != 3 {
		t.Fatalf("unexpected stats %+v", stats)
	}
}

func TestManagerRejectsWhenSaturated(t *testing.T) {
	t.Parallel()

	manager := NewManager(1)
	block := make(chan struct{})
	started := make(chan struct{})
	if err := manager.Submit(Task{
		ID: "blocking",
		Run: func() error {
			close(started)
			<-block
			return nil
		},
	}); err != nil {
		t.Fatalf("unexpected first submit error: %v", err)
	}
	<-started
	if err := manager.Submit(Task{
		ID:  "queued",
		Run: func() error { return nil },
	}); err != nil {
		t.Fatalf("unexpected second submit error: %v", err)
	}
	if err := manager.Submit(Task{
		ID:  "overflow",
		Run: func() error { return nil },
	}); err == nil {
		t.Fatalf("expected queue saturation reject")
	} else if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	close(block)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}

	if err := manager.Submit(Task{
		ID:  "after-drain",
		Run: func() error { return nil },
	}); err == nil {
		t.Fatalf("expected closed execution pool reject")
	} else if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestManagerEnforcesPerKeyOutstandingLimit(t *testing.T) {
	t.Parallel()

	manager := NewManager(4)
	heavyRelease := make(chan struct{})
	heavyStarted := make(chan struct{})

	if err := manager.Submit(Task{
		ID:             "heavy-1",
		FairnessKey:    "provider-heavy",
		MaxOutstanding: 1,
		Run: func() error {
			close(heavyStarted)
			<-heavyRelease
			return nil
		},
	}); err != nil {
		t.Fatalf("unexpected first heavy submit error: %v", err)
	}
	<-heavyStarted

	if err := manager.Submit(Task{
		ID:             "heavy-2",
		FairnessKey:    "provider-heavy",
		MaxOutstanding: 1,
		Run:            func() error { return nil },
	}); err == nil {
		t.Fatalf("expected per-key outstanding limit reject")
	} else if !errors.Is(err, ErrNodeConcurrencyExceeded) {
		t.Fatalf("expected ErrNodeConcurrencyExceeded, got %v", err)
	}

	if err := manager.Submit(Task{
		ID:             "light-1",
		FairnessKey:    "provider-light",
		MaxOutstanding: 1,
		Run:            func() error { return nil },
	}); err != nil {
		t.Fatalf("expected different fairness key submit to succeed, got %v", err)
	}

	close(heavyRelease)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}

	stats := manager.Stats()
	if stats.RejectedByConcurrency < 1 {
		t.Fatalf("expected concurrency-limited rejections, got %+v", stats)
	}
}
