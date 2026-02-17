package integration_test

import (
	"testing"

	runtimestate "github.com/tiger/realtime-speech-pipeline/internal/runtime/state"
)

func TestRuntimeStateServiceDeterministicIdempotencyDedup(t *testing.T) {
	t.Parallel()

	stateSvc := runtimestate.NewService()

	first, err := stateSvc.RegisterIdempotencyKey("sess-state-int-1", "idem-key-1", 3)
	if err != nil {
		t.Fatalf("register idempotency first: %v", err)
	}
	if !first {
		t.Fatalf("expected first idempotency key registration to be accepted")
	}

	second, err := stateSvc.RegisterIdempotencyKey("sess-state-int-1", "idem-key-1", 3)
	if err != nil {
		t.Fatalf("register idempotency duplicate: %v", err)
	}
	if second {
		t.Fatalf("expected duplicate idempotency key registration to be rejected")
	}

	firstInvocation, err := stateSvc.RegisterInvocationKey("sess-state-int-1", "pvi-key-1", 3)
	if err != nil {
		t.Fatalf("register invocation first: %v", err)
	}
	if !firstInvocation {
		t.Fatalf("expected first invocation registration to be accepted")
	}

	duplicateInvocation, err := stateSvc.RegisterInvocationKey("sess-state-int-1", "pvi-key-1", 3)
	if err != nil {
		t.Fatalf("register invocation duplicate: %v", err)
	}
	if duplicateInvocation {
		t.Fatalf("expected duplicate invocation registration to be rejected")
	}

	if err := stateSvc.ValidateAuthority("sess-state-int-1", 5); err != nil {
		t.Fatalf("advance authority epoch: %v", err)
	}
	_, err = stateSvc.RegisterIdempotencyKey("sess-state-int-1", "idem-key-2", 4)
	if err == nil {
		t.Fatalf("expected stale authority registration to fail")
	}
	if !runtimestate.IsStaleAuthority(err) {
		t.Fatalf("expected stale authority error, got %v", err)
	}
}
