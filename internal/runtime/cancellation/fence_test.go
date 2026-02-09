package cancellation

import "testing"

func TestFenceAcceptAndIsFenced(t *testing.T) {
	t.Parallel()

	fence := NewFence()
	if fence.IsFenced("sess-1", "turn-1") {
		t.Fatalf("expected fence false before accept")
	}
	if err := fence.Accept("sess-1", "turn-1"); err != nil {
		t.Fatalf("unexpected accept error: %v", err)
	}
	if !fence.IsFenced("sess-1", "turn-1") {
		t.Fatalf("expected fence true after accept")
	}
}

func TestFenceRejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	fence := NewFence()
	if err := fence.Accept("", "turn-1"); err == nil {
		t.Fatalf("expected empty session to fail")
	}
	if err := fence.Accept("sess-1", ""); err == nil {
		t.Fatalf("expected empty turn to fail")
	}
}
