package replay

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	securitypolicy "github.com/tiger/realtime-speech-pipeline/internal/security/policy"
)

// AccessPolicy defines tenant-scoped replay access policy constraints.
type AccessPolicy struct {
	TenantID          string
	AllowedRoles      []string
	AllowedPurposes   []string
	AllowedFidelities []obs.ReplayFidelity
}

// DefaultAccessPolicy returns a conservative baseline replay policy.
func DefaultAccessPolicy(tenantID string) AccessPolicy {
	return AccessPolicy{
		TenantID:          tenantID,
		AllowedRoles:      []string{"oncall", "operator", "compliance"},
		AllowedPurposes:   []string{"debug", "incident", "eval", "compliance"},
		AllowedFidelities: []obs.ReplayFidelity{obs.ReplayFidelityL0},
	}
}

// AuthorizeReplayAccess enforces deny-by-default replay authorization checks.
func AuthorizeReplayAccess(req obs.ReplayAccessRequest, policy AccessPolicy) obs.ReplayAccessDecision {
	if err := req.Validate(); err != nil {
		return deny("invalid_request_attributes")
	}
	if policy.TenantID == "" {
		return deny("policy_tenant_scope_missing")
	}
	if req.TenantID != policy.TenantID {
		return deny("cross_tenant_denied")
	}
	if !inStringSet(req.Role, policy.AllowedRoles) {
		return deny("role_not_authorized")
	}
	if !inStringSet(req.Purpose, policy.AllowedPurposes) {
		return deny("purpose_not_authorized")
	}
	if !inFidelitySet(req.RequestedFidelity, policy.AllowedFidelities) {
		return deny("requested_fidelity_not_authorized")
	}

	level, err := securitypolicy.ParseRecordingLevel(string(req.RequestedFidelity))
	if err != nil {
		return deny("requested_fidelity_not_authorized")
	}
	classes := req.RequestedClasses
	if len(classes) == 0 {
		classes = []eventabi.PayloadClass{eventabi.PayloadMetadata}
	}

	decisions, err := securitypolicy.BuildDefaultRedactionDecisions(securitypolicy.SurfaceOR03, level, classes)
	if err != nil {
		return deny("invalid_requested_classes")
	}
	markers := make([]obs.ReplayRedactionMarker, 0, len(decisions))
	for _, decision := range decisions {
		markers = append(markers, obs.ReplayRedactionMarker{
			PayloadClass: decision.PayloadClass,
			Action:       decision.Action,
			Reason:       "default_or03_policy",
		})
	}
	return obs.ReplayAccessDecision{Allowed: true, RedactionMarkers: markers}
}

// BuildReplayAuditEvent builds and validates immutable replay audit events.
func BuildReplayAuditEvent(timestampMS int64, req obs.ReplayAccessRequest, decision obs.ReplayAccessDecision) (obs.ReplayAuditEvent, error) {
	event := obs.ReplayAuditEvent{
		TimestampMS: timestampMS,
		Request:     req,
		Decision:    decision,
	}
	if err := event.Validate(); err != nil {
		return obs.ReplayAuditEvent{}, fmt.Errorf("invalid replay audit event: %w", err)
	}
	return event, nil
}

func deny(reason string) obs.ReplayAccessDecision {
	return obs.ReplayAccessDecision{Allowed: false, DenyReason: reason}
}

func inStringSet(value string, set []string) bool {
	for _, candidate := range set {
		if value == candidate {
			return true
		}
	}
	return false
}

func inFidelitySet(value obs.ReplayFidelity, set []obs.ReplayFidelity) bool {
	for _, candidate := range set {
		if value == candidate {
			return true
		}
	}
	return false
}
