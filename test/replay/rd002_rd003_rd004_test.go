package replay_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	replaycmp "github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
)

func TestRD002RecomputeDecisionsTimingTolerance(t *testing.T) {
	t.Parallel()

	baseline := []replaycmp.TraceArtifact{
		{
			PlanHash:              "plan-rd2",
			SnapshotProvenanceRef: "snapshot-set-a",
			OrderingMarker:        "runtime_sequence:10",
			AuthorityEpoch:        7,
			RuntimeTimestampMS:    100,
			Decision: controlplane.DecisionOutcome{
				OutcomeKind:        controlplane.OutcomeAdmit,
				Phase:              controlplane.PhasePreTurn,
				Scope:              controlplane.ScopeSession,
				SessionID:          "sess-rd2",
				EventID:            "evt-rd2-1",
				RuntimeTimestampMS: 100,
				WallClockMS:        100,
				EmittedBy:          controlplane.EmitterRK25,
				Reason:             "admission_capacity_allow",
			},
		},
	}
	replayed := []replaycmp.TraceArtifact{
		{
			PlanHash:              "plan-rd2",
			SnapshotProvenanceRef: "snapshot-set-a",
			OrderingMarker:        "runtime_sequence:10",
			AuthorityEpoch:        7,
			RuntimeTimestampMS:    112,
			Decision:              baseline[0].Decision,
		},
	}

	divergences := replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: 15})
	if len(divergences) != 0 {
		t.Fatalf("expected no divergence when recompute timing is within tolerance, got %+v", divergences)
	}

	replayed[0].RuntimeTimestampMS = 140
	divergences = replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: 15})
	if len(divergences) != 1 || divergences[0].Class != obs.TimingDivergence {
		t.Fatalf("expected timing divergence when tolerance exceeded, got %+v", divergences)
	}
}

func TestRD003OR02BaselineCompletenessL0(t *testing.T) {
	t.Parallel()

	complete := baselineEvidence("turn-rd3-1")
	preTurnRejected := baselineEvidence("turn-rd3-2")
	preTurnRejected.TurnOpenAtMS = nil
	preTurnRejected.TerminalOutcome = "abort"
	preTurnRejected.TerminalReason = "snapshot_invalid_or_missing"
	preTurnRejected.CloseEmitted = true

	report := timeline.BaselineCompleteness([]timeline.BaselineEvidence{complete, preTurnRejected})
	if report.TotalAcceptedTurns != 1 {
		t.Fatalf("expected exactly one accepted turn, got %d", report.TotalAcceptedTurns)
	}
	if report.CompletenessRatio != 1.0 {
		t.Fatalf("expected 100%% completeness for accepted turns, got %.2f", report.CompletenessRatio)
	}
}

func TestRD004SnapshotProvenancePlanDivergence(t *testing.T) {
	t.Parallel()

	baseline := []replaycmp.TraceArtifact{
		{
			PlanHash:              "plan-rd4",
			SnapshotProvenanceRef: "snapshot-a",
			OrderingMarker:        "runtime_sequence:10",
			AuthorityEpoch:        8,
			RuntimeTimestampMS:    100,
			Decision: controlplane.DecisionOutcome{
				OutcomeKind:        controlplane.OutcomeAdmit,
				Phase:              controlplane.PhasePreTurn,
				Scope:              controlplane.ScopeSession,
				SessionID:          "sess-rd4",
				EventID:            "evt-rd4-1",
				RuntimeTimestampMS: 100,
				WallClockMS:        100,
				EmittedBy:          controlplane.EmitterRK25,
				Reason:             "admission_capacity_allow",
			},
		},
	}
	replayed := []replaycmp.TraceArtifact{
		{
			PlanHash:              "plan-rd4",
			SnapshotProvenanceRef: "snapshot-b",
			OrderingMarker:        "runtime_sequence:10",
			AuthorityEpoch:        8,
			RuntimeTimestampMS:    100,
			Decision:              baseline[0].Decision,
		},
	}

	divergences := replaycmp.CompareTraceArtifacts(baseline, replayed, replaycmp.CompareConfig{TimingToleranceMS: 10})
	if len(divergences) != 1 || divergences[0].Class != obs.PlanDivergence {
		t.Fatalf("expected PLAN_DIVERGENCE for snapshot provenance mismatch, got %+v", divergences)
	}
}

func baselineEvidence(turnID string) timeline.BaselineEvidence {
	openProposed := int64(0)
	open := int64(80)
	firstOutput := int64(500)
	decision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          "sess-rd3",
		TurnID:             turnID,
		EventID:            "evt-rd3",
		RuntimeTimestampMS: 80,
		WallClockMS:        80,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
	return timeline.BaselineEvidence{
		SessionID:        "sess-rd3",
		TurnID:           turnID,
		PipelineVersion:  "pipeline-v1",
		EventID:          "evt-rd3",
		EnvelopeSnapshot: "event:turn_open",
		PayloadTags:      []eventabi.PayloadClass{eventabi.PayloadMetadata},
		RedactionDecisions: []eventabi.RedactionDecision{
			{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow},
		},
		PlanHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		DecisionOutcomes:     []controlplane.DecisionOutcome{decision},
		DeterminismSeed:      42,
		OrderingMarkers:      []string{"runtime_sequence", "event_id"},
		MergeRuleID:          "default-merge-rule",
		MergeRuleVersion:     "v1.0.0",
		AuthorityEpoch:       6,
		TerminalOutcome:      "commit",
		CloseEmitted:         true,
		TurnOpenProposedAtMS: &openProposed,
		TurnOpenAtMS:         &open,
		FirstOutputAtMS:      &firstOutput,
	}
}
