package integration_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestCF002ProviderLateOutputAfterCancelIsDropped(t *testing.T) {
	t.Parallel()

	fence := runtimetransport.NewOutputFence()
	_, err := fence.EvaluateOutput(runtimetransport.OutputAttempt{
		SessionID:            "sess-cf-002",
		TurnID:               "turn-cf-002",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-cf-002-cancel",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       8,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected cancel fence error: %v", err)
	}

	late, err := fence.EvaluateOutput(runtimetransport.OutputAttempt{
		SessionID:            "sess-cf-002",
		TurnID:               "turn-cf-002",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-cf-002-late-provider",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       8,
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
	})
	if err != nil {
		t.Fatalf("unexpected late output evaluation error: %v", err)
	}
	if late.Accepted || late.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected deterministic late-output fence with playback_cancelled, got %+v", late)
	}
}

func TestCF003CancelTerminalizationExactlyOnce(t *testing.T) {
	t.Parallel()

	arbiter := turnarbiter.New()
	result, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-cf-003",
		TurnID:               "turn-cf-003",
		EventID:              "evt-cf-003",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   120,
		WallClockTimestampMS: 120,
		AuthorityEpoch:       8,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected cancel terminalization error: %v", err)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected exactly two lifecycle events, got %+v", result.Events)
	}
	if result.Events[0].Name != "abort" || result.Events[0].Reason != "cancelled" || result.Events[1].Name != "close" {
		t.Fatalf("expected abort(cancelled)->close, got %+v", result.Events)
	}
}

func TestCF004CancelObservabilityMarkersInOR02(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 4, DetailCapacity: 4})
	cancelSent := int64(100)
	cancelAccepted := int64(105)
	cancelFence := int64(118)
	openProposed := int64(0)
	open := int64(80)
	firstOutput := int64(300)
	evidence := timeline.BaselineEvidence{
		SessionID:        "sess-cf-004",
		TurnID:           "turn-cf-004",
		PipelineVersion:  "pipeline-v1",
		EventID:          "evt-cf-004",
		EnvelopeSnapshot: "event:cancelled",
		PayloadTags:      []eventabi.PayloadClass{eventabi.PayloadMetadata},
		PlanHash:         "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		DecisionOutcomes: []controlplane.DecisionOutcome{{
			OutcomeKind:        controlplane.OutcomeAdmit,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeTurn,
			SessionID:          "sess-cf-004",
			TurnID:             "turn-cf-004",
			EventID:            "evt-cf-004-admit",
			RuntimeTimestampMS: 80,
			WallClockMS:        80,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_allow",
		}},
		DeterminismSeed:        42,
		OrderingMarkers:        []string{"runtime_sequence", "event_id"},
		MergeRuleID:            "default-merge-rule",
		MergeRuleVersion:       "v1.0.0",
		AuthorityEpoch:         6,
		TerminalOutcome:        "abort",
		TerminalReason:         "cancelled",
		CloseEmitted:           true,
		TurnOpenProposedAtMS:   &openProposed,
		TurnOpenAtMS:           &open,
		FirstOutputAtMS:        &firstOutput,
		CancelSentAtMS:         &cancelSent,
		CancelAcceptedAtMS:     &cancelAccepted,
		CancelFenceAppliedAtMS: &cancelFence,
	}
	if err := recorder.AppendBaseline(evidence); err != nil {
		t.Fatalf("unexpected OR-02 baseline append error: %v", err)
	}
	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one OR-02 baseline entry, got %d", len(entries))
	}
	if entries[0].CancelSentAtMS == nil || entries[0].CancelAcceptedAtMS == nil || entries[0].CancelFenceAppliedAtMS == nil {
		t.Fatalf("expected cancel markers in OR-02 evidence, got %+v", entries[0])
	}
}
