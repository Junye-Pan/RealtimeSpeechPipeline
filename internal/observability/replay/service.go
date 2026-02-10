package replay

import (
	"errors"
	"fmt"
	"time"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

var (
	ErrImmutableReplayAuditSinkRequired = errors.New("immutable replay audit sink is required")
	ErrReplayAuditBackendResolverNil    = errors.New("immutable replay audit backend resolver is required")
	ErrReplayAuditBackendNil            = errors.New("immutable replay audit backend is required")
)

// ImmutableReplayAuditSink is an append-only audit sink for replay access events.
type ImmutableReplayAuditSink interface {
	AppendReplayAuditEvent(event obs.ReplayAuditEvent) error
}

// ImmutableReplayAuditBackendResolver resolves tenant-scoped immutable replay-audit backends.
type ImmutableReplayAuditBackendResolver interface {
	ResolveReplayAuditBackend(tenantID string) (ImmutableReplayAuditSink, error)
}

// ResolverBackedAuditSink wraps a backend resolver as an immutable replay-audit sink.
type ResolverBackedAuditSink struct {
	Resolver ImmutableReplayAuditBackendResolver
}

// AppendReplayAuditEvent resolves a backend by tenant and appends an immutable audit event.
func (s ResolverBackedAuditSink) AppendReplayAuditEvent(event obs.ReplayAuditEvent) error {
	if s.Resolver == nil {
		return ErrReplayAuditBackendResolverNil
	}
	backend, err := s.Resolver.ResolveReplayAuditBackend(event.Request.TenantID)
	if err != nil {
		return fmt.Errorf("resolve replay audit backend: %w", err)
	}
	if backend == nil {
		return ErrReplayAuditBackendNil
	}
	if err := backend.AppendReplayAuditEvent(event); err != nil {
		return fmt.Errorf("append replay audit event to backend: %w", err)
	}
	return nil
}

// AccessService wires replay authorization decisions with immutable audit logging.
type AccessService struct {
	Policy    AccessPolicy
	AuditSink ImmutableReplayAuditSink
	NowMS     func() int64
}

// AuthorizeReplayAccess evaluates replay access and writes an immutable audit record.
// Any audit sink failure is fail-closed and returns a denied decision.
func (s AccessService) AuthorizeReplayAccess(req obs.ReplayAccessRequest) (obs.ReplayAccessDecision, error) {
	decision := AuthorizeReplayAccess(req, s.Policy)

	if s.AuditSink == nil {
		return deny("audit_sink_unavailable"), ErrImmutableReplayAuditSinkRequired
	}

	event, err := BuildReplayAuditEvent(s.nowMS(), req, decision)
	if err != nil {
		return deny("audit_event_invalid"), fmt.Errorf("build replay audit event: %w", err)
	}

	if err := s.AuditSink.AppendReplayAuditEvent(event); err != nil {
		return deny("audit_sink_write_failed"), fmt.Errorf("append replay audit event: %w", err)
	}

	return decision, nil
}

func (s AccessService) nowMS() int64 {
	if s.NowMS != nil {
		return s.NowMS()
	}
	return time.Now().UnixMilli()
}
