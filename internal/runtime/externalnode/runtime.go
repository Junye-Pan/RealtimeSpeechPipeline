package externalnode

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrResourceLimitExceeded indicates an external node invocation exceeded configured limits.
	ErrResourceLimitExceeded = errors.New("external node resource limit exceeded")
	// ErrExecutionTimeout indicates an external invocation exceeded runtime timeout.
	ErrExecutionTimeout = errors.New("external node execution timeout")
)

const defaultTimeout = 500 * time.Millisecond

// Request defines one out-of-process node invocation.
type Request struct {
	SessionID      string
	TurnID         string
	NodeID         string
	InvocationID   string
	AuthorityEpoch int64
	CPUUnits       int64
	MemoryMB       int64
	Payload        any
}

// Response is a normalized external invocation output.
type Response struct {
	InvocationID string
	Output       any
	Telemetry    map[string]string
}

// Adapter executes out-of-process nodes and handles cancellation.
type Adapter interface {
	Invoke(ctx context.Context, req Request) (Response, error)
	Cancel(ctx context.Context, invocationID string, reason string) error
}

// Config controls timeout and resource envelopes for external invocation.
type Config struct {
	Timeout     time.Duration
	MaxCPUUnits int64
	MaxMemoryMB int64
}

// Runtime enforces external-node boundary policies.
type Runtime struct {
	adapter Adapter
	cfg     Config
}

// NewRuntime constructs an external-node runtime boundary.
func NewRuntime(adapter Adapter, cfg Config) (*Runtime, error) {
	if adapter == nil {
		return nil, fmt.Errorf("adapter is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxCPUUnits <= 0 {
		cfg.MaxCPUUnits = 1
	}
	if cfg.MaxMemoryMB <= 0 {
		cfg.MaxMemoryMB = 256
	}
	return &Runtime{adapter: adapter, cfg: cfg}, nil
}

// Invoke executes an external node with timeout and resource checks.
func (r *Runtime) Invoke(ctx context.Context, req Request) (Response, error) {
	if err := validateRequest(req); err != nil {
		return Response{}, err
	}
	if req.CPUUnits > r.cfg.MaxCPUUnits || req.MemoryMB > r.cfg.MaxMemoryMB {
		return Response{}, fmt.Errorf("%w: requested cpu=%d memory_mb=%d max_cpu=%d max_memory_mb=%d", ErrResourceLimitExceeded, req.CPUUnits, req.MemoryMB, r.cfg.MaxCPUUnits, r.cfg.MaxMemoryMB)
	}

	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(callCtx, r.cfg.Timeout)
	defer cancel()

	respCh := make(chan invokeResult, 1)
	go func() {
		resp, err := r.adapter.Invoke(callCtx, req)
		respCh <- invokeResult{response: resp, err: err}
	}()

	select {
	case <-callCtx.Done():
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return Response{}, fmt.Errorf("%w: invocation_id=%s", ErrExecutionTimeout, req.InvocationID)
		}
		return Response{}, callCtx.Err()
	case result := <-respCh:
		if result.err != nil {
			if errors.Is(result.err, context.DeadlineExceeded) {
				return Response{}, fmt.Errorf("%w: invocation_id=%s", ErrExecutionTimeout, req.InvocationID)
			}
			return Response{}, result.err
		}
		resp := result.response
		if strings.TrimSpace(resp.InvocationID) == "" {
			resp.InvocationID = req.InvocationID
		}
		if resp.Telemetry == nil {
			resp.Telemetry = make(map[string]string)
		}
		resp.Telemetry["boundary"] = "external_node"
		resp.Telemetry["node_id"] = req.NodeID
		resp.Telemetry["cpu_units"] = strconv.FormatInt(req.CPUUnits, 10)
		resp.Telemetry["memory_mb"] = strconv.FormatInt(req.MemoryMB, 10)
		return resp, nil
	}
}

// Cancel propagates a cancellation request to the external adapter.
func (r *Runtime) Cancel(ctx context.Context, invocationID string, reason string) error {
	invocationID = strings.TrimSpace(invocationID)
	if invocationID == "" {
		return fmt.Errorf("invocation_id is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("reason is required")
	}

	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(callCtx, r.cfg.Timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.adapter.Cancel(callCtx, invocationID, reason)
	}()

	select {
	case <-callCtx.Done():
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: invocation_id=%s", ErrExecutionTimeout, invocationID)
		}
		return callCtx.Err()
	case err := <-errCh:
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: invocation_id=%s", ErrExecutionTimeout, invocationID)
		}
		return err
	}
}

func validateRequest(req Request) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	if strings.TrimSpace(req.NodeID) == "" {
		return fmt.Errorf("node_id is required")
	}
	if strings.TrimSpace(req.InvocationID) == "" {
		return fmt.Errorf("invocation_id is required")
	}
	if req.AuthorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >= 0")
	}
	if req.CPUUnits < 0 || req.MemoryMB < 0 {
		return fmt.Errorf("resource values must be >= 0")
	}
	return nil
}

type invokeResult struct {
	response Response
	err      error
}
