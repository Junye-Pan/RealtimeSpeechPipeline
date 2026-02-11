package transport

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
)

func TestCF002LateOutputAfterCancelIsFenced(t *testing.T) {
	t.Parallel()

	fence := NewOutputFence()

	beforeCancel, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-cf-2",
		TurnID:               "turn-cf-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-before-cancel",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected pre-cancel output decision error: %v", err)
	}
	if !beforeCancel.Accepted || beforeCancel.Signal.Signal != "output_accepted" {
		t.Fatalf("expected output accepted before cancel, got %+v", beforeCancel)
	}

	cancelFence, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-cf-2",
		TurnID:               "turn-cf-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-cancel",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected cancel fence error: %v", err)
	}
	if cancelFence.Accepted || cancelFence.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected cancel fence to reject output, got %+v", cancelFence)
	}

	lateProviderOutput, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-cf-2",
		TurnID:               "turn-cf-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-late-provider-output",
		TransportSequence:    3,
		RuntimeSequence:      3,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   102,
		WallClockTimestampMS: 102,
	})
	if err != nil {
		t.Fatalf("unexpected late-output fence error: %v", err)
	}
	if lateProviderOutput.Accepted || lateProviderOutput.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected deterministic late-output fence, got %+v", lateProviderOutput)
	}
}

func TestEvaluateOutputEmitsTelemetryEvents(t *testing.T) {
	sink := telemetry.NewMemorySink()
	pipeline := telemetry.NewPipeline(sink, telemetry.Config{QueueCapacity: 16})
	previous := telemetry.DefaultEmitter()
	telemetry.SetDefaultEmitter(pipeline)
	t.Cleanup(func() {
		telemetry.SetDefaultEmitter(previous)
		_ = pipeline.Close()
	})

	fence := NewOutputFence()
	_, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-rk22-telemetry-1",
		TurnID:               "turn-rk22-telemetry-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-rk22-telemetry-1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       2,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected evaluate output error: %v", err)
	}
	if err := pipeline.Close(); err != nil {
		t.Fatalf("unexpected pipeline close error: %v", err)
	}

	var cancelMetric bool
	var nodeSpan bool
	var decisionLog bool
	for _, event := range sink.Events() {
		if event.Correlation.SessionID != "sess-rk22-telemetry-1" {
			continue
		}
		if event.Kind == telemetry.EventKindMetric && event.Metric != nil && event.Metric.Name == telemetry.MetricCancelLatencyMS {
			cancelMetric = true
		}
		if event.Kind == telemetry.EventKindSpan && event.Span != nil && event.Span.Name == "node_span" {
			nodeSpan = true
		}
		if event.Kind == telemetry.EventKindLog && event.Log != nil && event.Log.Name == "output_fence_decision" {
			decisionLog = true
		}
	}
	if !cancelMetric || !nodeSpan || !decisionLog {
		t.Fatalf("expected output-fence telemetry events, got cancel_latency=%v node_span=%v decision_log=%v", cancelMetric, nodeSpan, decisionLog)
	}
}
