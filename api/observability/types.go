package observability

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// DivergenceClass is a minimal shared type for replay/reporting artifacts.
type DivergenceClass string

const (
	PlanDivergence           DivergenceClass = "PLAN_DIVERGENCE"
	OutcomeDivergence        DivergenceClass = "OUTCOME_DIVERGENCE"
	OrderingDivergence       DivergenceClass = "ORDERING_DIVERGENCE"
	TimingDivergence         DivergenceClass = "TIMING_DIVERGENCE"
	AuthorityDivergence      DivergenceClass = "AUTHORITY_DIVERGENCE"
	ProviderChoiceDivergence DivergenceClass = "PROVIDER_CHOICE_DIVERGENCE"
)

// ReplayDivergence captures a classified replay mismatch entry.
type ReplayDivergence struct {
	Class    DivergenceClass `json:"class"`
	Scope    string          `json:"scope"`
	Message  string          `json:"message"`
	DiffMS   *int64          `json:"diff_ms,omitempty"`
	Expected bool            `json:"expected,omitempty"`
}

// Validate enforces divergence schema constraints.
func (d ReplayDivergence) Validate() error {
	if !isDivergenceClass(d.Class) {
		return fmt.Errorf("invalid divergence class: %q", d.Class)
	}
	if d.Scope == "" || d.Message == "" {
		return fmt.Errorf("scope and message are required")
	}
	if d.Class == TimingDivergence && d.DiffMS == nil {
		return fmt.Errorf("diff_ms is required for timing divergence")
	}
	if d.DiffMS != nil && *d.DiffMS < 0 {
		return fmt.Errorf("diff_ms must be >=0")
	}
	return nil
}

// ReplayFidelity captures requested replay fidelity levels.
type ReplayFidelity string

const (
	ReplayFidelityL0 ReplayFidelity = "L0"
	ReplayFidelityL1 ReplayFidelity = "L1"
	ReplayFidelityL2 ReplayFidelity = "L2"
)

// ReplayMode captures explicit replay execution strategy.
type ReplayMode string

const (
	ReplayModeReSimulateNodes          ReplayMode = "re_simulate_nodes"
	ReplayModePlaybackRecordedProvider ReplayMode = "playback_recorded_provider_outputs"
	ReplayModeReplayDecisions          ReplayMode = "replay_decisions"
	ReplayModeRecomputeDecisions       ReplayMode = "recompute_decisions"
)

// Validate enforces replay mode enum requirements.
func (m ReplayMode) Validate() error {
	if !isReplayMode(m) {
		return fmt.Errorf("invalid replay mode: %q", m)
	}
	return nil
}

// IsReplayMode validates an untyped replay mode string.
func IsReplayMode(v string) bool {
	return isReplayMode(ReplayMode(v))
}

// ReplayCursor captures deterministic replay resume boundary.
type ReplayCursor struct {
	SessionID       string        `json:"session_id"`
	TurnID          string        `json:"turn_id"`
	Lane            eventabi.Lane `json:"lane"`
	RuntimeSequence int64         `json:"runtime_sequence"`
	EventID         string        `json:"event_id"`
}

// Validate enforces replay cursor portability constraints.
func (c ReplayCursor) Validate() error {
	if c.SessionID == "" || c.TurnID == "" || c.EventID == "" {
		return fmt.Errorf("session_id, turn_id, and event_id are required")
	}
	if !isReplayLane(c.Lane) {
		return fmt.Errorf("invalid lane: %q", c.Lane)
	}
	if c.RuntimeSequence < 0 {
		return fmt.Errorf("runtime_sequence must be >=0")
	}
	return nil
}

// ReplayRunRequest defines replay invocation input for tooling/runtime boundaries.
type ReplayRunRequest struct {
	BaselineRef      string        `json:"baseline_ref"`
	CandidatePlanRef string        `json:"candidate_plan_ref"`
	Mode             ReplayMode    `json:"mode"`
	Cursor           *ReplayCursor `json:"cursor,omitempty"`
}

// Validate enforces replay-run request requirements.
func (r ReplayRunRequest) Validate() error {
	if r.BaselineRef == "" || r.CandidatePlanRef == "" {
		return fmt.Errorf("baseline_ref and candidate_plan_ref are required")
	}
	if err := r.Mode.Validate(); err != nil {
		return err
	}
	if r.Cursor != nil {
		if err := r.Cursor.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ReplayRunResult captures replay outputs for deterministic reporting.
type ReplayRunResult struct {
	RunID       string             `json:"run_id"`
	Mode        ReplayMode         `json:"mode"`
	Divergences []ReplayDivergence `json:"divergences,omitempty"`
	Cursor      *ReplayCursor      `json:"cursor,omitempty"`
}

// Validate enforces replay-run result schema requirements.
func (r ReplayRunResult) Validate() error {
	if r.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	if err := r.Mode.Validate(); err != nil {
		return err
	}
	for _, divergence := range r.Divergences {
		if err := divergence.Validate(); err != nil {
			return err
		}
	}
	if r.Cursor != nil {
		if err := r.Cursor.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ReplayRunReport captures machine-readable replay diff output.
type ReplayRunReport struct {
	Request       ReplayRunRequest `json:"request"`
	Result        ReplayRunResult  `json:"result"`
	GeneratedAtMS int64            `json:"generated_at_ms"`
}

// Validate enforces replay report consistency.
func (r ReplayRunReport) Validate() error {
	if r.GeneratedAtMS < 0 {
		return fmt.Errorf("generated_at_ms must be >=0")
	}
	if err := r.Request.Validate(); err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	if err := r.Result.Validate(); err != nil {
		return fmt.Errorf("invalid result: %w", err)
	}
	if r.Request.Mode != r.Result.Mode {
		return fmt.Errorf("request mode and result mode must match")
	}
	return nil
}

// ReplayAccessRequest captures minimum replay authorization attributes.
type ReplayAccessRequest struct {
	TenantID          string                  `json:"tenant_id"`
	PrincipalID       string                  `json:"principal_id"`
	Role              string                  `json:"role"`
	Purpose           string                  `json:"purpose"`
	RequestedScope    string                  `json:"requested_scope"`
	RequestedFidelity ReplayFidelity          `json:"requested_fidelity"`
	RequestedClasses  []eventabi.PayloadClass `json:"requested_classes,omitempty"`
}

// Validate enforces deny-by-default replay request requirements.
func (r ReplayAccessRequest) Validate() error {
	if r.TenantID == "" || r.PrincipalID == "" || r.Role == "" || r.Purpose == "" || r.RequestedScope == "" {
		return fmt.Errorf("tenant_id, principal_id, role, purpose, and requested_scope are required")
	}
	if !isReplayFidelity(r.RequestedFidelity) {
		return fmt.Errorf("invalid requested_fidelity: %q", r.RequestedFidelity)
	}
	for _, class := range r.RequestedClasses {
		if err := (eventabi.RedactionDecision{PayloadClass: class, Action: eventabi.RedactionAllow}).Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ReplayRedactionMarker indicates response-time redaction action per class.
type ReplayRedactionMarker struct {
	PayloadClass eventabi.PayloadClass    `json:"payload_class"`
	Action       eventabi.RedactionAction `json:"action"`
	Reason       string                   `json:"reason"`
}

// ReplayAccessDecision captures allow/deny decision and response redaction markers.
type ReplayAccessDecision struct {
	Allowed          bool                    `json:"allowed"`
	DenyReason       string                  `json:"deny_reason,omitempty"`
	RedactionMarkers []ReplayRedactionMarker `json:"redaction_markers,omitempty"`
}

// Validate enforces replay access decision requirements.
func (d ReplayAccessDecision) Validate() error {
	if !d.Allowed && d.DenyReason == "" {
		return fmt.Errorf("deny_reason is required when replay decision is denied")
	}
	if d.Allowed && d.DenyReason != "" {
		return fmt.Errorf("deny_reason must be empty when replay decision is allowed")
	}
	for _, marker := range d.RedactionMarkers {
		if err := validateReplayRedactionMarker(marker); err != nil {
			return err
		}
	}
	return nil
}

// ReplayAuditEvent is an immutable audit schema for OR-03 replay access.
type ReplayAuditEvent struct {
	TimestampMS int64                `json:"timestamp_ms"`
	Request     ReplayAccessRequest  `json:"request"`
	Decision    ReplayAccessDecision `json:"decision"`
}

// Validate enforces minimal replay audit event schema requirements.
func (e ReplayAuditEvent) Validate() error {
	if e.TimestampMS < 0 {
		return fmt.Errorf("timestamp_ms must be >=0")
	}
	if err := e.Request.Validate(); err != nil {
		return err
	}
	if err := e.Decision.Validate(); err != nil {
		return err
	}
	return nil
}

func isReplayFidelity(v ReplayFidelity) bool {
	switch v {
	case ReplayFidelityL0, ReplayFidelityL1, ReplayFidelityL2:
		return true
	default:
		return false
	}
}

func isReplayMode(v ReplayMode) bool {
	switch v {
	case ReplayModeReSimulateNodes, ReplayModePlaybackRecordedProvider, ReplayModeReplayDecisions, ReplayModeRecomputeDecisions:
		return true
	default:
		return false
	}
}

func isDivergenceClass(v DivergenceClass) bool {
	switch v {
	case PlanDivergence, OutcomeDivergence, OrderingDivergence, TimingDivergence, AuthorityDivergence, ProviderChoiceDivergence:
		return true
	default:
		return false
	}
}

func isReplayLane(v eventabi.Lane) bool {
	switch v {
	case eventabi.LaneData, eventabi.LaneControl, eventabi.LaneTelemetry:
		return true
	default:
		return false
	}
}

func validateReplayRedactionMarker(marker ReplayRedactionMarker) error {
	if err := (eventabi.RedactionDecision{PayloadClass: marker.PayloadClass, Action: marker.Action}).Validate(); err != nil {
		return err
	}
	if marker.Reason == "" {
		return fmt.Errorf("redaction marker reason is required")
	}
	return nil
}
