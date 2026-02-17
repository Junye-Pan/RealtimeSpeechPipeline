package externalnode

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeInvokeInjectsTelemetryAndEnforcesLimits(t *testing.T) {
	t.Parallel()

	adapter := &stubAdapter{
		invokeFn: func(_ context.Context, req Request) (Response, error) {
			return Response{
				InvocationID: req.InvocationID,
				Output:       "ok",
			}, nil
		},
	}
	runtime, err := NewRuntime(adapter, Config{
		Timeout:     100 * time.Millisecond,
		MaxCPUUnits: 2,
		MaxMemoryMB: 512,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	resp, err := runtime.Invoke(context.Background(), Request{
		SessionID:      "sess-ext-1",
		TurnID:         "turn-ext-1",
		NodeID:         "external-node-a",
		InvocationID:   "invoke-ext-1",
		AuthorityEpoch: 5,
		CPUUnits:       1,
		MemoryMB:       128,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if resp.InvocationID != "invoke-ext-1" || resp.Telemetry["boundary"] != "external_node" || resp.Telemetry["node_id"] != "external-node-a" {
		t.Fatalf("unexpected invoke response/telemetry: %+v", resp)
	}

	_, err = runtime.Invoke(context.Background(), Request{
		SessionID:      "sess-ext-1",
		TurnID:         "turn-ext-1",
		NodeID:         "external-node-a",
		InvocationID:   "invoke-ext-2",
		AuthorityEpoch: 5,
		CPUUnits:       3,
		MemoryMB:       128,
	})
	if err == nil {
		t.Fatalf("expected resource limit violation")
	}
	if !errors.Is(err, ErrResourceLimitExceeded) {
		t.Fatalf("expected ErrResourceLimitExceeded, got %v", err)
	}
}

func TestRuntimeInvokeTimeout(t *testing.T) {
	t.Parallel()

	adapter := &stubAdapter{
		invokeFn: func(ctx context.Context, _ Request) (Response, error) {
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-time.After(250 * time.Millisecond):
				return Response{InvocationID: "late"}, nil
			}
		},
	}
	runtime, err := NewRuntime(adapter, Config{
		Timeout:     30 * time.Millisecond,
		MaxCPUUnits: 2,
		MaxMemoryMB: 512,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	_, err = runtime.Invoke(context.Background(), Request{
		SessionID:      "sess-ext-timeout-1",
		TurnID:         "turn-ext-timeout-1",
		NodeID:         "external-node-timeout",
		InvocationID:   "invoke-ext-timeout-1",
		AuthorityEpoch: 5,
		CPUUnits:       1,
		MemoryMB:       128,
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !errors.Is(err, ErrExecutionTimeout) {
		t.Fatalf("expected ErrExecutionTimeout, got %v", err)
	}
}

func TestRuntimeCancelPropagates(t *testing.T) {
	t.Parallel()

	adapter := &stubAdapter{
		cancelFn: func(_ context.Context, invocationID string, reason string) error {
			if invocationID != "invoke-ext-cancel-1" || reason != "barge_in" {
				t.Fatalf("unexpected cancel arguments invocation_id=%s reason=%s", invocationID, reason)
			}
			return nil
		},
	}
	runtime, err := NewRuntime(adapter, Config{
		Timeout:     100 * time.Millisecond,
		MaxCPUUnits: 2,
		MaxMemoryMB: 512,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if err := runtime.Cancel(context.Background(), "invoke-ext-cancel-1", "barge_in"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
}

type stubAdapter struct {
	invokeFn func(ctx context.Context, req Request) (Response, error)
	cancelFn func(ctx context.Context, invocationID string, reason string) error
}

func (s *stubAdapter) Invoke(ctx context.Context, req Request) (Response, error) {
	if s.invokeFn == nil {
		return Response{InvocationID: req.InvocationID}, nil
	}
	return s.invokeFn(ctx, req)
}

func (s *stubAdapter) Cancel(ctx context.Context, invocationID string, reason string) error {
	if s.cancelFn == nil {
		return nil
	}
	return s.cancelFn(ctx, invocationID, reason)
}
