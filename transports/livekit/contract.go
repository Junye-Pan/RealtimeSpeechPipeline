package livekit

import (
	"fmt"

	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
)

// Kind returns the shared transport kind identifier.
func (a Adapter) Kind() apitransport.TransportKind {
	return apitransport.TransportLiveKit
}

// Capabilities returns shared contract capabilities for the LiveKit adapter.
func (a Adapter) Capabilities() apitransport.AdapterCapabilities {
	return apitransport.AdapterCapabilities{
		TransportKind: apitransport.TransportLiveKit,
		SupportedConnectionSignals: []apitransport.ConnectionSignal{
			apitransport.SignalConnected,
			apitransport.SignalReconnecting,
			apitransport.SignalDisconnected,
			apitransport.SignalEnded,
			apitransport.SignalSilence,
			apitransport.SignalStall,
		},
		SupportsIngressAudio:       true,
		SupportsIngressControl:     true,
		SupportsEgressAudio:        true,
		SupportsReconnectSemantics: true,
	}
}

// ValidateBootstrap validates shared control-plane bootstrap for LiveKit sessions.
func ValidateBootstrap(bootstrap apitransport.SessionBootstrap) error {
	if bootstrap.TransportKind != apitransport.TransportLiveKit {
		return fmt.Errorf("livekit bootstrap requires transport_kind=livekit")
	}
	return bootstrap.Validate()
}
