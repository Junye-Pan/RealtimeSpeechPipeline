package turnarbiter

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
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

func TestHandleTurnOpenProposedUsesResolvedTurnStartBundle(t *testing.T) {
	t.Parallel()

	customSnapshot := controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/custom",
		AdmissionPolicySnapshot:   "admission-policy/custom",
		ABICompatibilitySnapshot:  "abi-compat/custom",
		VersionResolutionSnapshot: "version-resolution/custom",
		PolicyResolutionSnapshot:  "policy-resolution/custom",
		ProviderHealthSnapshot:    "provider-health/custom",
	}
	arbiter := NewWithDependencies(nil, stubTurnStartBundleResolver{
		bundle: TurnStartBundle{
			PipelineVersion:        "pipeline-custom",
			GraphDefinitionRef:     "graph/custom",
			ExecutionProfile:       "simple",
			AllowedAdaptiveActions: []string{"retry", "fallback"},
			SnapshotProvenance:     customSnapshot,
		},
	})

	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:             "sess-custom-1",
		TurnID:                "turn-custom-1",
		EventID:               "evt-custom-1",
		RuntimeTimestampMS:    11,
		WallClockTimestampMS:  11,
		PipelineVersion:       "pipeline-ignored",
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
	if result.Plan == nil {
		t.Fatalf("expected resolved plan")
	}
	if result.Plan.PipelineVersion != "pipeline-custom" || result.Plan.GraphDefinitionRef != "graph/custom" {
		t.Fatalf("expected resolved plan to use custom bundle, got %+v", result.Plan)
	}
	if !reflect.DeepEqual(result.Plan.SnapshotProvenance, customSnapshot) {
		t.Fatalf("expected custom snapshot provenance, got %+v", result.Plan.SnapshotProvenance)
	}
}

func TestHandleTurnOpenProposedAppliesCPAdmissionDecision(t *testing.T) {
	t.Parallel()

	arbiter := NewWithDependencies(nil, stubTurnStartBundleResolver{
		bundle: TurnStartBundle{
			PipelineVersion:        "pipeline-custom",
			GraphDefinitionRef:     "graph/custom",
			ExecutionProfile:       "simple",
			AllowedAdaptiveActions: []string{"retry", "fallback"},
			SnapshotProvenance:     defaultSnapshotProvenance(),
			HasCPAdmissionDecision: true,
			CPAdmissionOutcomeKind: controlplane.OutcomeDefer,
			CPAdmissionScope:       controlplane.ScopeSession,
			CPAdmissionReason:      "cp_admission_defer_capacity",
		},
	})

	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-cp-admission-1",
		TurnID:               "turn-cp-admission-1",
		EventID:              "evt-cp-admission-1",
		RuntimeTimestampMS:   19,
		WallClockTimestampMS: 19,
		AuthorityEpoch:       2,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnIdle {
		t.Fatalf("expected Idle after CP admission defer, got %s", result.State)
	}
	if result.Decision == nil {
		t.Fatalf("expected CP admission decision")
	}
	if result.Decision.OutcomeKind != controlplane.OutcomeDefer || result.Decision.EmittedBy != controlplane.EmitterCP05 {
		t.Fatalf("unexpected CP admission outcome: %+v", result.Decision)
	}
	if result.Decision.Reason != "cp_admission_defer_capacity" {
		t.Fatalf("unexpected CP admission reason: %+v", result.Decision)
	}
}

func TestHandleTurnOpenProposedAppliesLeaseDecisionToAuthorityGate(t *testing.T) {
	t.Parallel()

	arbiter := NewWithDependencies(nil, stubTurnStartBundleResolver{
		bundle: TurnStartBundle{
			PipelineVersion:        "pipeline-custom",
			GraphDefinitionRef:     "graph/custom",
			ExecutionProfile:       "simple",
			AllowedAdaptiveActions: []string{"retry", "fallback"},
			SnapshotProvenance:     defaultSnapshotProvenance(),
			HasLeaseDecision:       true,
			LeaseAuthorityEpoch:    41,
			LeaseAuthorityValid:    false,
			LeaseAuthorityGranted:  true,
		},
	})

	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-lease-gate-1",
		TurnID:               "turn-lease-gate-1",
		EventID:              "evt-lease-gate-1",
		RuntimeTimestampMS:   23,
		WallClockTimestampMS: 23,
		AuthorityEpoch:       40,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnIdle {
		t.Fatalf("expected Idle when lease reports stale epoch, got %s", result.State)
	}
	if result.Decision == nil || result.Decision.OutcomeKind != controlplane.OutcomeStaleEpochReject {
		t.Fatalf("expected stale_epoch_reject from lease authority gate, got %+v", result.Decision)
	}
}

func TestHandleTurnOpenProposedUsesLeaseAuthorityEpochForResolvedPlan(t *testing.T) {
	t.Parallel()

	arbiter := NewWithDependencies(nil, stubTurnStartBundleResolver{
		bundle: TurnStartBundle{
			PipelineVersion:        "pipeline-custom",
			GraphDefinitionRef:     "graph/custom",
			ExecutionProfile:       "simple",
			AllowedAdaptiveActions: []string{"retry", "fallback"},
			SnapshotProvenance:     defaultSnapshotProvenance(),
			HasLeaseDecision:       true,
			LeaseAuthorityEpoch:    88,
			LeaseAuthorityValid:    true,
			LeaseAuthorityGranted:  true,
		},
	})

	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-lease-epoch-1",
		TurnID:               "turn-lease-epoch-1",
		EventID:              "evt-lease-epoch-1",
		RuntimeTimestampMS:   29,
		WallClockTimestampMS: 29,
		AuthorityEpoch:       12,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnActive || result.Plan == nil {
		t.Fatalf("expected active turn with resolved plan, got %+v", result)
	}
	if result.Plan.AuthorityEpoch != 88 {
		t.Fatalf("expected lease authority epoch to be threaded into resolved plan, got %+v", result.Plan)
	}
}

func TestHandleTurnOpenProposedBundleResolutionFailureUsesPlanFailurePolicy(t *testing.T) {
	t.Parallel()

	arbiter := NewWithDependencies(nil, stubTurnStartBundleResolver{err: errors.New("resolver unavailable")})
	result, err := arbiter.HandleTurnOpenProposed(OpenRequest{
		SessionID:            "sess-bundle-fail-1",
		TurnID:               "turn-bundle-fail-1",
		EventID:              "evt-bundle-fail-1",
		RuntimeTimestampMS:   12,
		WallClockTimestampMS: 12,
		AuthorityEpoch:       1,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
		PlanFailurePolicy:    controlplane.OutcomeReject,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != controlplane.TurnIdle {
		t.Fatalf("expected Idle after bundle resolution failure, got %s", result.State)
	}
	if result.Decision == nil {
		t.Fatalf("expected deterministic decision outcome on bundle resolution failure")
	}
	if result.Decision.OutcomeKind != controlplane.OutcomeReject {
		t.Fatalf("expected plan failure policy reject outcome, got %+v", result.Decision)
	}
	if result.Decision.Reason != "turn_start_bundle_resolution_failed" {
		t.Fatalf("expected bundle resolution failure reason, got %+v", result.Decision)
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

func TestHandleActiveAppendsBaselineEvidenceOnTerminalPath(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 2, DetailCapacity: 2})
	arbiter := NewWithRecorder(&recorder)

	_, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-or02-1",
		TurnID:               "turn-or02-1",
		EventID:              "evt-or02-1",
		PipelineVersion:      "pipeline-v1",
		RuntimeSequence:      10,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		AuthorityEpoch:       1,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one OR-02 baseline entry, got %d", len(entries))
	}
	if entries[0].TerminalOutcome != "commit" {
		t.Fatalf("expected terminal outcome commit in OR-02 baseline, got %+v", entries[0])
	}
}

func TestHandleActiveBaselineAppendFailureFallsBackDeterministically(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 1, DetailCapacity: 2})
	arbiter := NewWithRecorder(&recorder)

	_, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-or02-2",
		TurnID:               "turn-or02-2a",
		EventID:              "evt-or02-2a",
		PipelineVersion:      "pipeline-v1",
		RuntimeSequence:      20,
		RuntimeTimestampMS:   200,
		WallClockTimestampMS: 200,
		AuthorityEpoch:       1,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}

	result, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-or02-2",
		TurnID:               "turn-or02-2b",
		EventID:              "evt-or02-2b",
		PipelineVersion:      "pipeline-v1",
		RuntimeSequence:      21,
		RuntimeTimestampMS:   210,
		WallClockTimestampMS: 210,
		AuthorityEpoch:       1,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Events) != 2 {
		t.Fatalf("expected fallback abort+close, got %+v", result.Events)
	}
	if result.Events[0].Name != "abort" || result.Events[0].Reason != "recording_evidence_unavailable" {
		t.Fatalf("expected fallback abort(recording_evidence_unavailable), got %+v", result.Events[0])
	}
	if result.Events[1].Name != "close" {
		t.Fatalf("expected close after fallback abort, got %+v", result.Events[1])
	}
}

func TestHandleActiveUsesResolvedSnapshotDefaultsForBaselineEvidence(t *testing.T) {
	t.Parallel()

	customSnapshot := controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/custom",
		AdmissionPolicySnapshot:   "admission-policy/custom",
		ABICompatibilitySnapshot:  "abi-compat/custom",
		VersionResolutionSnapshot: "version-resolution/custom",
		PolicyResolutionSnapshot:  "policy-resolution/custom",
		ProviderHealthSnapshot:    "provider-health/custom",
	}
	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 2, DetailCapacity: 2})
	arbiter := NewWithDependencies(&recorder, stubTurnStartBundleResolver{
		bundle: TurnStartBundle{
			PipelineVersion:        "pipeline-custom",
			GraphDefinitionRef:     "graph/custom",
			ExecutionProfile:       "simple",
			AllowedAdaptiveActions: []string{"retry"},
			SnapshotProvenance:     customSnapshot,
		},
	})

	_, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-or02-custom-1",
		TurnID:               "turn-or02-custom-1",
		EventID:              "evt-or02-custom-1",
		RuntimeSequence:      44,
		RuntimeTimestampMS:   444,
		WallClockTimestampMS: 444,
		AuthorityEpoch:       4,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one baseline entry, got %d", len(entries))
	}
	if !reflect.DeepEqual(entries[0].SnapshotProvenance, customSnapshot) {
		t.Fatalf("expected baseline to use custom snapshot defaults, got %+v", entries[0].SnapshotProvenance)
	}
}

func TestHandleActivePromotesProviderAttemptsToBaselineWhenOutcomesMissing(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 2, DetailCapacity: 2, AttemptCapacity: 8})
	if err := recorder.AppendProviderInvocationAttempts([]timeline.ProviderAttemptEvidence{
		{
			SessionID:            "sess-promote-1",
			TurnID:               "turn-promote-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-provider-1",
			ProviderInvocationID: "pvi-promote-1",
			Modality:             "stt",
			ProviderID:           "stt-a",
			Attempt:              1,
			OutcomeClass:         "overload",
			Retryable:            true,
			RetryDecision:        "provider_switch",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   10,
			WallClockTimestampMS: 10,
		},
		{
			SessionID:            "sess-promote-1",
			TurnID:               "turn-promote-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-provider-1",
			ProviderInvocationID: "pvi-promote-1",
			Modality:             "stt",
			ProviderID:           "stt-b",
			Attempt:              1,
			OutcomeClass:         "success",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    2,
			RuntimeSequence:      2,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   11,
			WallClockTimestampMS: 11,
		},
	}); err != nil {
		t.Fatalf("append attempts: %v", err)
	}

	arbiter := NewWithRecorder(&recorder)
	_, err := arbiter.HandleActive(ActiveInput{
		SessionID:            "sess-promote-1",
		TurnID:               "turn-promote-1",
		EventID:              "evt-active-promote-1",
		PipelineVersion:      "pipeline-v1",
		RuntimeSequence:      99,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		AuthorityEpoch:       1,
		TerminalSuccessReady: true,
	})
	if err != nil {
		t.Fatalf("unexpected active handling error: %v", err)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one baseline entry, got %d", len(entries))
	}
	if len(entries[0].InvocationOutcomes) != 1 {
		t.Fatalf("expected promoted invocation outcomes, got %+v", entries[0].InvocationOutcomes)
	}
	outcome := entries[0].InvocationOutcomes[0]
	if outcome.ProviderInvocationID != "pvi-promote-1" || outcome.ProviderID != "stt-b" || outcome.AttemptCount != 2 {
		t.Fatalf("expected promoted final attempt evidence, got %+v", outcome)
	}
	if outcome.FinalAttemptLatencyMS != 1 || outcome.TotalInvocationLatencyMS != 1 {
		t.Fatalf("expected promoted latency fields to equal 1ms, got %+v", outcome)
	}
}

func TestHandleActiveExplicitInvocationOutcomesOverridePromotedAttempts(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 2, DetailCapacity: 2, AttemptCapacity: 8})
	if err := recorder.AppendProviderInvocationAttempts([]timeline.ProviderAttemptEvidence{
		{
			SessionID:            "sess-promote-2",
			TurnID:               "turn-promote-2",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-provider-2",
			ProviderInvocationID: "pvi-promote-2",
			Modality:             "stt",
			ProviderID:           "stt-a",
			Attempt:              1,
			OutcomeClass:         "success",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   10,
			WallClockTimestampMS: 10,
		},
	}); err != nil {
		t.Fatalf("append attempts: %v", err)
	}

	explicit := timeline.InvocationOutcomeEvidence{
		ProviderInvocationID:     "pvi-explicit",
		Modality:                 "external",
		ProviderID:               "manual-provider",
		OutcomeClass:             "success",
		Retryable:                false,
		RetryDecision:            "none",
		AttemptCount:             1,
		FinalAttemptLatencyMS:    0,
		TotalInvocationLatencyMS: 0,
	}
	arbiter := NewWithRecorder(&recorder)
	_, err := arbiter.HandleActive(ActiveInput{
		SessionID:                  "sess-promote-2",
		TurnID:                     "turn-promote-2",
		EventID:                    "evt-active-promote-2",
		PipelineVersion:            "pipeline-v1",
		RuntimeSequence:            100,
		RuntimeTimestampMS:         101,
		WallClockTimestampMS:       101,
		AuthorityEpoch:             1,
		TerminalSuccessReady:       true,
		ProviderInvocationOutcomes: []timeline.InvocationOutcomeEvidence{explicit},
	})
	if err != nil {
		t.Fatalf("unexpected active handling error: %v", err)
	}

	entries := recorder.BaselineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one baseline entry, got %d", len(entries))
	}
	if len(entries[0].InvocationOutcomes) != 1 {
		t.Fatalf("expected one invocation outcome, got %+v", entries[0].InvocationOutcomes)
	}
	if !reflect.DeepEqual(entries[0].InvocationOutcomes[0], explicit) {
		t.Fatalf("expected explicit invocation outcome to win, got %+v", entries[0].InvocationOutcomes[0])
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

type stubTurnStartBundleResolver struct {
	bundle TurnStartBundle
	err    error
}

func (s stubTurnStartBundleResolver) ResolveTurnStartBundle(_ TurnStartBundleInput) (TurnStartBundle, error) {
	if s.err != nil {
		return TurnStartBundle{}, s.err
	}
	return s.bundle, nil
}
