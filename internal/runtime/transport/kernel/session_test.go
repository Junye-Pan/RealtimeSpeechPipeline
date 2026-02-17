package kernel

import (
	"errors"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/authority"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/routing"
)

func TestSessionLifecycleIngressEgressAndRouting(t *testing.T) {
	t.Parallel()

	session, err := NewSession(Config{}, testBootstrap(), authority.NewGuard(nil), nil)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if _, err := session.OnConnection(apitransport.SignalConnected); err != nil {
		t.Fatalf("connected transition: %v", err)
	}

	transportSeq := int64(1)
	authorityEpoch := int64(2)
	record := eventabi.EventRecord{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeTurn,
		SessionID:          "sess-kernel-1",
		TurnID:             "turn-kernel-1",
		PipelineVersion:    "pipeline-v1",
		EventID:            "evt-kernel-ingress-1",
		Lane:               eventabi.LaneData,
		TransportSequence:  &transportSeq,
		RuntimeSequence:    1,
		AuthorityEpoch:     &authorityEpoch,
		RuntimeTimestampMS: 10,
		WallClockMS:        10,
	}
	normalized, err := session.NormalizeIngress(record, "opus")
	if err != nil {
		t.Fatalf("normalize ingress: %v", err)
	}
	if normalized.PayloadClass == "" {
		t.Fatalf("expected normalized payload class")
	}

	decision, err := session.DeliverEgress(runtimetransport.OutputAttempt{
		SessionID:            "sess-kernel-1",
		TurnID:               "turn-kernel-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-kernel-egress-1",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       2,
		RuntimeTimestampMS:   11,
		WallClockTimestampMS: 11,
	}, 16)
	if err != nil {
		t.Fatalf("deliver egress: %v", err)
	}
	if !decision.Accepted {
		t.Fatalf("expected egress decision accepted, got %+v", decision)
	}

	changed, err := session.ApplyRoutingUpdate(routing.View{Version: 1, ActiveEndpoint: "runtime-b", LeaseEpoch: 3})
	if err != nil {
		t.Fatalf("apply routing update: %v", err)
	}
	if !changed {
		t.Fatalf("expected routing update to report change")
	}
	if session.Bootstrap().AuthorityEpoch != 3 {
		t.Fatalf("expected bootstrap authority epoch to advance to 3, got %d", session.Bootstrap().AuthorityEpoch)
	}
}

func TestSessionRejectsStaleAuthorityEpoch(t *testing.T) {
	t.Parallel()

	session, err := NewSession(Config{}, testBootstrap(), authority.NewGuard(nil), nil)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	staleEpoch := int64(1)
	transportSeq := int64(1)
	record := eventabi.EventRecord{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeTurn,
		SessionID:          "sess-kernel-1",
		TurnID:             "turn-kernel-1",
		PipelineVersion:    "pipeline-v1",
		EventID:            "evt-kernel-ingress-stale",
		Lane:               eventabi.LaneData,
		TransportSequence:  &transportSeq,
		RuntimeSequence:    1,
		AuthorityEpoch:     &staleEpoch,
		RuntimeTimestampMS: 10,
		WallClockMS:        10,
	}
	_, err = session.NormalizeIngress(record, "pcm16")
	if !errors.Is(err, authority.ErrStaleAuthorityEpoch) {
		t.Fatalf("expected stale authority ingress error, got %v", err)
	}
}

func testBootstrap() apitransport.SessionBootstrap {
	return apitransport.SessionBootstrap{
		SchemaVersion:   "v1.0",
		TransportKind:   apitransport.TransportLiveKit,
		TenantID:        "tenant-kernel",
		SessionID:       "sess-kernel-1",
		TurnID:          "turn-kernel-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  2,
		LeaseTokenRef:   "lease-token-kernel",
		RouteRef:        "route-kernel",
		RequestedAtMS:   10,
	}
}
