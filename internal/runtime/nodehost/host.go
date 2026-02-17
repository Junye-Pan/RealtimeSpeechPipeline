package nodehost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNodeNotRegistered indicates an unknown node id.
	ErrNodeNotRegistered = errors.New("node not registered")
	// ErrNodeAlreadyRegistered indicates duplicate registration for a node id.
	ErrNodeAlreadyRegistered = errors.New("node already registered")
	// ErrNodeNotStarted indicates dispatch/cancel attempted before start.
	ErrNodeNotStarted = errors.New("node not started")
	// ErrNodeStopped indicates operations attempted after stop.
	ErrNodeStopped = errors.New("node stopped")
	// ErrHookTimeout indicates a node lifecycle hook exceeded the host timeout.
	ErrHookTimeout = errors.New("node hook timeout")
)

const defaultHookTimeout = 250 * time.Millisecond

// Node defines the in-process runtime node lifecycle contract.
type Node interface {
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	HandleEvent(ctx context.Context, event any) error
	OnCancel(ctx context.Context, reason string) error
	Stop(ctx context.Context) error
}

// NodeFactory creates a node instance for host registration.
type NodeFactory func() Node

// HostConfig controls lifecycle timeout behavior.
type HostConfig struct {
	HookTimeout time.Duration
}

type hostEntry struct {
	node        Node
	initialized bool
	started     bool
	stopped     bool
}

// Host orchestrates node lifecycle hooks with deterministic timeout enforcement.
type Host struct {
	mu      sync.Mutex
	cfg     HostConfig
	entries map[string]*hostEntry
}

// NewHost constructs a node host.
func NewHost(cfg HostConfig) *Host {
	if cfg.HookTimeout <= 0 {
		cfg.HookTimeout = defaultHookTimeout
	}
	return &Host{
		cfg:     cfg,
		entries: make(map[string]*hostEntry),
	}
}

// Register creates and initializes a node exactly once.
func (h *Host) Register(ctx context.Context, nodeID string, factory NodeFactory) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if factory == nil {
		return fmt.Errorf("factory is required")
	}

	h.mu.Lock()
	if _, ok := h.entries[nodeID]; ok {
		h.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeAlreadyRegistered, nodeID)
	}
	entry := &hostEntry{node: factory()}
	if entry.node == nil {
		h.mu.Unlock()
		return fmt.Errorf("factory returned nil node")
	}
	h.entries[nodeID] = entry
	h.mu.Unlock()

	if err := h.runHook(ctx, nodeID, "init", entry.node.Init); err != nil {
		return err
	}

	h.mu.Lock()
	entry.initialized = true
	h.mu.Unlock()
	return nil
}

// Start starts a previously registered node.
func (h *Host) Start(ctx context.Context, nodeID string) error {
	entry, err := h.lookup(nodeID)
	if err != nil {
		return err
	}

	h.mu.Lock()
	if entry.stopped {
		h.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeStopped, nodeID)
	}
	if entry.started {
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()

	if err := h.runHook(ctx, nodeID, "start", entry.node.Start); err != nil {
		return err
	}

	h.mu.Lock()
	entry.started = true
	h.mu.Unlock()
	return nil
}

// Dispatch sends one event to a started node.
func (h *Host) Dispatch(ctx context.Context, nodeID string, event any) error {
	entry, err := h.lookup(nodeID)
	if err != nil {
		return err
	}

	h.mu.Lock()
	if entry.stopped {
		h.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeStopped, nodeID)
	}
	if !entry.started {
		h.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeNotStarted, nodeID)
	}
	h.mu.Unlock()

	return h.runHook(ctx, nodeID, "dispatch", func(hookCtx context.Context) error {
		return entry.node.HandleEvent(hookCtx, event)
	})
}

// Cancel forwards cancellation to a started node.
func (h *Host) Cancel(ctx context.Context, nodeID string, reason string) error {
	entry, err := h.lookup(nodeID)
	if err != nil {
		return err
	}

	h.mu.Lock()
	if entry.stopped {
		h.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeStopped, nodeID)
	}
	if !entry.started {
		h.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeNotStarted, nodeID)
	}
	h.mu.Unlock()

	return h.runHook(ctx, nodeID, "cancel", func(hookCtx context.Context) error {
		return entry.node.OnCancel(hookCtx, reason)
	})
}

// Stop stops a started node exactly once.
func (h *Host) Stop(ctx context.Context, nodeID string) error {
	entry, err := h.lookup(nodeID)
	if err != nil {
		return err
	}

	h.mu.Lock()
	if entry.stopped {
		h.mu.Unlock()
		return nil
	}
	if !entry.started {
		h.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNodeNotStarted, nodeID)
	}
	h.mu.Unlock()

	if err := h.runHook(ctx, nodeID, "stop", entry.node.Stop); err != nil {
		return err
	}

	h.mu.Lock()
	entry.stopped = true
	h.mu.Unlock()
	return nil
}

func (h *Host) lookup(nodeID string) (*hostEntry, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.entries[nodeID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotRegistered, nodeID)
	}
	if !entry.initialized {
		return nil, fmt.Errorf("node %q is not initialized", nodeID)
	}
	return entry, nil
}

func (h *Host) runHook(ctx context.Context, nodeID, hook string, fn func(context.Context) error) error {
	hookCtx := ctx
	if hookCtx == nil {
		hookCtx = context.Background()
	}
	hookCtx, cancel := context.WithTimeout(hookCtx, h.cfg.HookTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- fn(hookCtx)
	}()

	select {
	case <-hookCtx.Done():
		if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: node=%s hook=%s", ErrHookTimeout, nodeID, hook)
		}
		return hookCtx.Err()
	case err := <-errCh:
		if err == nil {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: node=%s hook=%s", ErrHookTimeout, nodeID, hook)
		}
		return err
	}
}
