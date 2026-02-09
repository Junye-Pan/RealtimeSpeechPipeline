package integration_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestAE001PreTurnStaleEpochReject(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-ae-001",
		TurnID:               "turn-ae-001",
		EventID:              "evt-ae-001",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       9,
		SnapshotValid:        true,
		AuthorityEpochValid:  false,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if open.Decision == nil || open.Decision.OutcomeKind != controlplane.OutcomeStaleEpochReject {
		t.Fatalf("expected stale_epoch_reject, got %+v", open.Decision)
	}
	if containsLifecycle(open.Events, "turn_open") || containsLifecycle(open.Events, "abort") || containsLifecycle(open.Events, "close") {
		t.Fatalf("AE-001 requires no turn_open/abort/close on pre-turn stale epoch path")
	}
}

func TestAE005PreTurnAuthorityDeauthorization(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-ae-005",
		TurnID:               "turn-ae-005",
		EventID:              "evt-ae-005",
		RuntimeTimestampMS:   120,
		WallClockTimestampMS: 120,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       10,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if open.Decision == nil || open.Decision.OutcomeKind != controlplane.OutcomeDeauthorized {
		t.Fatalf("expected deauthorized_drain, got %+v", open.Decision)
	}
	if containsLifecycle(open.Events, "turn_open") || containsLifecycle(open.Events, "abort") || containsLifecycle(open.Events, "close") {
		t.Fatalf("AE-005 requires no turn_open/abort/close on pre-turn deauthorization path")
	}
}

func TestCF001CancelDuringActiveGenerationFencesOutput(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-cf-001",
		TurnID:               "turn-cf-001",
		EventID:              "evt-cf-001",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		AuthorityEpoch:       5,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(active.Events) != 2 || active.Events[0].Name != "abort" || active.Events[0].Reason != "cancelled" || active.Events[1].Name != "close" {
		t.Fatalf("expected abort(cancelled)->close terminalization, got %+v", active.Events)
	}
	if containsLifecycle(active.Events, "output_accepted") {
		t.Fatalf("CF-001 requires no accepted output after cancel fence")
	}
}

func containsLifecycle(events []turnarbiter.LifecycleEvent, name string) bool {
	for _, e := range events {
		if e.Name == name {
			return true
		}
	}
	return false
}
