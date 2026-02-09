package replay

import (
	"errors"
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
