package telemetry

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// MetricQueueDepth captures current bounded telemetry queue depth.
	MetricQueueDepth = "queue_depth"
	// MetricDropsTotal captures total dropped telemetry events.
	MetricDropsTotal = "drops_total"
	// MetricCancelLatencyMS captures cancel-path latency observations.
	MetricCancelLatencyMS = "cancel_latency_ms"
	// MetricProviderRTTMS captures provider invocation RTT observations.
	MetricProviderRTTMS = "provider_rtt_ms"
	// MetricShedRate captures scheduling-point shed outcomes.
	MetricShedRate = "shed_rate"
)

// EventKind defines telemetry payload kind.
type EventKind string

const (
	EventKindMetric EventKind = "metric"
	EventKindSpan   EventKind = "span"
	EventKindLog    EventKind = "log"
)

// Correlation carries runtime-safe correlation fields.
type Correlation struct {
	SessionID            string `json:"session_id,omitempty"`
	TurnID               string `json:"turn_id,omitempty"`
	EventID              string `json:"event_id,omitempty"`
	PipelineVersion      string `json:"pipeline_version,omitempty"`
	AuthorityEpoch       int64  `json:"authority_epoch,omitempty"`
	Lane                 string `json:"lane,omitempty"`
	EmittedBy            string `json:"emitted_by,omitempty"`
	RuntimeTimestampMS   int64  `json:"runtime_timestamp_ms,omitempty"`
	WallClockTimestampMS int64  `json:"wall_clock_timestamp_ms,omitempty"`
}

// MetricEvent captures a metric sample payload.
type MetricEvent struct {
	Name       string            `json:"name"`
	Value      float64           `json:"value"`
	Unit       string            `json:"unit,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// SpanEvent captures an OTel-friendly span payload.
type SpanEvent struct {
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	StartMS      int64             `json:"start_ms"`
	EndMS        int64             `json:"end_ms"`
	TraceID      string            `json:"trace_id,omitempty"`
	SpanID       string            `json:"span_id,omitempty"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// LogEvent captures a telemetry log payload.
type LogEvent struct {
	Name       string            `json:"name"`
	Severity   string            `json:"severity"`
	Message    string            `json:"message"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Event is the normalized telemetry emission envelope.
type Event struct {
	Kind        EventKind    `json:"kind"`
	TimestampMS int64        `json:"timestamp_ms"`
	Correlation Correlation  `json:"correlation"`
	Metric      *MetricEvent `json:"metric,omitempty"`
	Span        *SpanEvent   `json:"span,omitempty"`
	Log         *LogEvent    `json:"log,omitempty"`
}

// Sink exports normalized telemetry events.
type Sink interface {
	Export(context.Context, Event) error
}

// Emitter defines a non-blocking telemetry emission handle.
type Emitter interface {
	EmitMetric(name string, value float64, unit string, attributes map[string]string, correlation Correlation)
	EmitSpan(name, kind string, startMS, endMS int64, attributes map[string]string, correlation Correlation)
	EmitLog(name, severity, message string, attributes map[string]string, correlation Correlation)
}

type noopEmitter struct{}

func (noopEmitter) EmitMetric(string, float64, string, map[string]string, Correlation) {}
func (noopEmitter) EmitSpan(string, string, int64, int64, map[string]string, Correlation) {
}
func (noopEmitter) EmitLog(string, string, string, map[string]string, Correlation) {}

type emitterHolder struct {
	emitter Emitter
}

var globalEmitter atomic.Value

func init() {
	globalEmitter.Store(emitterHolder{emitter: noopEmitter{}})
}

// SetDefaultEmitter replaces the process-local default telemetry emitter.
func SetDefaultEmitter(emitter Emitter) {
	if emitter == nil {
		emitter = noopEmitter{}
	}
	globalEmitter.Store(emitterHolder{emitter: emitter})
}

// DefaultEmitter returns the process-local default telemetry emitter.
func DefaultEmitter() Emitter {
	holder, ok := globalEmitter.Load().(emitterHolder)
	if !ok || holder.emitter == nil {
		return noopEmitter{}
	}
	return holder.emitter
}

// Config controls bounded queue and export behavior.
type Config struct {
	QueueCapacity int
	ExportTimeout time.Duration
	// LogSampleRate drops deterministic debug log events when >1.
	// With N, only every Nth debug log event is accepted.
	LogSampleRate int
}

func (c Config) withDefaults() Config {
	if c.QueueCapacity < 1 {
		c.QueueCapacity = 256
	}
	if c.ExportTimeout <= 0 {
		c.ExportTimeout = 200 * time.Millisecond
	}
	if c.LogSampleRate < 1 {
		c.LogSampleRate = 1
	}
	return c
}

// Stats captures current pipeline counters.
type Stats struct {
	Enqueued       uint64
	Dropped        uint64
	SampledDropped uint64
	Exported       uint64
	ExportFailures uint64
	QueueDepth     int
}

// Pipeline is a bounded non-blocking telemetry pipeline.
type Pipeline struct {
	sink Sink
	cfg  Config

	queue chan Event
	stop  chan struct{}

	closeOnce sync.Once
	wg        sync.WaitGroup

	enqueued       atomic.Uint64
	dropped        atomic.Uint64
	sampledDropped atomic.Uint64
	exported       atomic.Uint64
	exportFailures atomic.Uint64
	logCounter     atomic.Uint64
}

type discardSink struct{}

func (discardSink) Export(context.Context, Event) error { return nil }

// NewPipeline constructs and starts a telemetry pipeline.
func NewPipeline(sink Sink, cfg Config) *Pipeline {
	cfg = cfg.withDefaults()
	if sink == nil {
		sink = discardSink{}
	}
	p := &Pipeline{
		sink:  sink,
		cfg:   cfg,
		queue: make(chan Event, cfg.QueueCapacity),
		stop:  make(chan struct{}),
	}
	p.wg.Add(1)
	go p.run()
	return p
}

// Close drains pending events and stops background export.
func (p *Pipeline) Close() error {
	p.closeOnce.Do(func() {
		close(p.stop)
		p.wg.Wait()
	})
	return nil
}

// Stats returns current queue/counter snapshots.
func (p *Pipeline) Stats() Stats {
	return Stats{
		Enqueued:       p.enqueued.Load(),
		Dropped:        p.dropped.Load(),
		SampledDropped: p.sampledDropped.Load(),
		Exported:       p.exported.Load(),
		ExportFailures: p.exportFailures.Load(),
		QueueDepth:     len(p.queue),
	}
}

// EmitMetric enqueues a metric sample without blocking.
func (p *Pipeline) EmitMetric(name string, value float64, unit string, attributes map[string]string, correlation Correlation) {
	p.enqueue(Event{
		Kind:        EventKindMetric,
		TimestampMS: eventTimestampMS(correlation),
		Correlation: normalizeCorrelation(correlation),
		Metric: &MetricEvent{
			Name:       strings.TrimSpace(name),
			Value:      value,
			Unit:       strings.TrimSpace(unit),
			Attributes: cloneAttributes(attributes),
		},
	}, true)
}

// EmitSpan enqueues a span sample without blocking.
func (p *Pipeline) EmitSpan(name, kind string, startMS, endMS int64, attributes map[string]string, correlation Correlation) {
	p.enqueue(Event{
		Kind:        EventKindSpan,
		TimestampMS: eventTimestampMS(correlation),
		Correlation: normalizeCorrelation(correlation),
		Span: &SpanEvent{
			Name:       strings.TrimSpace(name),
			Kind:       strings.TrimSpace(kind),
			StartMS:    nonNegative(startMS),
			EndMS:      nonNegative(endMS),
			Attributes: cloneAttributes(attributes),
		},
	}, true)
}

// EmitLog enqueues a log sample without blocking.
func (p *Pipeline) EmitLog(name, severity, message string, attributes map[string]string, correlation Correlation) {
	event := Event{
		Kind:        EventKindLog,
		TimestampMS: eventTimestampMS(correlation),
		Correlation: normalizeCorrelation(correlation),
		Log: &LogEvent{
			Name:       strings.TrimSpace(name),
			Severity:   strings.TrimSpace(severity),
			Message:    message,
			Attributes: cloneAttributes(attributes),
		},
	}
	sampled := p.shouldSampleLog(event)
	p.enqueue(event, sampled)
}

func (p *Pipeline) shouldSampleLog(event Event) bool {
	if p.cfg.LogSampleRate <= 1 {
		return true
	}
	if event.Log == nil || !strings.EqualFold(strings.TrimSpace(event.Log.Severity), "debug") {
		return true
	}
	n := p.logCounter.Add(1)
	// Keep first event, then every Nth thereafter.
	return (n-1)%uint64(p.cfg.LogSampleRate) == 0
}

func (p *Pipeline) enqueue(event Event, sampled bool) {
	if !sampled {
		p.sampledDropped.Add(1)
		return
	}
	select {
	case p.queue <- event:
		p.enqueued.Add(1)
	default:
		p.dropped.Add(1)
	}
}

func (p *Pipeline) run() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stop:
			for {
				select {
				case event := <-p.queue:
					p.export(event)
				default:
					return
				}
			}
		case event := <-p.queue:
			p.export(event)
		}
	}
}

func (p *Pipeline) export(event Event) {
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.ExportTimeout)
	defer cancel()
	if err := p.sink.Export(ctx, event); err != nil {
		p.exportFailures.Add(1)
		return
	}
	p.exported.Add(1)
}

func eventTimestampMS(correlation Correlation) int64 {
	if correlation.RuntimeTimestampMS > 0 {
		return correlation.RuntimeTimestampMS
	}
	return time.Now().UnixMilli()
}

func normalizeCorrelation(c Correlation) Correlation {
	c.AuthorityEpoch = nonNegative(c.AuthorityEpoch)
	c.RuntimeTimestampMS = nonNegative(c.RuntimeTimestampMS)
	c.WallClockTimestampMS = nonNegative(c.WallClockTimestampMS)
	c.SessionID = strings.TrimSpace(c.SessionID)
	c.TurnID = strings.TrimSpace(c.TurnID)
	c.EventID = strings.TrimSpace(c.EventID)
	c.PipelineVersion = strings.TrimSpace(c.PipelineVersion)
	c.Lane = strings.TrimSpace(c.Lane)
	c.EmittedBy = strings.TrimSpace(c.EmittedBy)
	return c
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func cloneAttributes(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
