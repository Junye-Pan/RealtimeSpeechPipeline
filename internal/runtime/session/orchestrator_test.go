package session

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestOrchestratorPreTurnIdleOutcomesTrackSessionState(t *testing.T) {
	t.Parallel()

	o, err := NewOrchestrator("sess-orch-idle", turnarbiter.New())
	if err != nil {
		t.Fatalf("new orchestrator: %v", err)
	}

	tests := []struct {
		name                  string
		turnID                string
		eventID               string
		snapshotFailurePolicy controlplane.OutcomeKind
		wantOutcome           controlplane.OutcomeKind
	}{
		{
			name:                  "reject",
			turnID:                "turn-reject",
			eventID:               "evt-reject",
			snapshotFailurePolicy: controlplane.OutcomeReject,
			wantOutcome:           controlplane.OutcomeReject,
		},
		{
			name:                  "defer",
			turnID:                "turn-defer",
			eventID:               "evt-defer",
			snapshotFailurePolicy: controlplane.OutcomeDefer,
			wantOutcome:           controlplane.OutcomeDefer,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := o.HandleTurnOpen(turnarbiter.OpenRequest{
				SessionID:             "sess-orch-idle",
				TurnID:                tt.turnID,
				EventID:               tt.eventID,
				RuntimeTimestampMS:    100,
				WallClockTimestampMS:  100,
				PipelineVersion:       "pipeline-v1",
				AuthorityEpoch:        7,
				SnapshotValid:         false,
				SnapshotFailurePolicy: tt.snapshotFailurePolicy,
				AuthorityEpochValid:   true,
				AuthorityAuthorized:   true,
			})
			if err != nil {
				t.Fatalf("open path failed: %v", err)
			}
			if out.State != controlplane.TurnIdle {
				t.Fatalf("expected idle state, got %s", out.State)
			}
			if out.Decision == nil || out.Decision.OutcomeKind != tt.wantOutcome {
				t.Fatalf("expected %s decision, got %+v", tt.wantOutcome, out.Decision)
			}
			state, ok := o.State(tt.turnID)
			if !ok || state != controlplane.TurnIdle {
				t.Fatalf("expected tracked Idle state, got state=%s ok=%t", state, ok)
			}
		})
	}
}

func TestOrchestratorAcceptedTurnTerminalSequenceClosesLifecycle(t *testing.T) {
	t.Parallel()

	o, err := NewOrchestrator("sess-orch-terminal", turnarbiter.New())
	if err != nil {
		t.Fatalf("new orchestrator: %v", err)
	}

	open, err := o.HandleTurnOpen(turnarbiter.OpenRequest{
		SessionID:            "sess-orch-terminal",
		TurnID:               "turn-terminal",
		EventID:              "evt-open-terminal",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       11,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnActive {
		t.Fatalf("expected active state after open, got %s", open.State)
	}

	active, err := o.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-orch-terminal",
		TurnID:               "turn-terminal",
		EventID:              "evt-active-terminal",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   130,
		WallClockTimestampMS: 130,
		AuthorityEpoch:       11,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("active path failed: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected closed state, got %s", active.State)
	}

	state, ok := o.State("turn-terminal")
	if !ok || state != controlplane.TurnClosed {
		t.Fatalf("expected tracked Closed state, got state=%s ok=%t", state, ok)
	}
}

func TestOrchestratorRejectsSessionMismatch(t *testing.T) {
	t.Parallel()

	o, err := NewOrchestrator("sess-owned", turnarbiter.New())
	if err != nil {
		t.Fatalf("new orchestrator: %v", err)
	}

	if _, err := o.HandleTurnOpen(turnarbiter.OpenRequest{
		SessionID:            "sess-other",
		TurnID:               "turn-1",
		EventID:              "evt-1",
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       1,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	}); err == nil {
		t.Fatalf("expected session mismatch to fail")
	}
}
