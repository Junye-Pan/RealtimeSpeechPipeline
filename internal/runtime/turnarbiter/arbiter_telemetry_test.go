package turnarbiter

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
)

func TestApplyOpenEmitsTelemetryEvents(t *testing.T) {
	sink := telemetry.NewMemorySink()
	pipeline := telemetry.NewPipeline(sink, telemetry.Config{QueueCapacity: 32})
	previous := telemetry.DefaultEmitter()
	telemetry.SetDefaultEmitter(pipeline)
	t.Cleanup(func() {
		telemetry.SetDefaultEmitter(previous)
		_ = pipeline.Close()
	})

	arbiter := New()
	_, err := arbiter.Apply(ApplyInput{Open: &OpenRequest{
		SessionID:             "sess-rk03-telemetry-1",
		TurnID:                "turn-rk03-telemetry-1",
		EventID:               "evt-rk03-telemetry-1",
		RuntimeTimestampMS:    10,
		WallClockTimestampMS:  10,
		PipelineVersion:       "pipeline-v1",
		AuthorityEpoch:        3,
		SnapshotValid:         true,
		AuthorityEpochValid:   true,
		AuthorityAuthorized:   true,
		SnapshotFailurePolicy: controlplane.OutcomeDefer,
		PlanFailurePolicy:     controlplane.OutcomeReject,
	}})
	if err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	if err := pipeline.Close(); err != nil {
		t.Fatalf("unexpected pipeline close error: %v", err)
	}

	var turnSpan bool
	var turnLog bool
	for _, event := range sink.Events() {
		if event.Correlation.SessionID != "sess-rk03-telemetry-1" {
			continue
		}
		if event.Kind == telemetry.EventKindSpan && event.Span != nil && event.Span.Name == "turn_span" {
			turnSpan = true
		}
		if event.Kind == telemetry.EventKindLog && event.Log != nil && event.Log.Name == "turn_open_result" {
			turnLog = true
		}
	}
	if !turnSpan || !turnLog {
		t.Fatalf("expected turn telemetry events, got turn_span=%v turn_log=%v", turnSpan, turnLog)
	}
}
