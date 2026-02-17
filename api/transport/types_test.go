package transport

import "testing"

func TestSessionBootstrapValidate(t *testing.T) {
	t.Parallel()

	valid := SessionBootstrap{
		SchemaVersion:   "v1.0",
		TransportKind:   TransportLiveKit,
		TenantID:        "tenant-a",
		SessionID:       "sess-1",
		TurnID:          "turn-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  7,
		LeaseTokenRef:   "lease-token-1",
		RouteRef:        "route-ref-1",
		RequestedAtMS:   100,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid session bootstrap, got %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*SessionBootstrap)
	}{
		{name: "invalid schema", mutate: func(v *SessionBootstrap) { v.SchemaVersion = "1.0" }},
		{name: "invalid transport kind", mutate: func(v *SessionBootstrap) { v.TransportKind = "unknown" }},
		{name: "missing tenant", mutate: func(v *SessionBootstrap) { v.TenantID = "" }},
		{name: "negative authority epoch", mutate: func(v *SessionBootstrap) { v.AuthorityEpoch = -1 }},
		{name: "missing lease token", mutate: func(v *SessionBootstrap) { v.LeaseTokenRef = "" }},
		{name: "negative requested_at_ms", mutate: func(v *SessionBootstrap) { v.RequestedAtMS = -1 }},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestAdapterCapabilitiesValidate(t *testing.T) {
	t.Parallel()

	valid := AdapterCapabilities{
		TransportKind:              TransportLiveKit,
		SupportedConnectionSignals: []ConnectionSignal{SignalConnected, SignalDisconnected},
		SupportsIngressAudio:       true,
		SupportsIngressControl:     true,
		SupportsEgressAudio:        true,
		SupportsReconnectSemantics: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid adapter capabilities, got %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*AdapterCapabilities)
	}{
		{name: "invalid transport kind", mutate: func(v *AdapterCapabilities) { v.TransportKind = "invalid" }},
		{name: "missing signals", mutate: func(v *AdapterCapabilities) { v.SupportedConnectionSignals = nil }},
		{name: "invalid signal", mutate: func(v *AdapterCapabilities) { v.SupportedConnectionSignals = []ConnectionSignal{"bad"} }},
		{name: "duplicate signal", mutate: func(v *AdapterCapabilities) {
			v.SupportedConnectionSignals = []ConnectionSignal{SignalConnected, SignalConnected}
		}},
		{name: "no io capability", mutate: func(v *AdapterCapabilities) {
			v.SupportsIngressAudio = false
			v.SupportsIngressControl = false
			v.SupportsEgressAudio = false
		}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}
