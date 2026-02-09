package integration_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/executor"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestGuardPlanResolverTurnArbiterChain(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-1",
		TurnID:               "turn-integration-1",
		EventID:              "evt-open-1",
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
		t.Fatalf("expected turn to become Active, got %s", open.State)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-1",
		TurnID:               "turn-integration-1",
		EventID:              "evt-active-1",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   150,
		WallClockTimestampMS: 150,
		AuthorityEpoch:       11,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("active path failed: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected turn to close, got %s", active.State)
	}
	if len(active.Transitions) != 2 {
		t.Fatalf("expected terminal transitions, got %d", len(active.Transitions))
	}
}

func TestGuardRejectsStaleAuthorityBeforeOpen(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-2",
		TurnID:               "turn-integration-2",
		EventID:              "evt-open-2",
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       22,
		SnapshotValid:        true,
		AuthorityEpochValid:  false,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if open.State != controlplane.TurnIdle {
		t.Fatalf("expected pre-turn rejection to stay Idle, got %s", open.State)
	}
	if open.Decision == nil || open.Decision.OutcomeKind != controlplane.OutcomeStaleEpochReject {
		t.Fatalf("expected stale_epoch_reject outcome, got %+v", open.Decision)
	}
}

func TestSchedulingPointShedDoesNotForceTerminalLifecycle(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	scheduler := executor.NewScheduler(localadmission.Evaluator{})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-integration-3",
		TurnID:               "turn-integration-3",
		EventID:              "evt-open-3",
		RuntimeTimestampMS:   300,
		WallClockTimestampMS: 300,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       33,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("open path failed: %v", err)
	}
	if open.State != controlplane.TurnActive {
		t.Fatalf("expected Active after open, got %s", open.State)
	}

	decision, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-integration-3",
		TurnID:               "turn-integration-3",
		EventID:              "evt-dispatch-3",
		RuntimeTimestampMS:   320,
		WallClockTimestampMS: 320,
		Shed:                 true,
	})
	if err != nil {
		t.Fatalf("scheduling decision failed: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected shed outcome at scheduling point")
	}
	if decision.Outcome == nil || decision.Outcome.OutcomeKind != controlplane.OutcomeShed {
		t.Fatalf("expected shed decision outcome, got %+v", decision.Outcome)
	}
	if decision.ControlSignal == nil {
		t.Fatalf("expected shed control signal")
	}
	if decision.ControlSignal.Signal != "shed" || decision.ControlSignal.EmittedBy != "RK-25" {
		t.Fatalf("unexpected shed control signal: %+v", decision.ControlSignal)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-integration-3",
		TurnID:               "turn-integration-3",
		EventID:              "evt-active-3",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   330,
		WallClockTimestampMS: 330,
		AuthorityEpoch:       33,
	})
	if err != nil {
		t.Fatalf("active lifecycle handling failed: %v", err)
	}
	if active.State != controlplane.TurnActive {
		t.Fatalf("scheduling shed alone must not terminalize turn, got %s", active.State)
	}
	if len(active.Transitions) != 0 {
		t.Fatalf("expected no lifecycle transitions from scheduling shed alone")
	}
}

func TestProviderInvocationEvidenceThreadedIntoTerminalBaseline(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 16})
	arbiter := turnarbiter.NewWithRecorder(&recorder)
	scheduler := executor.NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invocation.NewController(catalog))

	decision, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-integration-provider-1",
		TurnID:               "turn-integration-provider-1",
		EventID:              "evt-dispatch-provider-1",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		ProviderInvocation: &executor.ProviderInvocationInput{
			Modality:          contracts.ModalitySTT,
			PreferredProvider: "stt-a",
		},
	})
	if err != nil {
		t.Fatalf("unexpected scheduling/provider error: %v", err)
	}
	if !decision.Allowed || decision.Provider == nil {
		t.Fatalf("expected allowed provider decision, got %+v", decision)
	}

	active, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:                  "sess-integration-provider-1",
		TurnID:                     "turn-integration-provider-1",
		EventID:                    "evt-active-provider-1",
		PipelineVersion:            "pipeline-v1",
		RuntimeTimestampMS:         120,
		WallClockTimestampMS:       120,
		AuthorityEpoch:             3,
		RuntimeSequence:            2,
		TerminalSuccessReady:       true,
		ProviderInvocationOutcomes: []timeline.InvocationOutcomeEvidence{decision.Provider.ToInvocationOutcomeEvidence()},
	})
	if err != nil {
		t.Fatalf("unexpected active handling error: %v", err)
	}
	if active.State != controlplane.TurnClosed {
		t.Fatalf("expected terminal close state, got %s", active.State)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 baseline entry, got %d", len(entries))
	}
	if len(entries[0].InvocationOutcomes) != 1 {
		t.Fatalf("expected 1 invocation outcome evidence, got %d", len(entries[0].InvocationOutcomes))
	}
	if entries[0].InvocationOutcomes[0].OutcomeClass != "success" {
		t.Fatalf("expected success invocation outcome class, got %+v", entries[0].InvocationOutcomes[0])
	}
}
