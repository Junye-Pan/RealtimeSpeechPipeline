package nodehost

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHostLifecycleHooksInvokeOnceInOrder(t *testing.T) {
	t.Parallel()

	node := &recordingNode{}
	host := NewHost(HostConfig{HookTimeout: 100 * time.Millisecond})

	if err := host.Register(context.Background(), "node-a", func() Node { return node }); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := host.Start(context.Background(), "node-a"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := host.Dispatch(context.Background(), "node-a", "event-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if err := host.Cancel(context.Background(), "node-a", "barge_in"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := host.Stop(context.Background(), "node-a"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// Repeated stop should be idempotent.
	if err := host.Stop(context.Background(), "node-a"); err != nil {
		t.Fatalf("stop idempotent: %v", err)
	}

	node.mu.Lock()
	defer node.mu.Unlock()
	if node.initCount != 1 || node.startCount != 1 || node.handleCount != 1 || node.cancelCount != 1 || node.stopCount != 1 {
		t.Fatalf("unexpected hook counts: %+v", node)
	}
	expected := []string{"init", "start", "handle", "cancel", "stop"}
	if len(node.order) != len(expected) {
		t.Fatalf("unexpected hook order length: got=%v expected=%v", node.order, expected)
	}
	for idx := range expected {
		if node.order[idx] != expected[idx] {
			t.Fatalf("unexpected hook order: got=%v expected=%v", node.order, expected)
		}
	}
}

func TestHostStartTimeoutReturnsErrHookTimeout(t *testing.T) {
	t.Parallel()

	node := &recordingNode{
		startDelay: 200 * time.Millisecond,
	}
	host := NewHost(HostConfig{HookTimeout: 20 * time.Millisecond})

	if err := host.Register(context.Background(), "node-timeout", func() Node { return node }); err != nil {
		t.Fatalf("register: %v", err)
	}
	err := host.Start(context.Background(), "node-timeout")
	if err == nil {
		t.Fatalf("expected start timeout")
	}
	if !errors.Is(err, ErrHookTimeout) {
		t.Fatalf("expected ErrHookTimeout, got %v", err)
	}
}

func TestHostRejectsDuplicateRegistration(t *testing.T) {
	t.Parallel()

	host := NewHost(HostConfig{})
	if err := host.Register(context.Background(), "node-dup", func() Node { return &recordingNode{} }); err != nil {
		t.Fatalf("register first: %v", err)
	}
	err := host.Register(context.Background(), "node-dup", func() Node { return &recordingNode{} })
	if err == nil {
		t.Fatalf("expected duplicate registration error")
	}
	if !errors.Is(err, ErrNodeAlreadyRegistered) {
		t.Fatalf("expected ErrNodeAlreadyRegistered, got %v", err)
	}
}

type recordingNode struct {
	mu sync.Mutex

	initCount   int
	startCount  int
	handleCount int
	cancelCount int
	stopCount   int
	order       []string

	startDelay time.Duration
}

func (n *recordingNode) Init(context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.initCount++
	n.order = append(n.order, "init")
	return nil
}

func (n *recordingNode) Start(ctx context.Context) error {
	if n.startDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(n.startDelay):
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.startCount++
	n.order = append(n.order, "start")
	return nil
}

func (n *recordingNode) HandleEvent(context.Context, any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handleCount++
	n.order = append(n.order, "handle")
	return nil
}

func (n *recordingNode) OnCancel(context.Context, string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cancelCount++
	n.order = append(n.order, "cancel")
	return nil
}

func (n *recordingNode) Stop(context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stopCount++
	n.order = append(n.order, "stop")
	return nil
}
