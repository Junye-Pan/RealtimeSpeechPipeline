package admission

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

const defaultAdmissionPolicySnapshot = "admission-policy/v1"

const (
	// ReasonAllowed is emitted for default CP-05 allows.
	ReasonAllowed = "cp_admission_allowed"
	// ReasonDeferCapacity is emitted for deterministic CP-05 deferrals.
	ReasonDeferCapacity = "cp_admission_defer_capacity"
	// ReasonRejectPolicy is emitted for deterministic CP-05 rejections.
	ReasonRejectPolicy = "cp_admission_reject_policy"
	// ReasonInvalidInput is emitted when CP-05 input validation fails.
	ReasonInvalidInput = "cp_admission_invalid_input"
)

// Input models CP-05 admission evaluation context.
type Input struct {
	TenantID                 string
	SessionID                string
	TurnID                   string
	PipelineVersion          string
	PolicyResolutionSnapshot string
}

// Output is the deterministic CP-05 admission decision artifact.
type Output struct {
	AdmissionPolicySnapshot string
	OutcomeKind             controlplane.OutcomeKind
	Scope                   controlplane.OutcomeScope
	Reason                  string
}

// Backend evaluates admission decisions from a snapshot-fed control-plane source.
type Backend interface {
	Evaluate(in Input) (Output, error)
}

// Service evaluates deterministic CP-05 pre-turn decisions.
type Service struct {
	DefaultAdmissionPolicySnapshot string
	DefaultOutcomeKind             controlplane.OutcomeKind
	DefaultReason                  string
	Backend                        Backend
}

// NewService returns baseline CP-05 admission defaults.
func NewService() Service {
	return Service{
		DefaultAdmissionPolicySnapshot: defaultAdmissionPolicySnapshot,
		DefaultOutcomeKind:             controlplane.OutcomeAdmit,
		DefaultReason:                  ReasonAllowed,
	}
}

// Evaluate resolves deterministic CP-05 admission decision outputs.
func (s Service) Evaluate(in Input) (Output, error) {
	if in.SessionID == "" {
		return Output{}, fmt.Errorf("%s: session_id is required", ReasonInvalidInput)
	}

	if s.Backend != nil {
		out, err := s.Backend.Evaluate(in)
		if err != nil {
			return Output{}, fmt.Errorf("evaluate admission backend: %w", err)
		}
		return s.normalizeOutput(in, out), nil
	}

	return s.normalizeOutput(in, Output{}), nil
}

func (s Service) normalizeOutput(in Input, out Output) Output {
	snapshot := s.DefaultAdmissionPolicySnapshot
	if snapshot == "" {
		snapshot = defaultAdmissionPolicySnapshot
	}
	if out.AdmissionPolicySnapshot != "" {
		snapshot = out.AdmissionPolicySnapshot
	}

	outcome := out.OutcomeKind
	if outcome == "" {
		outcome = s.DefaultOutcomeKind
	}
	if outcome == "" {
		outcome = controlplane.OutcomeAdmit
	}
	if outcome != controlplane.OutcomeAdmit && outcome != controlplane.OutcomeReject && outcome != controlplane.OutcomeDefer {
		outcome = controlplane.OutcomeReject
	}

	defaultScope := controlplane.ScopeSession
	if in.TenantID != "" {
		defaultScope = controlplane.ScopeTenant
	}
	scope := out.Scope
	if scope == "" {
		scope = defaultScope
	}
	if scope != controlplane.ScopeTenant && scope != controlplane.ScopeSession {
		scope = defaultScope
	}

	reason := out.Reason
	if reason == "" {
		reason = defaultReasonForOutcome(outcome, s.DefaultReason)
	}

	return Output{
		AdmissionPolicySnapshot: snapshot,
		OutcomeKind:             outcome,
		Scope:                   scope,
		Reason:                  reason,
	}
}

func defaultReasonForOutcome(outcome controlplane.OutcomeKind, fallback string) string {
	switch outcome {
	case controlplane.OutcomeReject:
		return ReasonRejectPolicy
	case controlplane.OutcomeDefer:
		return ReasonDeferCapacity
	default:
		if fallback != "" {
			return fallback
		}
		return ReasonAllowed
	}
}
