package telephony

import (
	"context"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/authority"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport/routing"
)

func TestCapabilitiesValidate(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(Config{}, authority.NewGuard(nil), nil)
	caps := adapter.Capabilities()
	if err := caps.Validate(); err != nil {
		t.Fatalf("capabilities validation failed: %v", err)
	}
}

func TestOpenAndIngressEgressFlow(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(Config{DefaultDataClass: eventabi.PayloadAudioRaw}, authority.NewGuard(nil), nil)
	if err := adapter.Open(context.Background(), testTelephonyBootstrap()); err != nil {
		t.Fatalf("open telephony session: %v", err)
	}

	transportSeq := int64(1)
	authorityEpoch := int64(1)
	normalized, err := adapter.HandleIngress(eventabi.EventRecord{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeTurn,
		SessionID:          "sess-tel-1",
		TurnID:             "turn-tel-1",
		PipelineVersion:    "pipeline-v1",
		EventID:            "evt-tel-ingress-1",
		Lane:               eventabi.LaneData,
		TransportSequence:  &transportSeq,
		RuntimeSequence:    1,
		AuthorityEpoch:     &authorityEpoch,
		RuntimeTimestampMS: 10,
		WallClockMS:        10,
	}, "pcmu")
	if err != nil {
		t.Fatalf("handle ingress: %v", err)
	}
	if normalized.PayloadClass != eventabi.PayloadAudioRaw {
		t.Fatalf("expected payload class audio_raw, got %s", normalized.PayloadClass)
	}

	decision, err := adapter.HandleEgress(runtimetransport.OutputAttempt{
		SessionID:            "sess-tel-1",
		TurnID:               "turn-tel-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-tel-egress-1",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   11,
		WallClockTimestampMS: 11,
	}, 64)
	if err != nil {
		t.Fatalf("handle egress: %v", err)
	}
	if !decision.Accepted {
		t.Fatalf("expected accepted egress decision, got %+v", decision)
	}

	changed, err := adapter.OnRoutingUpdate(routing.View{Version: 1, ActiveEndpoint: "runtime-tel-b", LeaseEpoch: 2})
	if err != nil {
		t.Fatalf("routing update: %v", err)
	}
	if !changed {
		t.Fatalf("expected routing update to report change")
	}
}

func TestOpenRejectsWrongTransportKind(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(Config{}, authority.NewGuard(nil), nil)
	err := adapter.Open(context.Background(), apitransport.SessionBootstrap{
		SchemaVersion:   "v1.0",
		TransportKind:   apitransport.TransportWebSocket,
		TenantID:        "tenant-tel",
		SessionID:       "sess-tel-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  1,
		LeaseTokenRef:   "lease-token",
		RouteRef:        "route-ref",
		RequestedAtMS:   1,
	})
	if err == nil {
		t.Fatalf("expected open with wrong transport kind to fail")
	}
}

func testTelephonyBootstrap() apitransport.SessionBootstrap {
	return apitransport.SessionBootstrap{
		SchemaVersion:   "v1.0",
		TransportKind:   apitransport.TransportTelephony,
		TenantID:        "tenant-tel",
		SessionID:       "sess-tel-1",
		TurnID:          "turn-tel-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  1,
		LeaseTokenRef:   "lease-token",
		RouteRef:        "route-ref",
		RequestedAtMS:   1,
	}
}
