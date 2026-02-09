package observability

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestReplayAccessRequestValidate(t *testing.T) {
	t.Parallel()

	req := ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:123",
		RequestedFidelity: ReplayFidelityL0,
		RequestedClasses:  []eventabi.PayloadClass{eventabi.PayloadMetadata},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid replay access request, got %v", err)
	}

	req.RequestedFidelity = "L9"
	if err := req.Validate(); err == nil {
		t.Fatalf("expected invalid replay fidelity to fail validation")
	}
}

func TestReplayAuditEventValidate(t *testing.T) {
	t.Parallel()

	event := ReplayAuditEvent{
		TimestampMS: 123,
		Request: ReplayAccessRequest{
			TenantID:          "tenant-a",
			PrincipalID:       "user-1",
			Role:              "oncall",
			Purpose:           "incident",
			RequestedScope:    "turn:123",
			RequestedFidelity: ReplayFidelityL0,
		},
		Decision: ReplayAccessDecision{
			Allowed:    false,
			DenyReason: "missing_role_scope",
			RedactionMarkers: []ReplayRedactionMarker{
				{
					PayloadClass: eventabi.PayloadTextRaw,
					Action:       eventabi.RedactionMask,
					Reason:       "default_or03_policy",
				},
			},
		},
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid replay audit event, got %v", err)
	}

	event.Decision.DenyReason = ""
	if err := event.Validate(); err == nil {
		t.Fatalf("expected denied event without reason to fail validation")
	}
}
