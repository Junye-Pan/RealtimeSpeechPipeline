package transport

import (
	"fmt"
	"regexp"
)

var schemaVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)

// TransportKind identifies adapter-level transport modality.
type TransportKind string

const (
	TransportLiveKit   TransportKind = "livekit"
	TransportWebSocket TransportKind = "websocket"
	TransportTelephony TransportKind = "telephony"
)

// ConnectionSignal is the normalized transport lifecycle signal set.
type ConnectionSignal string

const (
	SignalConnected    ConnectionSignal = "connected"
	SignalReconnecting ConnectionSignal = "reconnecting"
	SignalDisconnected ConnectionSignal = "disconnected"
	SignalEnded        ConnectionSignal = "ended"
	SignalSilence      ConnectionSignal = "silence"
	SignalStall        ConnectionSignal = "stall"
)

// SessionBootstrap is the transport-neutral control-plane bootstrap contract.
type SessionBootstrap struct {
	SchemaVersion   string        `json:"schema_version"`
	TransportKind   TransportKind `json:"transport_kind"`
	TenantID        string        `json:"tenant_id"`
	SessionID       string        `json:"session_id"`
	TurnID          string        `json:"turn_id,omitempty"`
	PipelineVersion string        `json:"pipeline_version"`
	AuthorityEpoch  int64         `json:"authority_epoch"`
	LeaseTokenRef   string        `json:"lease_token_ref"`
	RouteRef        string        `json:"route_ref"`
	RequestedAtMS   int64         `json:"requested_at_ms"`
}

// Validate enforces bootstrap invariants shared across adapters.
func (s SessionBootstrap) Validate() error {
	if !schemaVersionRE.MatchString(s.SchemaVersion) {
		return fmt.Errorf("invalid schema_version: %q", s.SchemaVersion)
	}
	if !isTransportKind(s.TransportKind) {
		return fmt.Errorf("invalid transport_kind: %q", s.TransportKind)
	}
	if s.TenantID == "" || s.SessionID == "" || s.PipelineVersion == "" {
		return fmt.Errorf("tenant_id, session_id, and pipeline_version are required")
	}
	if s.AuthorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >=0")
	}
	if s.LeaseTokenRef == "" || s.RouteRef == "" {
		return fmt.Errorf("lease_token_ref and route_ref are required")
	}
	if s.RequestedAtMS < 0 {
		return fmt.Errorf("requested_at_ms must be >=0")
	}
	return nil
}

// AdapterCapabilities declares normalized transport capability shape.
type AdapterCapabilities struct {
	TransportKind              TransportKind      `json:"transport_kind"`
	SupportedConnectionSignals []ConnectionSignal `json:"supported_connection_signals"`
	SupportsIngressAudio       bool               `json:"supports_ingress_audio"`
	SupportsIngressControl     bool               `json:"supports_ingress_control"`
	SupportsEgressAudio        bool               `json:"supports_egress_audio"`
	SupportsReconnectSemantics bool               `json:"supports_reconnect_semantics"`
}

// Validate enforces capability contract invariants.
func (c AdapterCapabilities) Validate() error {
	if !isTransportKind(c.TransportKind) {
		return fmt.Errorf("invalid transport_kind: %q", c.TransportKind)
	}
	if len(c.SupportedConnectionSignals) < 1 {
		return fmt.Errorf("supported_connection_signals requires at least one signal")
	}
	seen := map[ConnectionSignal]struct{}{}
	for _, signal := range c.SupportedConnectionSignals {
		if !isConnectionSignal(signal) {
			return fmt.Errorf("invalid connection signal: %q", signal)
		}
		if _, ok := seen[signal]; ok {
			return fmt.Errorf("duplicate connection signal: %q", signal)
		}
		seen[signal] = struct{}{}
	}
	if !c.SupportsIngressAudio && !c.SupportsIngressControl && !c.SupportsEgressAudio {
		return fmt.Errorf("at least one ingress/egress capability must be true")
	}
	return nil
}

func isTransportKind(v TransportKind) bool {
	switch v {
	case TransportLiveKit, TransportWebSocket, TransportTelephony:
		return true
	default:
		return false
	}
}

func isConnectionSignal(v ConnectionSignal) bool {
	switch v {
	case SignalConnected, SignalReconnecting, SignalDisconnected, SignalEnded, SignalSilence, SignalStall:
		return true
	default:
		return false
	}
}
