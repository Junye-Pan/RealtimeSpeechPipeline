package integration_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/session"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestSessionLifecycleRejectAndDeferDoNotEmitAcceptedTurnTerminalSignals(t *testing.T) {
	t.Parallel()

	orch, err := session.NewOrchestrator("sess-int-r01-preturn", turnarbiter.New())
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
			turnID:                "turn-int-r01-reject",
			eventID:               "evt-int-r01-reject",
			snapshotFailurePolicy: controlplane.OutcomeReject,
			wantOutcome:           controlplane.OutcomeReject,
		},
		{
			name:                  "defer",
			turnID:                "turn-int-r01-defer",
			eventID:               "evt-int-r01-defer",
			snapshotFailurePolicy: controlplane.OutcomeDefer,
			wantOutcome:           controlplane.OutcomeDefer,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			open, err := orch.HandleTurnOpen(turnarbiter.OpenRequest{
				SessionID:             "sess-int-r01-preturn",
				TurnID:                tt.turnID,
				EventID:               tt.eventID,
				RuntimeTimestampMS:    100,
				WallClockTimestampMS:  100,
				PipelineVersion:       "pipeline-v1",
				AuthorityEpoch:        5,
				SnapshotValid:         false,
				SnapshotFailurePolicy: tt.snapshotFailurePolicy,
				AuthorityEpochValid:   true,
				AuthorityAuthorized:   true,
			})
			if err != nil {
				t.Fatalf("open path failed: %v", err)
			}
			if open.State != controlplane.TurnIdle {
				t.Fatalf("expected idle state, got %s", open.State)
			}
			if open.Decision == nil || open.Decision.OutcomeKind != tt.wantOutcome {
				t.Fatalf("expected %s decision outcome, got %+v", tt.wantOutcome, open.Decision)
			}
			if lifecycleContains(open.Events, "turn_open") || lifecycleContains(open.Events, "abort") || lifecycleContains(open.Events, "close") {
				t.Fatalf("pre-turn %s must not emit turn_open/abort/close lifecycle events: %+v", tt.wantOutcome, open.Events)
			}

			state, ok := orch.State(tt.turnID)
			if !ok || state != controlplane.TurnIdle {
				t.Fatalf("expected orchestrator state Idle, got state=%s ok=%t", state, ok)
			}
		})
	}
}

func TestSessionLifecycleAcceptedTurnAbortCloseSequence(t *testing.T) {
	t.Parallel()

	orch, err := session.NewOrchestrator("sess-int-r01-accepted", turnarbiter.New())
	if err != nil {
		t.Fatalf("new orchestrator: %v", err)
	}

	open, err := orch.HandleTurnOpen(turnarbiter.OpenRequest{
		SessionID:            "sess-int-r01-accepted",
		TurnID:               "turn-int-r01-accepted",
		EventID:              "evt-int-r01-open",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       7,
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

	active, err := orch.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-int-r01-accepted",
		TurnID:               "turn-int-r01-accepted",
		EventID:              "evt-int-r01-active",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   130,
		WallClockTimestampMS: 130,
		AuthorityEpoch:       7,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("active path failed: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected closed state, got %s", active.State)
	}
	if !lifecycleContains(active.Events, "abort") || !lifecycleContains(active.Events, "close") {
		t.Fatalf("expected abort->close lifecycle, got %+v", active.Events)
	}

	state, ok := orch.State("turn-int-r01-accepted")
	if !ok || state != controlplane.TurnClosed {
		t.Fatalf("expected orchestrator state Closed, got state=%s ok=%t", state, ok)
	}
}

func lifecycleContains(events []turnarbiter.LifecycleEvent, name string) bool {
	for _, ev := range events {
		if ev.Name == name {
			return true
		}
	}
	return false
}
