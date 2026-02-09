package turnarbiter

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
)

func TestHandleTurnOpenProposedSuccess(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:             "sess-1",
		TurnID:                "turn-1",
		EventID:               "evt-1",
		RuntimeTimestampMS:    1,
		WallClockTimestampMS:  1,
		PipelineVersion:       "pipeline-v1",
		AuthorityEpoch:        2,
		SnapshotValid:         true,
		AuthorityEpochValid:   true,
		AuthorityAuthorized:   true,
		SnapshotFailurePolicy: controlplane.OutcomeDefer,
		PlanFailurePolicy:     controlplane.OutcomeReject,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnActive {
		t.Fatalf("expected Active, got %s", result.State)
	}
	if result.Plan == nil {
		t.Fatalf("expected plan to be materialized")
	}
	if len(result.Transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(result.Transitions))
	}
	if result.Transitions[1].Trigger != controlplane.TriggerTurnOpen {
		t.Fatalf("expected second transition trigger turn_open, got %s", result.Transitions[1].Trigger)
	}
}

func TestHandleTurnOpenProposedSnapshotInvalidStaysPreTurn(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:             "sess-1",
		TurnID:                "turn-1",
		EventID:               "evt-2",
		RuntimeTimestampMS:    2,
		WallClockTimestampMS:  2,
		PipelineVersion:       "pipeline-v1",
		AuthorityEpoch:        3,
		SnapshotValid:         false,
		AuthorityEpochValid:   true,
		AuthorityAuthorized:   true,
		SnapshotFailurePolicy: controlplane.OutcomeDefer,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnIdle {
		t.Fatalf("expected Idle, got %s", result.State)
	}
	if result.Decision == nil || result.Decision.OutcomeKind != controlplane.OutcomeDefer {
		t.Fatalf("expected pre-turn defer outcome, got %+v", result.Decision)
	}
	if containsLifecycleEvent(result.Events, "abort") || containsLifecycleEvent(result.Events, "close") {
		t.Fatalf("pre-turn failure must not emit abort/close")
	}
}

func TestHandleTurnOpenProposedAdmissionRejectPrecedesAuthority(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-1",
		TurnID:               "turn-admit-1",
		EventID:              "evt-admit-1",
		RuntimeTimestampMS:   2,
		WallClockTimestampMS: 2,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       3,
		SnapshotValid:        true,
		CapacityDisposition:  localadmission.CapacityReject,
		AuthorityEpochValid:  false,
		AuthorityAuthorized:  false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnIdle {
		t.Fatalf("expected Idle, got %s", result.State)
	}
	if result.Decision == nil || result.Decision.OutcomeKind != controlplane.OutcomeReject {
		t.Fatalf("expected pre-turn reject from admission, got %+v", result.Decision)
	}
}

func TestHandleTurnOpenProposedPreTurnDeauthorized(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-1",
		TurnID:               "turn-auth-1",
		EventID:              "evt-auth-1",
		RuntimeTimestampMS:   3,
		WallClockTimestampMS: 3,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       4,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnIdle {
		t.Fatalf("expected Idle on pre-turn deauthorization, got %s", result.State)
	}
	if result.Decision == nil || result.Decision.OutcomeKind != controlplane.OutcomeDeauthorized {
		t.Fatalf("expected deauthorized_drain, got %+v", result.Decision)
	}
	if containsLifecycleEvent(result.Events, "abort") || containsLifecycleEvent(result.Events, "close") {
		t.Fatalf("pre-turn deauthorization must not emit abort/close")
	}
}

func TestHandleTurnOpenProposedPlanMaterializationFailure(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-1",
		TurnID:               "turn-2",
		EventID:              "evt-3",
		RuntimeTimestampMS:   3,
		WallClockTimestampMS: 3,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       4,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
		PlanShouldFail:       true,
		PlanFailurePolicy:    controlplane.OutcomeReject,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnIdle {
		t.Fatalf("expected Idle, got %s", result.State)
	}
	if result.Decision == nil || result.Decision.OutcomeKind != controlplane.OutcomeReject {
		t.Fatalf("expected deterministic reject outcome, got %+v", result.Decision)
	}
	if containsLifecycleEvent(result.Events, "turn_open") {
		t.Fatalf("plan failure must not emit turn_open")
	}
}

func TestHandleActiveAuthorityRevokeWinsSamePointCancel(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-1",
		TurnID:               "turn-3",
		EventID:              "evt-4",
		RuntimeTimestampMS:   4,
		WallClockTimestampMS: 4,
		AuthorityEpoch:       5,
		AuthorityRevoked:     true,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnClosed {
		t.Fatalf("expected Closed, got %s", result.State)
	}
	if len(result.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(result.Events))
	}
	if result.Events[0].Name != "deauthorized_drain" {
		t.Fatalf("expected deauthorized_drain first, got %s", result.Events[0].Name)
	}
	if result.Events[1].Name != "abort" || result.Events[1].Reason != "authority_loss" {
		t.Fatalf("expected abort(authority_loss), got %+v", result.Events[1])
	}
	if result.Events[2].Name != "close" {
		t.Fatalf("expected close last, got %s", result.Events[2].Name)
	}
}

func TestHandleActiveCancelPath(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-1",
		TurnID:               "turn-4",
		EventID:              "evt-5",
		RuntimeTimestampMS:   5,
		WallClockTimestampMS: 5,
		AuthorityEpoch:       6,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected abort+close, got %d events", len(result.Events))
	}
	if result.Events[0].Name != "abort" || result.Events[0].Reason != "cancelled" {
		t.Fatalf("expected abort(cancelled), got %+v", result.Events[0])
	}
	if result.Events[1].Name != "close" {
		t.Fatalf("expected close, got %+v", result.Events[1])
	}
}

func TestHandleActiveProviderFailurePath(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-1",
		TurnID:               "turn-provider-fail-1",
		EventID:              "evt-provider-fail-1",
		RuntimeTimestampMS:   6,
		WallClockTimestampMS: 6,
		AuthorityEpoch:       7,
		ProviderFailure:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected abort+close, got %d events", len(result.Events))
	}
	if result.Events[0].Name != "abort" || result.Events[0].Reason != "provider_failure" {
		t.Fatalf("expected abort(provider_failure), got %+v", result.Events[0])
	}
	if result.Events[1].Name != "close" {
		t.Fatalf("expected close, got %+v", result.Events[1])
	}
}

func TestHandleActiveNodeTimeoutOrFailurePath(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-1",
		TurnID:               "turn-node-fail-1",
		EventID:              "evt-node-fail-1",
		RuntimeTimestampMS:   7,
		WallClockTimestampMS: 7,
		AuthorityEpoch:       8,
		NodeTimeoutOrFailure: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected abort+close, got %d events", len(result.Events))
	}
	if result.Events[0].Name != "abort" || result.Events[0].Reason != "node_timeout_or_failure" {
		t.Fatalf("expected abort(node_timeout_or_failure), got %+v", result.Events[0])
	}
	if result.Events[1].Name != "close" {
		t.Fatalf("expected close, got %+v", result.Events[1])
	}
}

func TestHandleActiveTransportDisconnectOrStallPath(t *testing.T) {
	t.Parallel()

	arbiter := New()
	result, err := arbiter.HandleActive(ActiveInput{
		SessionID:                  "sess-1",
		TurnID:                     "turn-transport-fail-1",
		EventID:                    "evt-transport-fail-1",
		PipelineVersion:            "pipeline-v1",
		TransportSequence:          9,
		RuntimeSequence:            10,
		RuntimeTimestampMS:         8,
		WallClockTimestampMS:       8,
		AuthorityEpoch:             11,
		TransportDisconnectOrStall: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ControlLane) != 2 {
		t.Fatalf("expected disconnected+stall control signals, got %d", len(result.ControlLane))
	}
	if result.ControlLane[0].Signal != "disconnected" || result.ControlLane[1].Signal != "stall" {
		t.Fatalf("unexpected transport control signals: %+v", result.ControlLane)
	}
	if len(result.Events) != 4 || result.Events[2].Name != "abort" || result.Events[2].Reason != "transport_disconnect_or_stall" || result.Events[3].Name != "close" {
		t.Fatalf("expected disconnected/stall + abort(transport_disconnect_or_stall)->close, got %+v", result.Events)
	}
}

func TestApplyDispatchOpen(t *testing.T) {
	t.Parallel()

	arbiter := New()
	res, err := arbiter.Apply(ApplyInput{
		Open: &OpenRequest{
			SessionID:            "sess-apply-1",
			TurnID:               "turn-apply-1",
			EventID:              "evt-apply-open-1",
			RuntimeTimestampMS:   10,
			WallClockTimestampMS: 10,
			PipelineVersion:      "pipeline-v1",
			AuthorityEpoch:       1,
			SnapshotValid:        true,
			AuthorityEpochValid:  true,
			AuthorityAuthorized:  true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Open == nil || res.Open.State != controlplane.TurnActive {
		t.Fatalf("expected open apply result to be active")
	}
}

func TestApplyRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	arbiter := New()
	_, err := arbiter.Apply(ApplyInput{})
	if err == nil {
		t.Fatalf("expected error for empty apply input")
	}
}

func containsLifecycleEvent(events []LifecycleEvent, name string) bool {
	for _, e := range events {
		if e.Name == name {
			return true
		}
	}
	return false
}
