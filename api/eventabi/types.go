package eventabi

import (
	"fmt"
	"regexp"
)

// Lane mirrors docs/ContractArtifacts.schema.json $defs.lane.
type Lane string

const (
	LaneData      Lane = "DataLane"
	LaneControl   Lane = "ControlLane"
	LaneTelemetry Lane = "TelemetryLane"
)

// EventScope mirrors docs/ContractArtifacts.schema.json $defs.event_scope.
type EventScope string

const (
	ScopeSession EventScope = "session"
	ScopeTurn    EventScope = "turn"
)

// PayloadClass mirrors docs/SecurityDataHandlingBaseline.md taxonomy.
type PayloadClass string

const (
	PayloadPII            PayloadClass = "PII"
	PayloadPHI            PayloadClass = "PHI"
	PayloadAudioRaw       PayloadClass = "audio_raw"
	PayloadTextRaw        PayloadClass = "text_raw"
	PayloadDerivedSummary PayloadClass = "derived_summary"
	PayloadMetadata       PayloadClass = "metadata"
)

// MediaTime captures event media-time coordinates for audio payloads.
type MediaTime struct {
	SampleIndex *int64 `json:"sample_index,omitempty"`
	PTSMS       *int64 `json:"pts_ms,omitempty"`
}

func (m MediaTime) Validate() error {
	if m.SampleIndex == nil && m.PTSMS == nil {
		return fmt.Errorf("media_time requires sample_index or pts_ms")
	}
	if m.SampleIndex != nil && *m.SampleIndex < 0 {
		return fmt.Errorf("media_time.sample_index must be >=0")
	}
	if m.PTSMS != nil && *m.PTSMS < 0 {
		return fmt.Errorf("media_time.pts_ms must be >=0")
	}
	return nil
}

// EventRecord mirrors the event_record artifact shape.
type EventRecord struct {
	SchemaVersion      string       `json:"schema_version"`
	EventScope         EventScope   `json:"event_scope"`
	SessionID          string       `json:"session_id"`
	TurnID             string       `json:"turn_id,omitempty"`
	PipelineVersion    string       `json:"pipeline_version"`
	EventID            string       `json:"event_id"`
	Lane               Lane         `json:"lane"`
	TransportSequence  *int64       `json:"transport_sequence"`
	RuntimeSequence    int64        `json:"runtime_sequence"`
	AuthorityEpoch     *int64       `json:"authority_epoch,omitempty"`
	RuntimeTimestampMS int64        `json:"runtime_timestamp_ms"`
	WallClockMS        int64        `json:"wall_clock_timestamp_ms"`
	PayloadClass       PayloadClass `json:"payload_class"`
	MediaTime          *MediaTime   `json:"media_time,omitempty"`
}

// ControlSignal mirrors the control_signal artifact shape.
type ControlSignal struct {
	SchemaVersion      string       `json:"schema_version"`
	EventScope         EventScope   `json:"event_scope"`
	SessionID          string       `json:"session_id"`
	TurnID             string       `json:"turn_id,omitempty"`
	PipelineVersion    string       `json:"pipeline_version"`
	EventID            string       `json:"event_id"`
	Lane               Lane         `json:"lane"`
	TransportSequence  *int64       `json:"transport_sequence"`
	RuntimeSequence    int64        `json:"runtime_sequence"`
	AuthorityEpoch     int64        `json:"authority_epoch"`
	RuntimeTimestampMS int64        `json:"runtime_timestamp_ms"`
	WallClockMS        int64        `json:"wall_clock_timestamp_ms"`
	PayloadClass       PayloadClass `json:"payload_class"`
	Signal             string       `json:"signal"`
	EmittedBy          string       `json:"emitted_by"`
	Reason             string       `json:"reason,omitempty"`
	Scope              string       `json:"scope,omitempty"`
}

var schemaVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)

func (e EventRecord) Validate() error {
	if !schemaVersionRE.MatchString(e.SchemaVersion) {
		return fmt.Errorf("invalid schema_version: %q", e.SchemaVersion)
	}
	if e.EventScope != ScopeSession && e.EventScope != ScopeTurn {
		return fmt.Errorf("invalid event_scope: %q", e.EventScope)
	}
	if e.EventScope == ScopeTurn {
		if e.TurnID == "" {
			return fmt.Errorf("turn_id is required when event_scope=turn")
		}
		if e.AuthorityEpoch == nil {
			return fmt.Errorf("authority_epoch is required when event_scope=turn")
		}
	}
	if e.SessionID == "" || e.PipelineVersion == "" || e.EventID == "" {
		return fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	if !isLane(e.Lane) {
		return fmt.Errorf("invalid lane: %q", e.Lane)
	}
	if e.Lane == LaneControl {
		return fmt.Errorf("event_record cannot be ControlLane")
	}
	if e.TransportSequence == nil || *e.TransportSequence < 0 {
		return fmt.Errorf("transport_sequence is required and must be >=0")
	}
	if e.RuntimeSequence < 0 {
		return fmt.Errorf("runtime_sequence must be >=0")
	}
	if e.RuntimeTimestampMS < 0 || e.WallClockMS < 0 {
		return fmt.Errorf("timestamps must be >= 0")
	}
	if !isPayloadClass(e.PayloadClass) {
		return fmt.Errorf("invalid payload_class: %q", e.PayloadClass)
	}
	if e.PayloadClass == PayloadAudioRaw {
		if e.MediaTime == nil {
			return fmt.Errorf("media_time is required for payload_class=audio_raw")
		}
		if err := e.MediaTime.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c ControlSignal) Validate() error {
	if !schemaVersionRE.MatchString(c.SchemaVersion) {
		return fmt.Errorf("invalid schema_version: %q", c.SchemaVersion)
	}
	if c.EventScope != ScopeSession && c.EventScope != ScopeTurn {
		return fmt.Errorf("invalid event_scope: %q", c.EventScope)
	}
	if c.EventScope == ScopeTurn && c.TurnID == "" {
		return fmt.Errorf("turn_id is required when event_scope=turn")
	}
	if c.SessionID == "" || c.PipelineVersion == "" || c.EventID == "" {
		return fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	if c.Lane != LaneControl {
		return fmt.Errorf("control_signal lane must be ControlLane")
	}
	if c.PayloadClass != PayloadMetadata {
		return fmt.Errorf("control_signal payload_class must be metadata")
	}
	if c.TransportSequence == nil || *c.TransportSequence < 0 {
		return fmt.Errorf("transport_sequence is required and must be >=0")
	}
	if c.RuntimeSequence < 0 || c.RuntimeTimestampMS < 0 || c.WallClockMS < 0 {
		return fmt.Errorf("runtime sequence and timestamps must be >=0")
	}
	if c.Signal == "" {
		return fmt.Errorf("signal is required")
	}
	if !isControlSignalName(c.Signal) {
		return fmt.Errorf("invalid signal: %q", c.Signal)
	}
	if !isControlSignalEmitter(c.EmittedBy) {
		return fmt.Errorf("invalid emitted_by: %q", c.EmittedBy)
	}

	if c.Signal == "abort" && c.Reason == "" {
		return fmt.Errorf("abort requires reason")
	}
	if c.Signal == "cancel" && c.Scope == "" {
		return fmt.Errorf("cancel requires scope")
	}

	if inStringSet(c.Signal, []string{"turn_open", "commit", "abort", "close"}) {
		if c.EventScope != ScopeTurn || c.EmittedBy != "RK-03" || c.TurnID == "" {
			return fmt.Errorf("%s requires turn scope, emitted_by=RK-03, and turn_id", c.Signal)
		}
	}

	if inStringSet(c.Signal, []string{"admit", "reject", "defer"}) {
		if c.EventScope != ScopeSession || !inStringSet(c.EmittedBy, []string{"RK-25", "CP-05"}) || c.Reason == "" {
			return fmt.Errorf("%s requires session scope, emitted_by RK-25|CP-05, and reason", c.Signal)
		}
	}

	if c.Signal == "shed" {
		if c.EmittedBy != "RK-25" || c.Reason == "" {
			return fmt.Errorf("shed requires emitted_by=RK-25 and reason")
		}
	}

	if inStringSet(c.Signal, []string{"stale_epoch_reject", "deauthorized_drain"}) {
		if c.EmittedBy != "RK-24" || c.Reason == "" {
			return fmt.Errorf("%s requires emitted_by=RK-24 and reason", c.Signal)
		}
	}

	return nil
}

func isLane(l Lane) bool {
	switch l {
	case LaneData, LaneControl, LaneTelemetry:
		return true
	default:
		return false
	}
}

func isPayloadClass(c PayloadClass) bool {
	switch c {
	case PayloadPII, PayloadPHI, PayloadAudioRaw, PayloadTextRaw, PayloadDerivedSummary, PayloadMetadata:
		return true
	default:
		return false
	}
}

func isControlSignalEmitter(v string) bool {
	switch v {
	case "RK-02", "RK-03", "RK-06", "RK-11", "RK-12", "RK-13", "RK-14", "RK-15", "RK-16", "RK-17", "RK-22", "RK-23", "RK-24", "RK-25", "OR-02", "CP-05", "CP-07", "CP-08":
		return true
	default:
		return false
	}
}

func isControlSignalName(v string) bool {
	switch v {
	case "turn_open_proposed", "turn_open", "commit", "abort", "close", "barge_in", "stop", "cancel", "watermark", "budget_warning", "budget_exhausted", "degrade", "fallback", "discontinuity", "drop_notice", "flow_xoff", "flow_xon", "credit_grant", "provider_error", "circuit_event", "provider_switch", "lease_issued", "lease_rotated", "migration_start", "migration_finish", "session_handoff", "admit", "reject", "defer", "shed", "stale_epoch_reject", "deauthorized_drain", "connected", "reconnecting", "disconnected", "ended", "silence", "stall", "output_accepted", "playback_started", "playback_completed", "playback_cancelled", "recording_level_downgraded":
		return true
	default:
		return false
	}
}

func inStringSet(v string, set []string) bool {
	for _, candidate := range set {
		if v == candidate {
			return true
		}
	}
	return false
}
