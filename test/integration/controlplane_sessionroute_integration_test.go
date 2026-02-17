package integration_test

import (
	"testing"
	"time"

	cpapi "github.com/tiger/realtime-speech-pipeline/api/controlplane"
	transportapi "github.com/tiger/realtime-speech-pipeline/api/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/sessionroute"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestControlPlaneSessionRouteTokenStatusBootstrapIntegration(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 1, 30, 0, 0, time.UTC)
	service := sessionroute.NewService()
	service.Now = func() time.Time { return now }
	service.TokenSecret = "integration-secret"

	route, err := service.ResolveSessionRoute(cpapi.SessionRouteRequest{
		TenantID:                 "tenant-cp03",
		SessionID:                "sess-cp03",
		RequestedPipelineVersion: "pipeline-v1",
		TransportKind:            "livekit",
		RequestedAuthorityEpoch:  11,
	})
	if err != nil {
		t.Fatalf("resolve session route failed: %v", err)
	}
	if err := route.Validate(); err != nil {
		t.Fatalf("resolved route failed validation: %v", err)
	}
	if route.RoutingViewSnapshot == "" || route.AdmissionPolicySnapshot == "" {
		t.Fatalf("expected versioned route snapshots, got %+v", route)
	}

	token, err := service.IssueSessionToken(cpapi.SessionTokenRequest{
		TenantID:                 "tenant-cp03",
		SessionID:                "sess-cp03",
		RequestedPipelineVersion: "pipeline-v1",
		TransportKind:            "livekit",
		RequestedAuthorityEpoch:  11,
		TTLSeconds:               120,
	})
	if err != nil {
		t.Fatalf("issue session token failed: %v", err)
	}
	if err := token.Validate(); err != nil {
		t.Fatalf("issued token failed validation: %v", err)
	}
	if token.Claims.PipelineVersion != route.PipelineVersion {
		t.Fatalf("expected token claims pipeline version to match resolved route, route=%+v token=%+v", route, token)
	}

	bootstrap := transportapi.SessionBootstrap{
		SchemaVersion:   "v1.0.0",
		TransportKind:   transportapi.TransportKind(route.Endpoint.TransportKind),
		TenantID:        route.TenantID,
		SessionID:       route.SessionID,
		PipelineVersion: route.PipelineVersion,
		AuthorityEpoch:  route.Lease.AuthorityEpoch,
		LeaseTokenRef:   token.TokenID,
		RouteRef:        route.Endpoint.Endpoint,
		RequestedAtMS:   now.UnixMilli(),
	}
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("session bootstrap validation failed: %v", err)
	}

	status, err := service.GetSessionStatus(cpapi.SessionStatusRequest{
		TenantID:  "tenant-cp03",
		SessionID: "sess-cp03",
	})
	if err != nil {
		t.Fatalf("get session status failed: %v", err)
	}
	if status.Status != cpapi.SessionStatusConnected || status.LastReason != "session_token_issued" {
		t.Fatalf("expected connected status after token issue, got %+v", status)
	}
}

func TestControlPlaneExpiredLeaseTokenDeauthorizesTurnOpen(t *testing.T) {
	t.Parallel()

	recorder := timeline.NewRecorder(timeline.StageAConfig{BaselineCapacity: 8, DetailCapacity: 8})
	arbiter := turnarbiter.NewWithControlPlaneBackends(&recorder, turnarbiter.ControlPlaneBackends{
		Lease: leaseBackendFunc{
			resolve: func(in lease.Input) (lease.Output, error) {
				valid := true
				authorized := true
				return lease.Output{
					LeaseResolutionSnapshot: "lease-resolution/expired",
					AuthorityEpoch:          in.RequestedAuthorityEpoch,
					AuthorityEpochValid:     &valid,
					AuthorityAuthorized:     &authorized,
					LeaseTokenID:            "lease-token-expired",
					LeaseExpiresAtUTC:       "2026-02-17T00:00:00Z",
				}, nil
			},
		},
	})

	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		TenantID:             "tenant-cp04",
		SessionID:            "sess-cp04-expired",
		TurnID:               "turn-cp04-expired",
		EventID:              "evt-cp04-expired",
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       11,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected turn-open error: %v", err)
	}
	if open.Decision == nil || open.Decision.OutcomeKind != cpapi.OutcomeDeauthorized {
		t.Fatalf("expected deauthorized outcome for expired lease token, got %+v", open.Decision)
	}
	if open.Decision.Reason == "" {
		t.Fatalf("expected non-empty reason for expired lease token deauthorization, got %+v", open.Decision)
	}
	if open.Plan != nil {
		t.Fatalf("expected no plan when lease token is expired, got %+v", open.Plan)
	}
}

type leaseBackendFunc struct {
	resolve func(in lease.Input) (lease.Output, error)
}

func (b leaseBackendFunc) Resolve(in lease.Input) (lease.Output, error) {
	return b.resolve(in)
}
