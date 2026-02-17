package state

import "testing"

func TestValidateAuthorityRejectsStaleEpoch(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if err := svc.ValidateAuthority("sess-state-1", 5); err != nil {
		t.Fatalf("seed authority: %v", err)
	}
	if err := svc.ValidateAuthority("sess-state-1", 7); err != nil {
		t.Fatalf("advance authority: %v", err)
	}
	err := svc.ValidateAuthority("sess-state-1", 6)
	if err == nil {
		t.Fatalf("expected stale epoch rejection")
	}
	if !IsStaleAuthority(err) {
		t.Fatalf("expected stale authority error, got %v", err)
	}
}

func TestStateScopesIsolationAndTurnClear(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if err := svc.PutSessionHot("sess-state-2", "profile", 3, "premium"); err != nil {
		t.Fatalf("put session hot: %v", err)
	}
	if err := svc.PutTurnEphemeral("sess-state-2", "turn-a", "partial_text", 3, "hello"); err != nil {
		t.Fatalf("put turn ephemeral: %v", err)
	}

	if value, ok := svc.GetSessionHot("sess-state-2", "profile"); !ok || value != "premium" {
		t.Fatalf("unexpected session hot value: value=%v ok=%v", value, ok)
	}
	if value, ok := svc.GetTurnEphemeral("sess-state-2", "turn-a", "partial_text"); !ok || value != "hello" {
		t.Fatalf("unexpected turn-ephemeral value: value=%v ok=%v", value, ok)
	}
	if _, ok := svc.GetTurnEphemeral("sess-state-2", "turn-b", "partial_text"); ok {
		t.Fatalf("unexpected cross-turn leakage")
	}

	svc.ClearTurnEphemeral("sess-state-2", "turn-a")
	if _, ok := svc.GetTurnEphemeral("sess-state-2", "turn-a", "partial_text"); ok {
		t.Fatalf("expected turn-ephemeral clear")
	}
	if value, ok := svc.GetSessionHot("sess-state-2", "profile"); !ok || value != "premium" {
		t.Fatalf("session-hot state should survive turn clear")
	}
}

func TestIdempotencyAndInvocationDedup(t *testing.T) {
	t.Parallel()

	svc := NewService()
	first, err := svc.RegisterIdempotencyKey("sess-state-3", "idem-1", 4)
	if err != nil {
		t.Fatalf("register idempotency first: %v", err)
	}
	if !first {
		t.Fatalf("expected first idempotency key registration to be fresh")
	}
	second, err := svc.RegisterIdempotencyKey("sess-state-3", "idem-1", 4)
	if err != nil {
		t.Fatalf("register idempotency duplicate: %v", err)
	}
	if second {
		t.Fatalf("expected duplicate idempotency key to be rejected")
	}

	invokeFirst, err := svc.RegisterInvocationKey("sess-state-3", "pvi-1", 4)
	if err != nil {
		t.Fatalf("register invocation first: %v", err)
	}
	if !invokeFirst {
		t.Fatalf("expected first invocation registration to be fresh")
	}
	invokeSecond, err := svc.RegisterInvocationKey("sess-state-3", "pvi-1", 4)
	if err != nil {
		t.Fatalf("register invocation duplicate: %v", err)
	}
	if invokeSecond {
		t.Fatalf("expected duplicate invocation registration to be rejected")
	}
}

func TestResetSessionHotRequiresCurrentOrNewerEpoch(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if err := svc.PutSessionHot("sess-state-4", "profile", 2, "standard"); err != nil {
		t.Fatalf("put session hot: %v", err)
	}
	if err := svc.ValidateAuthority("sess-state-4", 5); err != nil {
		t.Fatalf("advance authority: %v", err)
	}

	err := svc.ResetSessionHot("sess-state-4", 4)
	if err == nil {
		t.Fatalf("expected stale reset to be rejected")
	}
	if !IsStaleAuthority(err) {
		t.Fatalf("expected stale authority error, got %v", err)
	}

	if err := svc.ResetSessionHot("sess-state-4", 5); err != nil {
		t.Fatalf("reset session hot with current epoch: %v", err)
	}
	if _, ok := svc.GetSessionHot("sess-state-4", "profile"); ok {
		t.Fatalf("expected session-hot state to be cleared")
	}
}
