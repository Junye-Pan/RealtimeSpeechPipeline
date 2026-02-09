package observability

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// DivergenceClass is a minimal shared type for replay/reporting artifacts.
type DivergenceClass string

const (
	PlanDivergence      DivergenceClass = "PLAN_DIVERGENCE"
	OutcomeDivergence   DivergenceClass = "OUTCOME_DIVERGENCE"
	OrderingDivergence  DivergenceClass = "ORDERING_DIVERGENCE"
	TimingDivergence    DivergenceClass = "TIMING_DIVERGENCE"
	AuthorityDivergence DivergenceClass = "AUTHORITY_DIVERGENCE"
)

// ReplayDivergence captures a classified replay mismatch entry.
type ReplayDivergence struct {
	Class    DivergenceClass `json:"class"`
	Scope    string          `json:"scope"`
	Message  string          `json:"message"`
	DiffMS   *int64          `json:"diff_ms,omitempty"`
	Expected bool            `json:"expected,omitempty"`
}

// ReplayFidelity captures requested replay fidelity levels.
type ReplayFidelity string

const (
	ReplayFidelityL0 ReplayFidelity = "L0"
	ReplayFidelityL1 ReplayFidelity = "L1"
	ReplayFidelityL2 ReplayFidelity = "L2"
)

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
	for _, marker := range e.Decision.RedactionMarkers {
		if err := (eventabi.RedactionDecision{PayloadClass: marker.PayloadClass, Action: marker.Action}).Validate(); err != nil {
			return err
		}
		if marker.Reason == "" {
			return fmt.Errorf("redaction marker reason is required")
		}
	}
	if !e.Decision.Allowed && e.Decision.DenyReason == "" {
		return fmt.Errorf("deny_reason is required when replay decision is denied")
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
