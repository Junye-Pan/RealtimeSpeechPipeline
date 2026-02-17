package buffering

import "testing"

func TestJitterBufferDropOldest(t *testing.T) {
	t.Parallel()

	buffer, err := New(Config{MaxItems: 2, DropPolicy: DropOldest})
	if err != nil {
		t.Fatalf("new jitter buffer: %v", err)
	}
	if !buffer.Push(Item{Sequence: 1}) || !buffer.Push(Item{Sequence: 2}) || !buffer.Push(Item{Sequence: 3}) {
		t.Fatalf("expected push operations with drop_oldest to accept newest samples")
	}
	if buffer.DroppedCount() != 1 {
		t.Fatalf("expected one dropped sample, got %d", buffer.DroppedCount())
	}
	item, ok := buffer.Pop()
	if !ok || item.Sequence != 2 {
		t.Fatalf("expected oldest retained sample sequence 2 after drop, got %+v ok=%v", item, ok)
	}
}

func TestJitterBufferDropNewest(t *testing.T) {
	t.Parallel()

	buffer, err := New(Config{MaxItems: 1, DropPolicy: DropNewest})
	if err != nil {
		t.Fatalf("new jitter buffer: %v", err)
	}
	if !buffer.Push(Item{Sequence: 1}) {
		t.Fatalf("expected first push to succeed")
	}
	if buffer.Push(Item{Sequence: 2}) {
		t.Fatalf("expected overflow push to be rejected with drop_newest")
	}
	if buffer.DroppedCount() != 1 {
		t.Fatalf("expected one dropped sample, got %d", buffer.DroppedCount())
	}
	item, ok := buffer.Pop()
	if !ok || item.Sequence != 1 {
		t.Fatalf("expected first sample to remain in queue, got %+v ok=%v", item, ok)
	}
}

func TestJitterBufferValidation(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{MaxItems: 0}); err == nil {
		t.Fatalf("expected max_items validation error")
	}
	if _, err := New(Config{MaxItems: 1, DropPolicy: "unsupported"}); err == nil {
		t.Fatalf("expected unsupported drop policy validation error")
	}
}
