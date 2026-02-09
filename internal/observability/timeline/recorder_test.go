package timeline

import (
	"errors"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestAppendBaselineStageACapacity(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{BaselineCapacity: 1, DetailCapacity: 4})
	if err := recorder.AppendBaseline(minimalBaseline("turn-a")); err != nil {
		t.Fatalf("unexpected first append error: %v", err)
	}
	if err := recorder.AppendBaseline(minimalBaseline("turn-b")); !errors.Is(err, ErrBaselineCapacityExhausted) {
		t.Fatalf("expected baseline capacity exhaustion, got %v", err)
	}
}

func TestAppendDetailOverflowEmitsDowngradeOncePerTurn(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{BaselineCapacity: 4, DetailCapacity: 1})
	first, err := recorder.AppendDetail(DetailEvent{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected first detail append error: %v", err)
	}
	if !first.Stored || first.Dropped {
		t.Fatalf("expected first detail append to store")
	}

	second, err := recorder.AppendDetail(DetailEvent{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-2",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
	})
	if err != nil {
		t.Fatalf("unexpected overflow append error: %v", err)
	}
	if !second.Dropped || !second.DowngradeEmitted || second.DowngradeSignal == nil {
		t.Fatalf("expected deterministic overflow downgrade signal, got %+v", second)
	}
	if second.DowngradeSignal.Signal != "recording_level_downgraded" || second.DowngradeSignal.EmittedBy != "OR-02" {
		t.Fatalf("unexpected downgrade signal: %+v", second.DowngradeSignal)
	}

	third, err := recorder.AppendDetail(DetailEvent{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-3",
		TransportSequence:    3,
		RuntimeSequence:      3,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   102,
		WallClockTimestampMS: 102,
	})
	if err != nil {
		t.Fatalf("unexpected repeated overflow append error: %v", err)
	}
	if !third.Dropped || third.DowngradeEmitted {
		t.Fatalf("expected repeated overflow to drop without duplicate downgrade marker, got %+v", third)
	}
}

func TestBaselineCompletenessReport(t *testing.T) {
	t.Parallel()

	complete := minimalBaseline("turn-complete")
	incomplete := minimalBaseline("turn-incomplete")
	incomplete.OrderingMarkers = nil

	report := BaselineCompleteness([]BaselineEvidence{complete, incomplete})
	if report.TotalAcceptedTurns != 2 {
		t.Fatalf("expected 2 accepted turns, got %d", report.TotalAcceptedTurns)
	}
	if report.CompleteAcceptedTurns != 1 {
		t.Fatalf("expected 1 complete accepted turn, got %d", report.CompleteAcceptedTurns)
	}
	if report.CompletenessRatio != 0.5 {
		t.Fatalf("expected completeness ratio 0.5, got %.2f", report.CompletenessRatio)
	}
	if len(report.IncompleteAcceptedTurnIDs) != 1 || report.IncompleteAcceptedTurnIDs[0] != "turn-incomplete" {
		t.Fatalf("unexpected incomplete turn ids: %+v", report.IncompleteAcceptedTurnIDs)
	}
}

func TestValidateCompletenessCancelMarkers(t *testing.T) {
	t.Parallel()

	cancelSent := int64(130)
	cancelAccepted := int64(132)
	cancelFence := int64(140)
	baseline := minimalBaseline("turn-cancel")
	baseline.CancelSentAtMS = &cancelSent
	baseline.CancelAcceptedAtMS = &cancelAccepted
	baseline.CancelFenceAppliedAtMS = &cancelFence
	baseline.TerminalOutcome = "abort"
	baseline.TerminalReason = "cancelled"
	if err := baseline.ValidateCompleteness(); err != nil {
		t.Fatalf("expected cancel baseline to validate: %v", err)
	}

	missingCancelSent := baseline
	missingCancelSent.CancelSentAtMS = nil
	if err := missingCancelSent.ValidateCompleteness(); err == nil {
		t.Fatalf("expected missing cancel_sent_at to fail completeness")
	}

	missingFence := baseline
	missingFence.CancelFenceAppliedAtMS = nil
	if err := missingFence.ValidateCompleteness(); err == nil {
		t.Fatalf("expected missing cancel fence marker to fail completeness")
	}
}

func minimalBaseline(turnID string) BaselineEvidence {
	decision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          "sess-1",
		TurnID:             turnID,
		EventID:            "evt-1",
		RuntimeTimestampMS: 100,
		WallClockMS:        100,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
	openProposed := int64(90)
	open := int64(100)
	firstOutput := int64(300)
	return BaselineEvidence{
		SessionID:        "sess-1",
		TurnID:           turnID,
		PipelineVersion:  "pipeline-v1",
		EventID:          "evt-1",
		EnvelopeSnapshot: "event:turn_open",
		PayloadTags:      []eventabi.PayloadClass{eventabi.PayloadMetadata},
		PlanHash:         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
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
		AuthorityEpoch:       7,
		MigrationMarkers:     []string{"lease_rotated"},
		TerminalOutcome:      "commit",
		CloseEmitted:         true,
		TurnOpenProposedAtMS: &openProposed,
		TurnOpenAtMS:         &open,
		FirstOutputAtMS:      &firstOutput,
		TerminalReason:       "",
	}
}
