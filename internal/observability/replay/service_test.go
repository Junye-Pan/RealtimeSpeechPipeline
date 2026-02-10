package replay

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

func TestAccessServiceAuthorizeReplayAccessWritesImmutableAuditEvent(t *testing.T) {
	t.Parallel()

	sink := &captureAuditSink{}
	service := AccessService{
		Policy:    DefaultAccessPolicy("tenant-a"),
		AuditSink: sink,
		NowMS: func() int64 {
			return 123
		},
	}

	decision, err := service.AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	})
	if err != nil {
		t.Fatalf("expected access authorization to succeed, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected access decision to be allowed, got %+v", decision)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected exactly one audit event append, got %d", len(sink.events))
	}
	if sink.events[0].TimestampMS != 123 {
		t.Fatalf("expected deterministic audit timestamp, got %+v", sink.events[0])
	}
}

func TestAccessServiceAuthorizeReplayAccessFailsClosedOnSinkWriteFailure(t *testing.T) {
	t.Parallel()

	sink := &captureAuditSink{appendErr: errors.New("append failed")}
	service := AccessService{
		Policy:    DefaultAccessPolicy("tenant-a"),
		AuditSink: sink,
	}

	decision, err := service.AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	})
	if err == nil {
		t.Fatalf("expected sink append error")
	}
	if decision.Allowed || decision.DenyReason != "audit_sink_write_failed" {
		t.Fatalf("expected fail-closed decision on sink failure, got %+v", decision)
	}
}

func TestAccessServiceAuthorizeReplayAccessRequiresAuditSink(t *testing.T) {
	t.Parallel()

	service := AccessService{
		Policy: DefaultAccessPolicy("tenant-a"),
	}

	decision, err := service.AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	})
	if !errors.Is(err, ErrImmutableReplayAuditSinkRequired) {
		t.Fatalf("expected missing audit sink error, got %v", err)
	}
	if decision.Allowed || decision.DenyReason != "audit_sink_unavailable" {
		t.Fatalf("expected fail-closed decision without audit sink, got %+v", decision)
	}
}

func TestAccessServiceAuthorizeReplayAccessWithResolverBackedAuditSink(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "audit")
	service := AccessService{
		Policy: DefaultAccessPolicy("tenant-a"),
		AuditSink: ResolverBackedAuditSink{
			Resolver: JSONLFileAuditBackendResolver{RootDir: root},
		},
		NowMS: func() int64 { return 456 },
	}

	decision, err := service.AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	})
	if err != nil {
		t.Fatalf("expected resolver-backed service authorization to pass, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed decision, got %+v", decision)
	}

	resolver := JSONLFileAuditBackendResolver{RootDir: root}
	path, err := resolver.TenantLogPath("tenant-a")
	if err != nil {
		t.Fatalf("unexpected resolver path error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unexpected audit log read error: %v", err)
	}
	if !strings.Contains(string(raw), "\"timestamp_ms\":456") {
		t.Fatalf("expected persisted deterministic replay audit event, got %q", string(raw))
	}
}

func TestAccessServiceAuthorizeReplayAccessFailClosedOnResolverError(t *testing.T) {
	t.Parallel()

	service := AccessService{
		Policy: DefaultAccessPolicy("tenant-a"),
		AuditSink: ResolverBackedAuditSink{
			Resolver: stubReplayAuditBackendResolver{
				resolveFn: func(string) (ImmutableReplayAuditSink, error) {
					return nil, errors.New("resolver unavailable")
				},
			},
		},
	}

	decision, err := service.AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	})
	if err == nil {
		t.Fatalf("expected resolver error")
	}
	if decision.Allowed || decision.DenyReason != "audit_sink_write_failed" {
		t.Fatalf("expected fail-closed decision on resolver error, got %+v", decision)
	}
}

func TestResolverBackedAuditSinkAppendResolvesTenantBackend(t *testing.T) {
	t.Parallel()

	tenantSink := &captureAuditSink{}
	sink := ResolverBackedAuditSink{
		Resolver: stubReplayAuditBackendResolver{
			resolveFn: func(tenantID string) (ImmutableReplayAuditSink, error) {
				if tenantID != "tenant-a" {
					t.Fatalf("expected tenant-a backend resolution, got %s", tenantID)
				}
				return tenantSink, nil
			},
		},
	}

	err := sink.AppendReplayAuditEvent(obs.ReplayAuditEvent{
		TimestampMS: 1,
		Request: obs.ReplayAccessRequest{
			TenantID:          "tenant-a",
			PrincipalID:       "user-1",
			Role:              "oncall",
			Purpose:           "incident",
			RequestedScope:    "turn:1",
			RequestedFidelity: obs.ReplayFidelityL0,
		},
		Decision: obs.ReplayAccessDecision{Allowed: true},
	})
	if err != nil {
		t.Fatalf("unexpected resolver-backed append error: %v", err)
	}
	if len(tenantSink.events) != 1 {
		t.Fatalf("expected one backend append event, got %d", len(tenantSink.events))
	}
}

func TestResolverBackedAuditSinkFailsWithoutResolver(t *testing.T) {
	t.Parallel()

	sink := ResolverBackedAuditSink{}
	err := sink.AppendReplayAuditEvent(obs.ReplayAuditEvent{
		Request: obs.ReplayAccessRequest{
			TenantID:          "tenant-a",
			PrincipalID:       "user-1",
			Role:              "oncall",
			Purpose:           "incident",
			RequestedScope:    "turn:1",
			RequestedFidelity: obs.ReplayFidelityL0,
		},
		Decision: obs.ReplayAccessDecision{Allowed: true},
	})
	if !errors.Is(err, ErrReplayAuditBackendResolverNil) {
		t.Fatalf("expected missing resolver error, got %v", err)
	}
}

func TestResolverBackedAuditSinkFailsWhenResolverReturnsNilBackend(t *testing.T) {
	t.Parallel()

	sink := ResolverBackedAuditSink{
		Resolver: stubReplayAuditBackendResolver{
			resolveFn: func(string) (ImmutableReplayAuditSink, error) {
				return nil, nil
			},
		},
	}
	err := sink.AppendReplayAuditEvent(obs.ReplayAuditEvent{
		Request: obs.ReplayAccessRequest{
			TenantID:          "tenant-a",
			PrincipalID:       "user-1",
			Role:              "oncall",
			Purpose:           "incident",
			RequestedScope:    "turn:1",
			RequestedFidelity: obs.ReplayFidelityL0,
		},
		Decision: obs.ReplayAccessDecision{Allowed: true},
	})
	if !errors.Is(err, ErrReplayAuditBackendNil) {
		t.Fatalf("expected nil backend error, got %v", err)
	}
}

type captureAuditSink struct {
	events    []obs.ReplayAuditEvent
	appendErr error
}

func (s *captureAuditSink) AppendReplayAuditEvent(event obs.ReplayAuditEvent) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.events = append(s.events, event)
	return nil
}

type stubReplayAuditBackendResolver struct {
	resolveFn func(tenantID string) (ImmutableReplayAuditSink, error)
}

func (s stubReplayAuditBackendResolver) ResolveReplayAuditBackend(tenantID string) (ImmutableReplayAuditSink, error) {
	return s.resolveFn(tenantID)
}
