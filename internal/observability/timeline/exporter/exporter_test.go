package exporter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type blockingSink struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingSink) Export(ctx context.Context, _ Record) error {
	select {
	case s.started <- struct{}{}:
	default:
	}
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type flakySink struct {
	failuresBeforeSuccess int64
	calls                 atomic.Int64
}

func (s *flakySink) Export(context.Context, Record) error {
	call := s.calls.Add(1)
	if call <= s.failuresBeforeSuccess {
		return errors.New("durable backend unavailable")
	}
	return nil
}

type failingSink struct {
	calls atomic.Int64
}

func (s *failingSink) Export(context.Context, Record) error {
	s.calls.Add(1)
	return errors.New("durable backend unavailable")
}

func TestAsyncExporterQueueBoundedAndNonBlocking(t *testing.T) {
	sink := &blockingSink{started: make(chan struct{}, 2), release: make(chan struct{})}
	exporter := NewAsyncExporter(sink, Config{
		QueueCapacity: 1,
		ExportTimeout: time.Second,
		RetryDelay:    time.Millisecond,
		MaxAttempts:   1,
	})
	t.Cleanup(func() {
		close(sink.release)
		_ = exporter.Close()
	})

	if ok := exporter.Enqueue(testRecord("evt-exporter-1")); !ok {
		t.Fatalf("expected first enqueue to succeed")
	}
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for first export start")
	}

	if ok := exporter.Enqueue(testRecord("evt-exporter-2")); !ok {
		t.Fatalf("expected second enqueue to fill queue")
	}

	start := time.Now()
	if ok := exporter.Enqueue(testRecord("evt-exporter-3")); ok {
		t.Fatalf("expected third enqueue to drop when queue is full")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("expected non-blocking enqueue under pressure, took %s", elapsed)
	}

	waitFor(t, time.Second, func() bool {
		return exporter.Stats().Dropped >= 1
	})
}

func TestAsyncExporterRetriesAndEventuallySucceeds(t *testing.T) {
	sink := &flakySink{failuresBeforeSuccess: 2}
	exporter := NewAsyncExporter(sink, Config{
		QueueCapacity: 8,
		ExportTimeout: 20 * time.Millisecond,
		RetryDelay:    time.Millisecond,
		MaxAttempts:   4,
	})
	t.Cleanup(func() {
		_ = exporter.Close()
	})

	if ok := exporter.Enqueue(testRecord("evt-retry-success-1")); !ok {
		t.Fatalf("expected enqueue to succeed")
	}

	waitFor(t, time.Second, func() bool {
		return exporter.Stats().Exported == 1
	})

	stats := exporter.Stats()
	if stats.Retries < 2 {
		t.Fatalf("expected at least 2 retries, got %+v", stats)
	}
	if stats.Failures != 0 {
		t.Fatalf("expected no terminal failures after successful retry, got %+v", stats)
	}
}

func TestAsyncExporterExhaustedRetriesCountFailure(t *testing.T) {
	sink := &failingSink{}
	exporter := NewAsyncExporter(sink, Config{
		QueueCapacity: 8,
		ExportTimeout: 20 * time.Millisecond,
		RetryDelay:    time.Millisecond,
		MaxAttempts:   2,
	})
	t.Cleanup(func() {
		_ = exporter.Close()
	})

	if ok := exporter.Enqueue(testRecord("evt-retry-failure-1")); !ok {
		t.Fatalf("expected enqueue to succeed")
	}

	waitFor(t, time.Second, func() bool {
		return exporter.Stats().Failures == 1
	})

	stats := exporter.Stats()
	if stats.Retries < 1 {
		t.Fatalf("expected retry counter increment before failure, got %+v", stats)
	}
	if stats.Exported != 0 {
		t.Fatalf("expected no successful exports, got %+v", stats)
	}
}

func testRecord(eventID string) Record {
	return Record{
		Kind:                 "baseline",
		SessionID:            "sess-exporter-test",
		TurnID:               "turn-exporter-test",
		EventID:              eventID,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		Payload:              []byte(`{"kind":"baseline"}`),
	}
}

func waitFor(t *testing.T, timeout time.Duration, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not satisfied within %s", timeout)
}
