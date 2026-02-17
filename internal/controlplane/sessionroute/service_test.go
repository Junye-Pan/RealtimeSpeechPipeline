package sessionroute

import (
	"strings"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

func TestResolveSessionRouteDefaults(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, time.February, 17, 0, 30, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }

	route, err := svc.ResolveSessionRoute(controlplane.SessionRouteRequest{
		TenantID:                "tenant-1",
		SessionID:               "sess-1",
		RequestedAuthorityEpoch: 5,
	})
	if err != nil {
		t.Fatalf("resolve session route failed: %v", err)
	}
	if route.PipelineVersion != registry.DefaultPipelineVersion {
		t.Fatalf("unexpected pipeline version: %+v", route)
	}
	if route.Endpoint.TransportKind != defaultTransportKind {
		t.Fatalf("unexpected transport kind: %+v", route)
	}
	if !strings.HasPrefix(route.Endpoint.Endpoint, "wss://") {
		t.Fatalf("expected wss endpoint, got %+v", route.Endpoint)
	}
	if route.Lease.TokenRef.TokenID == "" || route.Lease.TokenRef.ExpiresAtUTC == "" {
		t.Fatalf("expected lease token metadata in route, got %+v", route.Lease)
	}

	status, err := svc.GetSessionStatus(controlplane.SessionStatusRequest{
		TenantID:  "tenant-1",
		SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("get session status failed: %v", err)
	}
	if status.Status != controlplane.SessionStatusConnected {
		t.Fatalf("expected connected status after route resolve, got %+v", status)
	}
}

func TestIssueSessionToken(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, time.February, 17, 0, 35, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	svc.TokenSecret = "test-secret"

	out, err := svc.IssueSessionToken(controlplane.SessionTokenRequest{
		TenantID:                 "tenant-1",
		SessionID:                "sess-1",
		RequestedPipelineVersion: "pipeline-v1",
		TransportKind:            "livekit",
		RequestedAuthorityEpoch:  3,
		TTLSeconds:               120,
	})
	if err != nil {
		t.Fatalf("issue session token failed: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected valid signed session token, got %v", err)
	}
	if !strings.Contains(out.Token, ".") {
		t.Fatalf("expected signed token envelope, got %q", out.Token)
	}

	status, err := svc.GetSessionStatus(controlplane.SessionStatusRequest{
		TenantID:  "tenant-1",
		SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("get session status failed: %v", err)
	}
	if status.LastReason != "session_token_issued" {
		t.Fatalf("expected token-issued status update, got %+v", status)
	}
}

func TestGetSessionStatusNotFoundDefaultsToEnded(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, time.February, 17, 0, 40, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }

	status, err := svc.GetSessionStatus(controlplane.SessionStatusRequest{
		TenantID:  "tenant-unknown",
		SessionID: "sess-unknown",
	})
	if err != nil {
		t.Fatalf("unexpected status lookup error: %v", err)
	}
	if status.Status != controlplane.SessionStatusEnded {
		t.Fatalf("expected ended status fallback, got %+v", status)
	}
	if status.LastReason != "session_not_found" {
		t.Fatalf("expected session_not_found reason, got %+v", status)
	}
}

func TestResolveSessionRouteUsesBackends(t *testing.T) {
	t.Parallel()

	svc := NewService()
	now := time.Date(2026, time.February, 17, 0, 45, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	svc.Registry.Backend = stubRegistryBackend{resolveFn: func(string) (registry.PipelineRecord, error) {
		return registry.PipelineRecord{
			PipelineVersion:    "pipeline-registry",
			GraphDefinitionRef: "graph/custom",
			ExecutionProfile:   "simple",
		}, nil
	}}
	svc.Rollout.Backend = stubRolloutBackend{resolveFn: func(rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
		return rollout.ResolveVersionOutput{
			PipelineVersion:           "pipeline-rollout",
			VersionResolutionSnapshot: "version-resolution/custom",
		}, nil
	}}
	svc.RoutingView.Backend = stubRoutingBackend{resolveFn: func(routingview.Input) (routingview.Snapshot, error) {
		return routingview.Snapshot{
			RoutingViewSnapshot:      "routing-view/custom",
			AdmissionPolicySnapshot:  "admission-policy/custom",
			ABICompatibilitySnapshot: "abi-compat/custom",
		}, nil
	}}
	svc.Lease.Backend = stubLeaseBackend{resolveFn: func(lease.Input) (lease.Output, error) {
		valid := true
		granted := true
		return lease.Output{
			LeaseResolutionSnapshot: "lease-resolution/custom",
			AuthorityEpoch:          42,
			AuthorityEpochValid:     &valid,
			AuthorityAuthorized:     &granted,
			LeaseTokenID:            "lease-token-custom",
			LeaseExpiresAtUTC:       "2026-02-17T01:00:00Z",
		}, nil
	}}

	route, err := svc.ResolveSessionRoute(controlplane.SessionRouteRequest{
		TenantID:                 "tenant-1",
		SessionID:                "sess-1",
		RequestedPipelineVersion: "pipeline-requested",
		RequestedAuthorityEpoch:  42,
	})
	if err != nil {
		t.Fatalf("resolve route with backends failed: %v", err)
	}
	if route.PipelineVersion != "pipeline-rollout" {
		t.Fatalf("expected rollout pipeline version, got %+v", route)
	}
	if route.RoutingViewSnapshot != "routing-view/custom" || route.AdmissionPolicySnapshot != "admission-policy/custom" {
		t.Fatalf("expected backend routing snapshot overrides, got %+v", route)
	}
	if route.Lease.AuthorityEpoch != 42 || route.Lease.TokenRef.TokenID != "lease-token-custom" {
		t.Fatalf("expected backend lease metadata, got %+v", route.Lease)
	}
}

type stubRegistryBackend struct {
	resolveFn func(string) (registry.PipelineRecord, error)
}

func (s stubRegistryBackend) ResolvePipelineRecord(version string) (registry.PipelineRecord, error) {
	return s.resolveFn(version)
}

type stubRolloutBackend struct {
	resolveFn func(rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error)
}

func (s stubRolloutBackend) ResolvePipelineVersion(in rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
	return s.resolveFn(in)
}

type stubRoutingBackend struct {
	resolveFn func(routingview.Input) (routingview.Snapshot, error)
}

func (s stubRoutingBackend) GetSnapshot(in routingview.Input) (routingview.Snapshot, error) {
	return s.resolveFn(in)
}

type stubLeaseBackend struct {
	resolveFn func(lease.Input) (lease.Output, error)
}

func (s stubLeaseBackend) Resolve(in lease.Input) (lease.Output, error) {
	return s.resolveFn(in)
}
