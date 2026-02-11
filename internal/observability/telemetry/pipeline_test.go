package telemetry

import (
	"context"
	"testing"
	"time"
)

type blockingSink struct {
	block <-chan struct{}
}

func (s blockingSink) Export(ctx context.Context, _ Event) error {
	select {
	case <-s.block:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPipelineEmitIsNonBlockingWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	pipeline := NewPipeline(blockingSink{block: block}, Config{
		QueueCapacity: 1,
		ExportTimeout: 5 * time.Millisecond,
	})
	defer func() {
		close(block)
		_ = pipeline.Close()
	}()

	start := time.Now()
	for i := 0; i < 2000; i++ {
		pipeline.EmitLog("queue-pressure", "debug", "message", nil, Correlation{
			SessionID:          "sess-1",
			TurnID:             "turn-1",
			PipelineVersion:    "pipeline-v1",
			RuntimeTimestampMS: int64(i + 1),
			EmittedBy:          "OR-01",
			Lane:               "telemetry",
		})
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("expected non-blocking emit under pressure, took %s", elapsed)
	}

	stats := pipeline.Stats()
	if stats.Dropped == 0 {
		t.Fatalf("expected dropped events under queue pressure, got %+v", stats)
	}
}

func TestPipelineDeterministicDebugLogSampling(t *testing.T) {
	t.Parallel()

	sink := NewMemorySink()
	pipeline := NewPipeline(sink, Config{
		QueueCapacity: 32,
		LogSampleRate: 3,
	})

	for i := 0; i < 10; i++ {
		pipeline.EmitLog("sampled-debug", "debug", "message", map[string]string{"idx": "x"}, Correlation{
			SessionID:          "sess-sample",
			PipelineVersion:    "pipeline-v1",
			RuntimeTimestampMS: int64(i + 1),
			EmittedBy:          "OR-01",
			Lane:               "telemetry",
		})
	}
	if err := pipeline.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	events := sink.Events()
	if len(events) != 4 {
		t.Fatalf("expected deterministic sampled count 4, got %d", len(events))
	}
	stats := pipeline.Stats()
	if stats.SampledDropped != 6 {
		t.Fatalf("expected 6 sampled drops, got %+v", stats)
	}
}

func TestPipelineExportsMetricSpanAndLogEvents(t *testing.T) {
	t.Parallel()

	sink := NewMemorySink()
	pipeline := NewPipeline(sink, Config{QueueCapacity: 16})

	correlation := Correlation{
		SessionID:          "sess-or01",
		TurnID:             "turn-or01",
		EventID:            "evt-or01",
		PipelineVersion:    "pipeline-v1",
		AuthorityEpoch:     7,
		RuntimeTimestampMS: 100,
		EmittedBy:          "OR-01",
		Lane:               "telemetry",
	}
	pipeline.EmitMetric(MetricQueueDepth, 5, "count", map[string]string{"node_type": "metrics"}, correlation)
	pipeline.EmitSpan("turn_span", "turn_span", 100, 105, map[string]string{"result": "active"}, correlation)
	pipeline.EmitLog("runtime_event", "info", "turn opened", map[string]string{"state": "active"}, correlation)

	if err := pipeline.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	events := sink.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 exported events, got %d", len(events))
	}
	if events[0].Kind != EventKindMetric || events[0].Metric == nil || events[0].Metric.Name != MetricQueueDepth {
		t.Fatalf("unexpected metric event: %+v", events[0])
	}
	if events[1].Kind != EventKindSpan || events[1].Span == nil || events[1].Span.Name != "turn_span" {
		t.Fatalf("unexpected span event: %+v", events[1])
	}
	if events[2].Kind != EventKindLog || events[2].Log == nil || events[2].Log.Name != "runtime_event" {
		t.Fatalf("unexpected log event: %+v", events[2])
	}
	for _, event := range events {
		if event.Correlation.SessionID != "sess-or01" || event.Correlation.PipelineVersion != "pipeline-v1" {
			t.Fatalf("unexpected correlation payload: %+v", event.Correlation)
		}
	}
}

func TestDefaultEmitterCanBeOverridden(t *testing.T) {
	sink := NewMemorySink()
	pipeline := NewPipeline(sink, Config{QueueCapacity: 8})
	defer func() {
		SetDefaultEmitter(nil)
		_ = pipeline.Close()
	}()

	SetDefaultEmitter(pipeline)
	DefaultEmitter().EmitMetric(MetricDropsTotal, 1, "count", nil, Correlation{
		SessionID:          "sess-default",
		PipelineVersion:    "pipeline-v1",
		RuntimeTimestampMS: 1,
	})

	_ = pipeline.Close()
	events := sink.Events()
	if len(events) != 1 || events[0].Metric == nil || events[0].Metric.Name != MetricDropsTotal {
		t.Fatalf("expected default emitter to route through pipeline, got %+v", events)
	}
}
