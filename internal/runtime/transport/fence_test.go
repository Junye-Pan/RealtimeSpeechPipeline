package transport

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/state"
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
		if event.Correlation.NodeID != "transport_fence" || event.Correlation.EdgeID != "transport/egress" {
			t.Fatalf("expected node/edge correlation IDs, got %+v", event.Correlation)
		}
		if event.Kind == telemetry.EventKindMetric && event.Metric != nil && event.Metric.Name == telemetry.MetricCancelLatencyMS {
			if event.Metric.Attributes["node_id"] != "transport_fence" || event.Metric.Attributes["edge_id"] != "transport/egress" {
				t.Fatalf("expected node/edge metric linkage labels, got %+v", event.Metric.Attributes)
			}
			cancelMetric = true
		}
		if event.Kind == telemetry.EventKindSpan && event.Span != nil && event.Span.Name == "node_span" {
			if event.Span.Attributes["node_type"] != "transport_fence" {
				t.Fatalf("expected node_type span linkage label, got %+v", event.Span.Attributes)
			}
			nodeSpan = true
		}
		if event.Kind == telemetry.EventKindLog && event.Log != nil && event.Log.Name == "output_fence_decision" {
			if event.Log.Attributes["node_id"] != "transport_fence" || event.Log.Attributes["edge_id"] != "transport/egress" {
				t.Fatalf("expected node/edge log linkage labels, got %+v", event.Log.Attributes)
			}
			decisionLog = true
		}
	}
	if !cancelMetric || !nodeSpan || !decisionLog {
		t.Fatalf("expected output-fence telemetry events, got cancel_latency=%v node_span=%v decision_log=%v", cancelMetric, nodeSpan, decisionLog)
	}
}

func TestEvaluateOutputRejectsStaleAuthorityEpoch(t *testing.T) {
	t.Parallel()

	stateSvc := state.NewService()
	if err := stateSvc.ValidateAuthority("sess-rk22-authority-1", 9); err != nil {
		t.Fatalf("seed authority epoch: %v", err)
	}
	fence := NewOutputFenceWithDependencies(nil, stateSvc)

	stale, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-rk22-authority-1",
		TurnID:               "turn-rk22-authority-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-rk22-stale-1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       8,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("evaluate stale authority output: %v", err)
	}
	if stale.Accepted {
		t.Fatalf("expected stale authority output to be rejected")
	}
	if stale.Signal.Signal != "stale_epoch_reject" || stale.Signal.EmittedBy != "RK-24" {
		t.Fatalf("expected stale_epoch_reject emitted by RK-24, got %+v", stale.Signal)
	}

	fresh, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-rk22-authority-1",
		TurnID:               "turn-rk22-authority-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-rk22-fresh-1",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       10,
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
	})
	if err != nil {
		t.Fatalf("evaluate fresh authority output: %v", err)
	}
	if !fresh.Accepted || fresh.Signal.Signal != "output_accepted" {
		t.Fatalf("expected fresh authority output to be accepted, got %+v", fresh)
	}
}
