package localadmission

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

func TestEvaluatePreTurnSnapshotFailure(t *testing.T) {
	t.Parallel()

	evaluator := Evaluator{}
	result := evaluator.EvaluatePreTurn(PreTurnInput{
		SessionID:             "sess-1",
		TurnID:                "turn-1",
		EventID:               "evt-1",
		RuntimeTimestampMS:    1,
		WallClockTimestampMS:  1,
		SnapshotValid:         false,
		SnapshotFailurePolicy: controlplane.OutcomeReject,
	})

	if result.Allowed {
		t.Fatalf("expected snapshot failure to reject")
	}
	if result.Outcome == nil || result.Outcome.OutcomeKind != controlplane.OutcomeReject {
		t.Fatalf("expected reject outcome, got %+v", result.Outcome)
	}
	if err := result.Outcome.Validate(); err != nil {
		t.Fatalf("outcome should validate: %v", err)
	}
}

func TestEvaluatePreTurnCapacityDefer(t *testing.T) {
	t.Parallel()

	evaluator := Evaluator{}
	result := evaluator.EvaluatePreTurn(PreTurnInput{
		SessionID:            "sess-1",
		TurnID:               "turn-2",
		EventID:              "evt-2",
		RuntimeTimestampMS:   2,
		WallClockTimestampMS: 2,
		SnapshotValid:        true,
		CapacityDisposition:  CapacityDefer,
	})

	if result.Allowed {
		t.Fatalf("expected capacity defer to block turn open")
	}
	if result.Outcome == nil || result.Outcome.OutcomeKind != controlplane.OutcomeDefer {
		t.Fatalf("expected defer outcome, got %+v", result.Outcome)
	}
	if err := result.Outcome.Validate(); err != nil {
		t.Fatalf("outcome should validate: %v", err)
	}
}

func TestEvaluateSchedulingPointShed(t *testing.T) {
	t.Parallel()

	evaluator := Evaluator{}
	result := evaluator.EvaluateSchedulingPoint(SchedulingPointInput{
		SessionID:            "sess-1",
		TurnID:               "turn-3",
		EventID:              "evt-3",
		RuntimeTimestampMS:   3,
		WallClockTimestampMS: 3,
		Scope:                controlplane.ScopeEdgeEnqueue,
		Shed:                 true,
	})

	if result.Allowed {
		t.Fatalf("expected shed decision to block scheduling point")
	}
	if result.Outcome == nil || result.Outcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected shed outcome, got %+v", result.Outcome)
	}
	if err := result.Outcome.Validate(); err != nil {
		t.Fatalf("outcome should validate: %v", err)
	}
}
