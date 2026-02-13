package livekit

import (
	"context"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestAdapterProcessDefaultScenarioProducesCancelFenceEvidence(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SessionID:        "sess-livekit-1",
		TurnID:           "turn-livekit-1",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   7,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		RequestTimeout:   5 * time.Second,
	}
	adapter, err := NewAdapter(cfg, Dependencies{
		Now: fixedNow,
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	report, err := adapter.Process(context.Background(), DefaultScenario(cfg))
	if err != nil {
		t.Fatalf("process default scenario: %v", err)
	}
	if report.ProcessedEvents != len(DefaultScenario(cfg)) {
		t.Fatalf("unexpected processed event count: %d", report.ProcessedEvents)
	}
	if len(report.IngressRecords) == 0 {
		t.Fatalf("expected ingress evidence")
	}
	if report.IngressRecords[0].PayloadClass != eventabi.PayloadAudioRaw {
		t.Fatalf("expected audio ingress class, got %s", report.IngressRecords[0].PayloadClass)
	}
	if len(report.OutputDecisions) != 2 {
		t.Fatalf("expected two output decisions, got %+v", report.OutputDecisions)
	}
	if !report.OutputDecisions[0].Accepted || report.OutputDecisions[0].Signal != "output_accepted" {
		t.Fatalf("expected first output accepted, got %+v", report.OutputDecisions[0])
	}
	if report.OutputDecisions[1].Accepted || report.OutputDecisions[1].Signal != "playback_cancelled" {
		t.Fatalf("expected late output fenced, got %+v", report.OutputDecisions[1])
	}
	if !containsLifecycle(report.LifecycleEvents, "abort", "cancelled") || !containsLifecycle(report.LifecycleEvents, "close", "") {
		t.Fatalf("expected abort(cancelled)->close lifecycle, got %+v", report.LifecycleEvents)
	}
}

func TestAdapterProcessAuthorityRevokedPath(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SessionID:        "sess-livekit-authority",
		TurnID:           "turn-livekit-authority",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   9,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		RequestTimeout:   5 * time.Second,
	}
	adapter, err := NewAdapter(cfg, Dependencies{Now: fixedNow})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	events := []Event{
		{Kind: EventTurnOpenProposed},
		{Kind: EventAuthorityRevoked},
	}
	report, err := adapter.Process(context.Background(), events)
	if err != nil {
		t.Fatalf("process authority-revoked scenario: %v", err)
	}
	if !containsLifecycle(report.LifecycleEvents, "deauthorized_drain", "authority_revoked_in_turn") {
		t.Fatalf("expected deauthorized_drain lifecycle event, got %+v", report.LifecycleEvents)
	}
	if !hasSignal(report.ControlSignals, "deauthorized_drain") {
		t.Fatalf("expected deauthorized_drain control signal, got %+v", report.ControlSignals)
	}
}

func TestAdapterProcessTransportDisconnectPath(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SessionID:        "sess-livekit-disconnect",
		TurnID:           "turn-livekit-disconnect",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   5,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		RequestTimeout:   5 * time.Second,
	}
	adapter, err := NewAdapter(cfg, Dependencies{Now: fixedNow})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	report, err := adapter.Process(context.Background(), []Event{
		{Kind: EventTurnOpenProposed},
		{Kind: EventTransportDisconnect},
	})
	if err != nil {
		t.Fatalf("process transport disconnect scenario: %v", err)
	}
	if !hasSignal(report.ControlSignals, "disconnected") || !hasSignal(report.ControlSignals, "stall") {
		t.Fatalf("expected disconnected/stall control signals, got %+v", report.ControlSignals)
	}
	if !containsLifecycle(report.LifecycleEvents, "abort", "transport_disconnect_or_stall") {
		t.Fatalf("expected abort(transport_disconnect_or_stall), got %+v", report.LifecycleEvents)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, time.February, 11, 9, 0, 0, 0, time.UTC)
}

func hasSignal(signals []eventabi.ControlSignal, signal string) bool {
	for _, candidate := range signals {
		if candidate.Signal == signal {
			return true
		}
	}
	return false
}

func containsLifecycle(events []turnarbiter.LifecycleEvent, name string, reason string) bool {
	for _, event := range events {
		if event.Name != name {
			continue
		}
		if reason == "" || event.Reason == reason {
			return true
		}
	}
	return false
}
