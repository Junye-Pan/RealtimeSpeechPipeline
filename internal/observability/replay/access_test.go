package replay

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

func TestAuthorizeReplayAccessDenyByDefault(t *testing.T) {
	t.Parallel()

	policy := DefaultAccessPolicy("tenant-a")

	denied := AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	}, policy)
	if denied.Allowed {
		t.Fatalf("expected missing principal_id request to be denied")
	}
	if denied.DenyReason == "" {
		t.Fatalf("expected deny reason for denied request")
	}

	crossTenant := AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-b",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	}, policy)
	if crossTenant.Allowed || crossTenant.DenyReason != "cross_tenant_denied" {
		t.Fatalf("expected cross-tenant request denial, got %+v", crossTenant)
	}
}

func TestAuthorizeReplayAccessAllowsAndAddsRedactionMarkers(t *testing.T) {
	t.Parallel()

	req := obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
		RequestedClasses:  []eventabi.PayloadClass{eventabi.PayloadPII, eventabi.PayloadMetadata},
	}
	decision := AuthorizeReplayAccess(req, DefaultAccessPolicy("tenant-a"))
	if !decision.Allowed {
		t.Fatalf("expected authorized replay request, got %+v", decision)
	}
	if len(decision.RedactionMarkers) != 2 {
		t.Fatalf("expected 2 redaction markers, got %+v", decision.RedactionMarkers)
	}
	if decision.RedactionMarkers[0].PayloadClass == eventabi.PayloadPII && decision.RedactionMarkers[0].Action != eventabi.RedactionTokenize {
		t.Fatalf("expected PII to be tokenized, got %+v", decision.RedactionMarkers[0])
	}
}

func TestBuildReplayAuditEvent(t *testing.T) {
	t.Parallel()

	req := obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	}
	decision := obs.ReplayAccessDecision{Allowed: false, DenyReason: "role_not_authorized"}

	event, err := BuildReplayAuditEvent(123, req, decision)
	if err != nil {
		t.Fatalf("expected valid replay audit event build, got %v", err)
	}
	if event.Decision.DenyReason != "role_not_authorized" {
		t.Fatalf("unexpected replay audit event: %+v", event)
	}
}
