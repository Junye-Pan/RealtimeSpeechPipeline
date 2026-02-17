package buffering

import "fmt"

// DropPolicy controls overflow behavior.
type DropPolicy string

const (
	DropOldest DropPolicy = "drop_oldest"
	DropNewest DropPolicy = "drop_newest"
)

// Config controls deterministic jitter-buffer behavior.
type Config struct {
	MaxItems   int
	DropPolicy DropPolicy
}

// Item captures one transport-frame payload in the jitter buffer.
type Item struct {
	Sequence int64
	Payload  []byte
}

// JitterBuffer is a bounded FIFO queue for ingress payload smoothing.
type JitterBuffer struct {
	cfg     Config
	queue   []Item
	dropped int
}

// New creates a bounded jitter buffer.
func New(cfg Config) (*JitterBuffer, error) {
	if cfg.MaxItems < 1 {
		return nil, fmt.Errorf("max_items must be >=1")
	}
	switch cfg.DropPolicy {
	case "", DropOldest, DropNewest:
	default:
		return nil, fmt.Errorf("unsupported drop policy %q", cfg.DropPolicy)
	}
	if cfg.DropPolicy == "" {
		cfg.DropPolicy = DropOldest
	}
	return &JitterBuffer{cfg: cfg, queue: make([]Item, 0, cfg.MaxItems)}, nil
}

// Push inserts one item and reports whether the new item was accepted.
func (b *JitterBuffer) Push(item Item) bool {
	if len(b.queue) >= b.cfg.MaxItems {
		b.dropped++
		if b.cfg.DropPolicy == DropNewest {
			return false
		}
		// Drop oldest and keep the newest sample.
		copy(b.queue[0:], b.queue[1:])
		b.queue = b.queue[:len(b.queue)-1]
	}
	b.queue = append(b.queue, item)
	return true
}

// Pop returns the oldest queued item.
func (b *JitterBuffer) Pop() (Item, bool) {
	if len(b.queue) == 0 {
		return Item{}, false
	}
	item := b.queue[0]
	copy(b.queue[0:], b.queue[1:])
	b.queue = b.queue[:len(b.queue)-1]
	return item, true
}

// Len returns current queue depth.
func (b *JitterBuffer) Len() int {
	return len(b.queue)
}

// DroppedCount returns total dropped items.
func (b *JitterBuffer) DroppedCount() int {
	return b.dropped
}
