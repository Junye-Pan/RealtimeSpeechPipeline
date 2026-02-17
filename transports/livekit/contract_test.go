package livekit

import (
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
)

func TestCapabilitiesValidate(t *testing.T) {
	t.Parallel()

	adapter, err := NewAdapter(Config{
		SessionID:        "sess-livekit-contract",
		TurnID:           "turn-livekit-contract",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		RequestTimeout:   2 * time.Second,
	}, Dependencies{})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	caps := adapter.Capabilities()
	if err := caps.Validate(); err != nil {
		t.Fatalf("capabilities validation failed: %v", err)
	}
}

func TestValidateBootstrap(t *testing.T) {
	t.Parallel()

	good := apitransport.SessionBootstrap{
		SchemaVersion:   "v1.0",
		TransportKind:   apitransport.TransportLiveKit,
		TenantID:        "tenant-livekit",
		SessionID:       "sess-livekit",
		TurnID:          "turn-livekit",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  1,
		LeaseTokenRef:   "lease-token",
		RouteRef:        "route-ref",
		RequestedAtMS:   1,
	}
	if err := ValidateBootstrap(good); err != nil {
		t.Fatalf("expected bootstrap validation success, got %v", err)
	}

	bad := good
	bad.TransportKind = apitransport.TransportWebSocket
	if err := ValidateBootstrap(bad); err == nil {
		t.Fatalf("expected wrong transport kind bootstrap to fail")
	}
}
