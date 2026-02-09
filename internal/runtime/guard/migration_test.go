package guard

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

func TestBuildMigrationSignalsF8(t *testing.T) {
	t.Parallel()

	signals, err := BuildMigrationSignals(MigrationInput{
		SessionID:             "sess-f8-1",
		TurnID:                "turn-f8-1",
		PipelineVersion:       "pipeline-v1",
		EventIDBase:           "evt-f8",
		TransportSequence:     10,
		RuntimeSequence:       11,
		AuthorityEpoch:        5,
		RuntimeTimestampMS:    1000,
		WallClockTimestampMS:  1000,
		IncludeSessionHandoff: true,
	})
	if err != nil {
		t.Fatalf("unexpected migration signal error: %v", err)
	}
	if len(signals) != 4 {
		t.Fatalf("expected 4 migration signals, got %d", len(signals))
	}
	if signals[0].Signal != "lease_rotated" || signals[1].Signal != "migration_start" || signals[2].Signal != "migration_finish" {
		t.Fatalf("unexpected migration ordering: %+v", signals)
	}
}

func TestRejectOldPlacementOutputF8(t *testing.T) {
	t.Parallel()

	outcome, signal, err := RejectOldPlacementOutput("sess-f8-2", "turn-f8-2", "evt-f8-old", 2000, 2000, 7)
	if err != nil {
		t.Fatalf("unexpected stale-output rejection error: %v", err)
	}
	if outcome.OutcomeKind != controlplane.OutcomeStaleEpochReject || outcome.Phase != controlplane.PhaseScheduling {
		t.Fatalf("unexpected stale-epoch outcome: %+v", outcome)
	}
	if signal.Signal != "stale_epoch_reject" || signal.EmittedBy != "RK-24" {
		t.Fatalf("unexpected stale-epoch control signal: %+v", signal)
	}
}
