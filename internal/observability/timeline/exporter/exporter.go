package exporter

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	telemetrycontext "github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry/context"
)

const (
	durableExporterNodeID = "timeline_exporter"
	durableExporterEdgeID = "timeline/durable_export"
	durableExporterSource = "OR-02"
)

// Record is a durable timeline export payload.
type Record struct {
	Kind                 string
	SessionID            string
	TurnID               string
	EventID              string
	PipelineVersion      string
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	Payload              []byte
}

// Validate enforces minimal durable-export record requirements.
func (r Record) Validate() error {
	if strings.TrimSpace(r.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(r.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	if strings.TrimSpace(r.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(r.PipelineVersion) == "" {
		return fmt.Errorf("pipeline_version is required")
	}
	if len(r.Payload) == 0 {
		return fmt.Errorf("payload is required")
	}
	return nil
}

// Sink exports one durable timeline record.
type Sink interface {
	Export(context.Context, Record) error
}

// Config controls bounded queue, timeout, and retry behavior.
type Config struct {
	QueueCapacity int
	ExportTimeout time.Duration
	RetryDelay    time.Duration
	MaxAttempts   int
}

func (c Config) withDefaults() Config {
	if c.QueueCapacity < 1 {
		c.QueueCapacity = 256
	}
	if c.ExportTimeout <= 0 {
		c.ExportTimeout = 200 * time.Millisecond
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = 25 * time.Millisecond
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = 3
	}
	return c
}

// Stats captures durable exporter counters.
type Stats struct {
	Enqueued   uint64
	Dropped    uint64
	Exported   uint64
	Retries    uint64
	Failures   uint64
	QueueDepth int
}

type discardSink struct{}

func (discardSink) Export(context.Context, Record) error { return nil }

type queuedRecord struct {
	record     Record
	enqueuedAt time.Time
}

// AsyncExporter executes non-blocking queueing with background export/retry.
type AsyncExporter struct {
	sink Sink
	cfg  Config

	queue chan queuedRecord
	stop  chan struct{}

	closeOnce sync.Once
	wg        sync.WaitGroup

	enqueued atomic.Uint64
	dropped  atomic.Uint64
	exported atomic.Uint64
	retries  atomic.Uint64
	failures atomic.Uint64
}

// NewAsyncExporter creates and starts a bounded async durable exporter.
func NewAsyncExporter(sink Sink, cfg Config) *AsyncExporter {
	cfg = cfg.withDefaults()
	if sink == nil {
		sink = discardSink{}
	}
	exporter := &AsyncExporter{
		sink:  sink,
		cfg:   cfg,
		queue: make(chan queuedRecord, cfg.QueueCapacity),
		stop:  make(chan struct{}),
	}
	exporter.wg.Add(1)
	go exporter.run()
	return exporter
}

// Close drains queued records and stops background export.
func (e *AsyncExporter) Close() error {
	e.closeOnce.Do(func() {
		close(e.stop)
		e.wg.Wait()
	})
	return nil
}

// Stats returns current queue/counter snapshots.
func (e *AsyncExporter) Stats() Stats {
	return Stats{
		Enqueued:   e.enqueued.Load(),
		Dropped:    e.dropped.Load(),
		Exported:   e.exported.Load(),
		Retries:    e.retries.Load(),
		Failures:   e.failures.Load(),
		QueueDepth: len(e.queue),
	}
}

// Enqueue performs best-effort non-blocking queue append.
func (e *AsyncExporter) Enqueue(record Record) bool {
	normalized := normalizeRecord(record)
	if err := normalized.Validate(); err != nil {
		e.dropped.Add(1)
		e.emitQueueDepthMetric(normalized, "invalid")
		return false
	}

	entry := queuedRecord{record: normalized, enqueuedAt: time.Now().UTC()}
	select {
	case e.queue <- entry:
		e.enqueued.Add(1)
		e.emitQueueDepthMetric(normalized, "enqueued")
		return true
	default:
		e.dropped.Add(1)
		e.emitQueueDepthMetric(normalized, "dropped")
		return false
	}
}

func (e *AsyncExporter) run() {
	defer e.wg.Done()

	for {
		select {
		case <-e.stop:
			for {
				select {
				case entry := <-e.queue:
					e.exportWithRetry(entry)
				default:
					return
				}
			}
		case entry := <-e.queue:
			e.exportWithRetry(entry)
		}
	}
}

func (e *AsyncExporter) exportWithRetry(entry queuedRecord) {
	e.emitQueueDepthMetric(entry.record, "dequeued")

	for attempt := 1; attempt <= e.cfg.MaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), e.cfg.ExportTimeout)
		err := e.sink.Export(ctx, entry.record)
		cancel()

		if err == nil {
			e.exported.Add(1)
			e.emitLagMetric(entry.record, durationMillisSince(entry.enqueuedAt), attempt)
			return
		}

		if attempt < e.cfg.MaxAttempts {
			e.retries.Add(1)
			e.emitCounterMetric(entry.record, telemetry.MetricDurableExportRetriesTotal, map[string]string{
				"kind":    entry.record.Kind,
				"attempt": strconv.Itoa(attempt),
			})
			if !e.waitRetryDelay() {
				return
			}
			continue
		}

		e.failures.Add(1)
		e.emitCounterMetric(entry.record, telemetry.MetricDurableExportFailuresTotal, map[string]string{
			"kind":     entry.record.Kind,
			"attempts": strconv.Itoa(e.cfg.MaxAttempts),
		})
	}
}

func (e *AsyncExporter) waitRetryDelay() bool {
	timer := time.NewTimer(e.cfg.RetryDelay)
	defer timer.Stop()
	select {
	case <-e.stop:
		return false
	case <-timer.C:
		return true
	}
}

func (e *AsyncExporter) emitQueueDepthMetric(record Record, status string) {
	e.emitMetric(record, telemetry.MetricDurableExportQueueDepth, float64(len(e.queue)), "count", map[string]string{
		"kind":    record.Kind,
		"status":  status,
		"edge_id": durableExporterEdgeID,
	})
}

func (e *AsyncExporter) emitLagMetric(record Record, lagMS int64, attempts int) {
	e.emitMetric(record, telemetry.MetricDurableExportLagMS, float64(nonNegative(lagMS)), "ms", map[string]string{
		"kind":     record.Kind,
		"attempts": strconv.Itoa(attempts),
		"edge_id":  durableExporterEdgeID,
	})
}

func (e *AsyncExporter) emitCounterMetric(record Record, metricName string, attributes map[string]string) {
	e.emitMetric(record, metricName, 1, "count", attributes)
}

func (e *AsyncExporter) emitMetric(record Record, metricName string, value float64, unit string, attributes map[string]string) {
	correlation, ok := correlationFromRecord(record)
	if !ok {
		return
	}
	attrs := cloneAttributes(attributes)
	if attrs == nil {
		attrs = map[string]string{}
	}
	attrs["node_id"] = durableExporterNodeID
	attrs["edge_id"] = durableExporterEdgeID

	telemetry.DefaultEmitter().EmitMetric(metricName, value, unit, attrs, correlation)
}

func correlationFromRecord(record Record) (telemetry.Correlation, bool) {
	correlation, err := telemetrycontext.Resolve(telemetrycontext.ResolveInput{
		SessionID:            record.SessionID,
		TurnID:               record.TurnID,
		EventID:              record.EventID,
		PipelineVersion:      record.PipelineVersion,
		NodeID:               durableExporterNodeID,
		EdgeID:               durableExporterEdgeID,
		AuthorityEpoch:       nonNegative(record.AuthorityEpoch),
		Lane:                 eventabi.LaneTelemetry,
		EmittedBy:            durableExporterSource,
		RuntimeTimestampMS:   nonNegative(record.RuntimeTimestampMS),
		WallClockTimestampMS: nonNegative(record.WallClockTimestampMS),
	})
	if err != nil {
		return telemetry.Correlation{}, false
	}
	return correlation, true
}

func normalizeRecord(record Record) Record {
	record.Kind = strings.TrimSpace(record.Kind)
	record.SessionID = strings.TrimSpace(record.SessionID)
	record.TurnID = strings.TrimSpace(record.TurnID)
	record.EventID = strings.TrimSpace(record.EventID)
	record.PipelineVersion = strings.TrimSpace(record.PipelineVersion)
	record.AuthorityEpoch = nonNegative(record.AuthorityEpoch)
	record.RuntimeTimestampMS = nonNegative(record.RuntimeTimestampMS)
	record.WallClockTimestampMS = nonNegative(record.WallClockTimestampMS)
	if len(record.Payload) > 0 {
		record.Payload = append([]byte(nil), record.Payload...)
	}
	return record
}

func cloneAttributes(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func durationMillisSince(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	ms := time.Since(start).Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
