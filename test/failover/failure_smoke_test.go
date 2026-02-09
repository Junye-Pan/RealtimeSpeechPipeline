package failover_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/executor"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestF1AdmissionOverloadSmoke(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	scheduler := executor.NewScheduler(localadmission.Evaluator{})

	reject, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-f1",
		TurnID:               "turn-f1-reject",
		EventID:              "evt-f1-reject",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		CapacityDisposition:  localadmission.CapacityReject,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected reject-path error: %v", err)
	}
	if reject.Decision == nil || reject.Decision.OutcomeKind != controlplane.OutcomeReject {
		t.Fatalf("expected reject outcome, got %+v", reject.Decision)
	}
	if hasLifecycle(reject.Events, "turn_open") || hasLifecycle(reject.Events, "abort") || hasLifecycle(reject.Events, "close") {
		t.Fatalf("F1 pre-turn reject must not emit turn_open/abort/close")
	}

	deferResult, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-f1",
		TurnID:               "turn-f1-defer",
		EventID:              "evt-f1-defer",
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       5,
		SnapshotValid:        true,
		CapacityDisposition:  localadmission.CapacityDefer,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected defer-path error: %v", err)
	}
	if deferResult.Decision == nil || deferResult.Decision.OutcomeKind != controlplane.OutcomeDefer {
		t.Fatalf("expected defer outcome, got %+v", deferResult.Decision)
	}
	if hasLifecycle(deferResult.Events, "turn_open") || hasLifecycle(deferResult.Events, "abort") || hasLifecycle(deferResult.Events, "close") {
		t.Fatalf("F1 pre-turn defer must not emit turn_open/abort/close")
	}

	shed, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-f1",
		TurnID:               "turn-f1-shed",
		EventID:              "evt-f1-shed",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    12,
		RuntimeSequence:      13,
		AuthorityEpoch:       5,
		RuntimeTimestampMS:   102,
		WallClockTimestampMS: 102,
		Shed:                 true,
	})
	if err != nil {
		t.Fatalf("unexpected shed-path error: %v", err)
	}
	if shed.Outcome == nil || shed.Outcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected scheduling-point shed, got %+v", shed.Outcome)
	}
	if shed.ControlSignal == nil || shed.ControlSignal.Signal != "shed" {
		t.Fatalf("expected shed control signal, got %+v", shed.ControlSignal)
	}
}

func TestF3ProviderFailureSmoke(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-f3",
		TurnID:               "turn-f3",
		EventID:              "evt-f3",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		AuthorityEpoch:       7,
		ProviderFailure:      true,
	})
	if err != nil {
		t.Fatalf("unexpected provider failure handling error: %v", err)
	}
	if len(active.Events) != 2 || active.Events[0].Name != "abort" || active.Events[0].Reason != "provider_failure" || active.Events[1].Name != "close" {
		t.Fatalf("expected abort(provider_failure)->close, got %+v", active.Events)
	}
}

func TestF7AuthorityConflictSmoke(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()

	preTurn, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-f7",
		TurnID:               "turn-f7-open",
		EventID:              "evt-f7-open",
		RuntimeTimestampMS:   300,
		WallClockTimestampMS: 300,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       9,
		SnapshotValid:        true,
		AuthorityEpochValid:  false,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected pre-turn stale-epoch error: %v", err)
	}
	if preTurn.Decision == nil || preTurn.Decision.OutcomeKind != controlplane.OutcomeStaleEpochReject {
		t.Fatalf("expected stale_epoch_reject outcome, got %+v", preTurn.Decision)
	}
	if hasLifecycle(preTurn.Events, "abort") || hasLifecycle(preTurn.Events, "close") {
		t.Fatalf("F7 pre-turn stale epoch must not emit abort/close")
	}

	inTurn, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-f7",
		TurnID:               "turn-f7-active",
		EventID:              "evt-f7-active",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   310,
		WallClockTimestampMS: 310,
		AuthorityEpoch:       9,
		AuthorityRevoked:     true,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected in-turn authority conflict error: %v", err)
	}
	if len(inTurn.Events) != 3 ||
		inTurn.Events[0].Name != "deauthorized_drain" ||
		inTurn.Events[1].Name != "abort" ||
		inTurn.Events[1].Reason != "authority_loss" ||
		inTurn.Events[2].Name != "close" {
		t.Fatalf("expected deauthorized_drain->abort(authority_loss)->close, got %+v", inTurn.Events)
	}
}

func hasLifecycle(events []turnarbiter.LifecycleEvent, name string) bool {
	for _, evt := range events {
		if evt.Name == name {
			return true
		}
	}
	return false
}
