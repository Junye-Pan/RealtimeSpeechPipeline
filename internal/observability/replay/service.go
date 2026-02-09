package replay

import (
	"errors"
	"fmt"
	"time"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

var (
	ErrImmutableReplayAuditSinkRequired = errors.New("immutable replay audit sink is required")
)

// ImmutableReplayAuditSink is an append-only audit sink for replay access events.
type ImmutableReplayAuditSink interface {
	AppendReplayAuditEvent(event obs.ReplayAuditEvent) error
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
