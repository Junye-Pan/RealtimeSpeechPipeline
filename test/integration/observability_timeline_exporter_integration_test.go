package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	timelineexporter "github.com/tiger/realtime-speech-pipeline/internal/observability/timeline/exporter"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

type failingDurableSink struct{}

func (f failingDurableSink) Export(ctx context.Context, _ timelineexporter.Record) error {
	select {
	case <-time.After(20 * time.Millisecond):
	case <-ctx.Done():
	}
	return errors.New("durable timeline backend unavailable")
}

func TestTimelineDurableExporterOutageDoesNotBlockTurnProgression(t *testing.T) {
	durableExporter := timelineexporter.NewAsyncExporter(failingDurableSink{}, timelineexporter.Config{
		QueueCapacity: 8,
		ExportTimeout: 5 * time.Millisecond,
		RetryDelay:    time.Millisecond,
		MaxAttempts:   2,
	})
	t.Cleanup(func() {
		_ = durableExporter.Close()
	})

	recorder := timeline.NewRecorderWithDurableExporter(timeline.StageAConfig{
		BaselineCapacity: 8,
		DetailCapacity:   8,
	}, durableExporter)
	arbiter := turnarbiter.NewWithRecorder(&recorder)

	start := time.Now()
	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-o03-outage-1",
		TurnID:               "turn-o03-outage-1",
		EventID:              "evt-o03-outage-1",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		AuthorityEpoch:       3,
		TerminalSuccessReady: true,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/o03",
			AdmissionPolicySnapshot:   "admission-policy/o03",
			ABICompatibilitySnapshot:  "abi-compat/o03",
			VersionResolutionSnapshot: "version-resolution/o03",
			PolicyResolutionSnapshot:  "policy-resolution/o03",
			ProviderHealthSnapshot:    "provider-health/o03",
		},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("handle active with durable exporter outage: %v", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected non-blocking terminal progression, elapsed=%s", elapsed)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected closed state on terminal success, got %+v", active)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected local OR-02 baseline append to succeed during outage, got %+v", entries)
	}

	waitForExporter(t, durableExporter, time.Second, func(stats timelineexporter.Stats) bool {
		return stats.Retries >= 1 && stats.Failures >= 1
	})
}

func waitForExporter(t *testing.T, exporter *timelineexporter.AsyncExporter, timeout time.Duration, predicate func(stats timelineexporter.Stats) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stats := exporter.Stats()
		if predicate(stats) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("durable exporter condition not met within %s", timeout)
}
