package telemetry

import (
	"context"
	"fmt"
	"reflect"
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

func TestPipelineDeterministicSamplingAcrossTelemetryChannels(t *testing.T) {
	t.Parallel()

	const (
		totalEvents    = 120
		emittedPerKind = totalEvents / 3
	)

	sinkA := NewMemorySink()
	pipelineA := NewPipeline(sinkA, Config{
		QueueCapacity: totalEvents,
		LogSampleRate: 3,
	})
	sinkB := NewMemorySink()
	pipelineB := NewPipeline(sinkB, Config{
		QueueCapacity: totalEvents,
		LogSampleRate: 3,
	})

	for i := 0; i < totalEvents; i++ {
		correlation := Correlation{
			SessionID:            "sess-sample",
			TurnID:               "turn-sample",
			EventID:              fmt.Sprintf("evt-sample-%d", i+1),
			PipelineVersion:      "pipeline-v1",
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   int64(i + 1),
			WallClockTimestampMS: int64(10_000 + i),
			EmittedBy:            "OR-01",
			Lane:                 "telemetry",
		}
		switch i % 3 {
		case 0:
			pipelineA.EmitMetric(
				MetricQueueDepth,
				float64(i+1),
				"count",
				map[string]string{"source": "test", "idx": fmt.Sprintf("%d", i)},
				correlation,
			)
			pipelineB.EmitMetric(
				MetricQueueDepth,
				float64(i+1),
				"count",
				map[string]string{"source": "test", "idx": fmt.Sprintf("%d", i)},
				correlation,
			)
		case 1:
			pipelineA.EmitSpan(
				"turn_span",
				"stream",
				int64(i),
				int64(i+5),
				map[string]string{"source": "test", "idx": fmt.Sprintf("%d", i)},
				correlation,
			)
			pipelineB.EmitSpan(
				"turn_span",
				"stream",
				int64(i),
				int64(i+5),
				map[string]string{"source": "test", "idx": fmt.Sprintf("%d", i)},
				correlation,
			)
		default:
			severity := "info"
			if i%2 == 0 {
				severity = "debug"
			}
			pipelineA.EmitLog(
				"runtime_event",
				severity,
				fmt.Sprintf("message-%d", i),
				map[string]string{"source": "test", "idx": fmt.Sprintf("%d", i)},
				correlation,
			)
			pipelineB.EmitLog(
				"runtime_event",
				severity,
				fmt.Sprintf("message-%d", i),
				map[string]string{"source": "test", "idx": fmt.Sprintf("%d", i)},
				correlation,
			)
		}
	}
	if err := pipelineA.Close(); err != nil {
		t.Fatalf("unexpected close error for pipelineA: %v", err)
	}
	if err := pipelineB.Close(); err != nil {
		t.Fatalf("unexpected close error for pipelineB: %v", err)
	}

	eventsA := sinkA.Events()
	eventsB := sinkB.Events()
	if !reflect.DeepEqual(eventsA, eventsB) {
		t.Fatalf("expected deterministic sampled events, got A=%d B=%d", len(eventsA), len(eventsB))
	}
	if len(eventsA) == 0 || len(eventsA) >= totalEvents {
		t.Fatalf("expected partial sampling outcome, got %d of %d", len(eventsA), totalEvents)
	}

	counts := map[EventKind]int{
		EventKindMetric: 0,
		EventKindSpan:   0,
		EventKindLog:    0,
	}
	for _, event := range eventsA {
		counts[event.Kind]++
	}
	if counts[EventKindMetric] == 0 || counts[EventKindSpan] == 0 || counts[EventKindLog] == 0 {
		t.Fatalf("expected sampled output across metric/span/log channels, got counts=%+v", counts)
	}
	if counts[EventKindMetric] >= emittedPerKind || counts[EventKindSpan] >= emittedPerKind || counts[EventKindLog] >= emittedPerKind {
		t.Fatalf("expected sampling to affect every channel, got counts=%+v emitted_per_kind=%d", counts, emittedPerKind)
	}

	statsA := pipelineA.Stats()
	statsB := pipelineB.Stats()
	if statsA.Dropped != 0 || statsB.Dropped != 0 {
		t.Fatalf("expected no queue overflow drops in deterministic sampling test, got A=%+v B=%+v", statsA, statsB)
	}
	if statsA.SampledDropped == 0 || statsB.SampledDropped == 0 {
		t.Fatalf("expected sampled drops in both pipelines, got A=%+v B=%+v", statsA, statsB)
	}
	if statsA.SampledDropped != statsB.SampledDropped {
		t.Fatalf("expected deterministic sampled drop counts, got A=%+v B=%+v", statsA, statsB)
	}
	if statsA.SampledDropped != uint64(totalEvents-len(eventsA)) {
		t.Fatalf("expected sampled drop accounting to match emitted minus exported, got stats=%+v events=%d", statsA, len(eventsA))
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
