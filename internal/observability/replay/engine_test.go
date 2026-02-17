package replay

import (
	"errors"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

func TestEngineRunPlaybackModeReportsProviderChoiceDivergence(t *testing.T) {
	t.Parallel()

	baseline := ReplayArtifact{
		TraceArtifacts: []TraceArtifact{
			testTraceArtifact("sess-1", "turn-1", "evt-1", 10),
		},
	}
	candidate := baseline.Clone()
	candidate.TraceArtifacts[0].ProviderID = "provider-b"
	candidate.TraceArtifacts[0].ProviderModel = "model-b"

	engine := Engine{
		ArtifactResolver: StaticReplayArtifactResolver{
			Artifacts: map[string]ReplayArtifact{
				"baseline":  baseline,
				"candidate": candidate,
			},
		},
		CompareConfig: CompareConfig{TimingToleranceMS: 0},
		NewRunID:      func() string { return "run-playback" },
	}
	result, err := engine.Run(obs.ReplayRunRequest{
		BaselineRef:      "baseline",
		CandidatePlanRef: "candidate",
		Mode:             obs.ReplayModePlaybackRecordedProvider,
	})
	if err != nil {
		t.Fatalf("unexpected replay run error: %v", err)
	}
	if result.Mode != obs.ReplayModePlaybackRecordedProvider {
		t.Fatalf("expected mode %s, got %s", obs.ReplayModePlaybackRecordedProvider, result.Mode)
	}
	if result.RunID != "run-playback" {
		t.Fatalf("unexpected run id: %s", result.RunID)
	}
	if result.Cursor == nil || result.Cursor.EventID != "evt-1" {
		t.Fatalf("expected replay cursor anchored at evt-1, got %+v", result.Cursor)
	}

	classes := map[obs.DivergenceClass]bool{}
	for _, divergence := range result.Divergences {
		classes[divergence.Class] = true
	}
	if !classes[obs.ProviderChoiceDivergence] {
		t.Fatalf("expected provider choice divergence, got %+v", result.Divergences)
	}
}

func TestEngineRunCursorResumeUsesBoundaryDeterministically(t *testing.T) {
	t.Parallel()

	baseline := ReplayArtifact{
		TraceArtifacts: []TraceArtifact{
			testTraceArtifact("sess-r", "turn-r", "evt-1", 10),
			testTraceArtifact("sess-r", "turn-r", "evt-2", 20),
			testTraceArtifact("sess-r", "turn-r", "evt-3", 30),
		},
	}
	candidate := baseline.Clone()
	candidate.TraceArtifacts[1].PlanHash = "plan-mismatch"

	engine := Engine{
		ArtifactResolver: StaticReplayArtifactResolver{
			Artifacts: map[string]ReplayArtifact{
				"baseline":  baseline,
				"candidate": candidate,
			},
		},
		CompareConfig: CompareConfig{TimingToleranceMS: 0},
		NewRunID:      func() string { return "run-cursor" },
	}

	result, err := engine.Run(obs.ReplayRunRequest{
		BaselineRef:      "baseline",
		CandidatePlanRef: "candidate",
		Mode:             obs.ReplayModeReSimulateNodes,
		Cursor: &obs.ReplayCursor{
			SessionID:       "sess-r",
			TurnID:          "turn-r",
			Lane:            eventabi.LaneData,
			RuntimeSequence: 20,
			EventID:         "evt-2",
		},
	})
	if err != nil {
		t.Fatalf("unexpected replay run error: %v", err)
	}

	if result.Cursor == nil || result.Cursor.EventID != "evt-3" || result.Cursor.RuntimeSequence != 30 {
		t.Fatalf("expected replay cursor to end on evt-3 sequence=30, got %+v", result.Cursor)
	}

	foundPlanDivergence := false
	for _, divergence := range result.Divergences {
		if divergence.Class == obs.PlanDivergence {
			foundPlanDivergence = true
			break
		}
	}
	if !foundPlanDivergence {
		t.Fatalf("expected plan divergence at resumed boundary, got %+v", result.Divergences)
	}
}

func TestEngineRunReturnsErrorWhenCursorBoundaryMissingFromBaseline(t *testing.T) {
	t.Parallel()

	baseline := ReplayArtifact{
		TraceArtifacts: []TraceArtifact{
			testTraceArtifact("sess-e", "turn-e", "evt-1", 10),
		},
	}
	engine := Engine{
		ArtifactResolver: StaticReplayArtifactResolver{
			Artifacts: map[string]ReplayArtifact{
				"baseline":  baseline,
				"candidate": baseline,
			},
		},
		NewRunID: func() string { return "run-missing-cursor" },
	}

	_, err := engine.Run(obs.ReplayRunRequest{
		BaselineRef:      "baseline",
		CandidatePlanRef: "candidate",
		Mode:             obs.ReplayModeReplayDecisions,
		Cursor: &obs.ReplayCursor{
			SessionID:       "sess-e",
			TurnID:          "turn-e",
			Lane:            eventabi.LaneData,
			RuntimeSequence: 99,
			EventID:         "evt-missing",
		},
	})
	if err == nil {
		t.Fatalf("expected missing cursor boundary error")
	}
	if !errors.Is(err, ErrReplayCursorNotFound) {
		t.Fatalf("expected ErrReplayCursorNotFound, got %v", err)
	}
}

func TestEngineRunAddsOrderingDivergenceWhenCandidateCursorBoundaryMissing(t *testing.T) {
	t.Parallel()

	baseline := ReplayArtifact{
		TraceArtifacts: []TraceArtifact{
			testTraceArtifact("sess-o", "turn-o", "evt-1", 10),
			testTraceArtifact("sess-o", "turn-o", "evt-2", 20),
		},
	}
	candidate := baseline.Clone()
	candidate.TraceArtifacts[1].Decision.EventID = "evt-2-other"

	engine := Engine{
		ArtifactResolver: StaticReplayArtifactResolver{
			Artifacts: map[string]ReplayArtifact{
				"baseline":  baseline,
				"candidate": candidate,
			},
		},
		NewRunID: func() string { return "run-ordering" },
	}

	result, err := engine.Run(obs.ReplayRunRequest{
		BaselineRef:      "baseline",
		CandidatePlanRef: "candidate",
		Mode:             obs.ReplayModeRecomputeDecisions,
		Cursor: &obs.ReplayCursor{
			SessionID:       "sess-o",
			TurnID:          "turn-o",
			Lane:            eventabi.LaneData,
			RuntimeSequence: 20,
			EventID:         "evt-2",
		},
	})
	if err != nil {
		t.Fatalf("unexpected replay run error: %v", err)
	}

	foundOrderingBoundary := false
	for _, divergence := range result.Divergences {
		if divergence.Class == obs.OrderingDivergence && divergence.Scope == "turn:turn-o" {
			foundOrderingBoundary = true
			break
		}
	}
	if !foundOrderingBoundary {
		t.Fatalf("expected ordering divergence for missing candidate resume boundary, got %+v", result.Divergences)
	}
}

func testTraceArtifact(sessionID, turnID, eventID string, sequence int64) TraceArtifact {
	return TraceArtifact{
		PlanHash:              "plan-a",
		SnapshotProvenanceRef: "snapshot-a",
		Lane:                  eventabi.LaneData,
		RuntimeSequence:       sequence,
		ProviderID:            "provider-a",
		ProviderModel:         "model-a",
		OrderingMarker:        "runtime_sequence:10",
		AuthorityEpoch:        7,
		RuntimeTimestampMS:    sequence,
		Decision: controlplane.DecisionOutcome{
			OutcomeKind:        controlplane.OutcomeAdmit,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeTurn,
			SessionID:          sessionID,
			TurnID:             turnID,
			EventID:            eventID,
			RuntimeTimestampMS: sequence,
			WallClockMS:        sequence,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_allow",
		},
	}
}
