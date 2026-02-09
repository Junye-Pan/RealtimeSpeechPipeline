package executionpool

import (
	"context"
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
	}
	close(block)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Drain(ctx); err != nil {
		t.Fatalf("unexpected drain error: %v", err)
	}
}
