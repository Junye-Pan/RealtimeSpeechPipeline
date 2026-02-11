package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
	livekittransport "github.com/tiger/realtime-speech-pipeline/transports/livekit"
)

func TestLiveKitTransportAdapterEndToEndFlow(t *testing.T) {
	t.Parallel()

	cfg := livekittransport.Config{
		SessionID:        "sess-livekit-integration",
		TurnID:           "turn-livekit-integration",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   3,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		RequestTimeout:   5 * time.Second,
	}
	adapter, err := livekittransport.NewAdapter(cfg, livekittransport.Dependencies{
		Now: fixedIntegrationNow,
	})
	if err != nil {
		t.Fatalf("new livekit adapter: %v", err)
	}

	report, err := adapter.Process(context.Background(), livekittransport.DefaultScenario(cfg))
	if err != nil {
		t.Fatalf("process livekit scenario: %v", err)
	}
	if report.ProcessedEvents != len(livekittransport.DefaultScenario(cfg)) {
		t.Fatalf("expected full event processing, got %d", report.ProcessedEvents)
	}
	if len(report.OpenResults) == 0 || report.OpenResults[0].State == "" {
		t.Fatalf("expected open-result evidence, got %+v", report.OpenResults)
	}
	if len(report.IngressRecords) == 0 || report.IngressRecords[0].PayloadClass != eventabi.PayloadAudioRaw {
		t.Fatalf("expected audio ingress classification evidence, got %+v", report.IngressRecords)
	}
	if !hasSignal(report.ControlSignals, "connected") {
		t.Fatalf("expected connected signal, got %+v", report.ControlSignals)
	}
	if !hasSignal(report.ControlSignals, "playback_cancelled") {
		t.Fatalf("expected playback_cancelled signal after cancel, got %+v", report.ControlSignals)
	}
	if !hasLifecycle(report.LifecycleEvents, "abort", "cancelled") || !hasLifecycle(report.LifecycleEvents, "close", "") {
		t.Fatalf("expected abort(cancelled)->close lifecycle, got %+v", report.LifecycleEvents)
	}
}

func TestLiveKitTransportDisconnectPathEndToEnd(t *testing.T) {
	t.Parallel()

	cfg := livekittransport.Config{
		SessionID:        "sess-livekit-disconnect",
		TurnID:           "turn-livekit-disconnect",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   6,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		RequestTimeout:   5 * time.Second,
	}
	adapter, err := livekittransport.NewAdapter(cfg, livekittransport.Dependencies{
		Now: fixedIntegrationNow,
	})
	if err != nil {
		t.Fatalf("new livekit adapter: %v", err)
	}

	report, err := adapter.Process(context.Background(), []livekittransport.Event{
		{Kind: livekittransport.EventConnected},
		{Kind: livekittransport.EventTurnOpenProposed},
		{Kind: livekittransport.EventTransportDisconnect},
	})
	if err != nil {
		t.Fatalf("process disconnect scenario: %v", err)
	}
	if !hasSignal(report.ControlSignals, "disconnected") || !hasSignal(report.ControlSignals, "stall") {
		t.Fatalf("expected disconnected/stall signals, got %+v", report.ControlSignals)
	}
	if !hasLifecycle(report.LifecycleEvents, "abort", "transport_disconnect_or_stall") {
		t.Fatalf("expected transport disconnect abort reason, got %+v", report.LifecycleEvents)
	}
}

func hasSignal(signals []eventabi.ControlSignal, signal string) bool {
	for _, candidate := range signals {
		if candidate.Signal == signal {
			return true
		}
	}
	return false
}

func hasLifecycle(events []turnarbiter.LifecycleEvent, name string, reason string) bool {
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

func fixedIntegrationNow() time.Time {
	return time.Date(2026, time.February, 11, 13, 0, 0, 0, time.UTC)
}
