package replay_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	replaycmp "github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
)

func TestRD001ReplayDecisionsSameTraceNoDivergence(t *testing.T) {
	t.Parallel()

	epoch := int64(7)
	baseline := []controlplane.DecisionOutcome{
		{
			OutcomeKind:        controlplane.OutcomeAdmit,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeSession,
			SessionID:          "sess-rd-1",
			EventID:            "evt-rd-1",
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_allow",
		},
		{
			OutcomeKind:        controlplane.OutcomeStaleEpochReject,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeTurn,
			SessionID:          "sess-rd-1",
			TurnID:             "turn-rd-1",
			EventID:            "evt-rd-2",
			RuntimeTimestampMS: 110,
			WallClockMS:        110,
			EmittedBy:          controlplane.EmitterRK24,
			AuthorityEpoch:     &epoch,
			Reason:             "authority_epoch_mismatch",
		},
	}
	replayed := append([]controlplane.DecisionOutcome(nil), baseline...)

	divergences := replaycmp.CompareDecisionOutcomes(baseline, replayed)
	if len(divergences) != 0 {
		t.Fatalf("expected zero divergences on replay-decisions mode, got %+v", divergences)
	}
}

func TestReplayComparatorReportsOutcomeDivergence(t *testing.T) {
	t.Parallel()

	baseline := []controlplane.DecisionOutcome{
		{
			OutcomeKind:        controlplane.OutcomeDefer,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeSession,
			SessionID:          "sess-rd-2",
			EventID:            "evt-rd-3",
			RuntimeTimestampMS: 200,
			WallClockMS:        200,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_defer",
		},
	}
	replayed := []controlplane.DecisionOutcome{
		{
			OutcomeKind:        controlplane.OutcomeReject,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeSession,
			SessionID:          "sess-rd-2",
			EventID:            "evt-rd-3",
			RuntimeTimestampMS: 200,
			WallClockMS:        200,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_reject",
		},
	}

	divergences := replaycmp.CompareDecisionOutcomes(baseline, replayed)
	if len(divergences) != 1 {
		t.Fatalf("expected one divergence, got %+v", divergences)
	}
	if divergences[0].Class != obs.OutcomeDivergence {
		t.Fatalf("expected OUTCOME_DIVERGENCE, got %s", divergences[0].Class)
	}
}

func TestReplayComparatorReportsAllDivergenceClasses(t *testing.T) {
	t.Parallel()

	epoch := int64(9)
	baselineDecision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeSession,
		SessionID:          "sess-rd-3",
		EventID:            "evt-rd-4",
		RuntimeTimestampMS: 100,
		WallClockMS:        100,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
		AuthorityEpoch:     &epoch,
	}
	replayDecision := baselineDecision
	replayDecision.Reason = "admission_capacity_reject"

	baseline := []replaycmp.TraceArtifact{
		{
			PlanHash:           "plan-a",
			Decision:           baselineDecision,
			OrderingMarker:     "runtime_sequence:10",
			AuthorityEpoch:     9,
			RuntimeTimestampMS: 100,
		},
	}
	replayed := []replaycmp.TraceArtifact{
		{
			PlanHash:           "plan-b",
			Decision:           replayDecision,
			OrderingMarker:     "runtime_sequence:11",
			AuthorityEpoch:     10,
			RuntimeTimestampMS: 130,
		},
	}

	divergences := replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: 5})
	classes := map[obs.DivergenceClass]bool{}
	for _, d := range divergences {
		classes[d.Class] = true
	}
	for _, required := range []obs.DivergenceClass{
		obs.PlanDivergence,
		obs.OutcomeDivergence,
		obs.OrderingDivergence,
		obs.AuthorityDivergence,
		obs.TimingDivergence,
	} {
		if !classes[required] {
			t.Fatalf("expected divergence class %s in %+v", required, divergences)
		}
	}
}
