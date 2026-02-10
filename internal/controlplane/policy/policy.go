package policy

import "fmt"

const defaultPolicyResolutionSnapshot = "policy-resolution/v1"

var allowedActionOrder = []string{"retry", "provider_switch", "degrade", "fallback"}

// Input models CP-04 policy evaluation context.
type Input struct {
	SessionID              string
	TurnID                 string
	PipelineVersion        string
	ProviderHealthSnapshot string
}

// Output is the policy evaluation artifact consumed by runtime turn-start.
type Output struct {
	PolicyResolutionSnapshot string
	AllowedAdaptiveActions   []string
}

// Backend evaluates policy from a snapshot-fed control-plane source.
type Backend interface {
	Evaluate(in Input) (Output, error)
}

// Service returns deterministic CP-04 policy outputs.
type Service struct {
	DefaultPolicyResolutionSnapshot string
	DefaultAllowedAdaptiveActions   []string
	Backend                         Backend
}

// NewService returns baseline CP-04 deterministic policy defaults.
func NewService() Service {
	return Service{
		DefaultPolicyResolutionSnapshot: defaultPolicyResolutionSnapshot,
		DefaultAllowedAdaptiveActions:   []string{"retry", "provider_switch", "fallback"},
	}
}

// Evaluate resolves policy snapshot and pre-authorized adaptive actions.
func (s Service) Evaluate(in Input) (Output, error) {
	if in.SessionID == "" {
		return Output{}, fmt.Errorf("session_id is required")
	}

	if s.Backend != nil {
		out, err := s.Backend.Evaluate(in)
		if err != nil {
			return Output{}, fmt.Errorf("evaluate policy backend: %w", err)
		}
		return s.normalizeOutput(out), nil
	}

	return s.normalizeOutput(Output{}), nil
}

func (s Service) normalizeOutput(out Output) Output {
	snapshot := s.DefaultPolicyResolutionSnapshot
	if snapshot == "" {
		snapshot = defaultPolicyResolutionSnapshot
	}
	if out.PolicyResolutionSnapshot != "" {
		snapshot = out.PolicyResolutionSnapshot
	}

	actions := s.DefaultAllowedAdaptiveActions
	if len(out.AllowedAdaptiveActions) > 0 {
		actions = out.AllowedAdaptiveActions
	}
	normalizedActions := normalizeAllowedAdaptiveActions(actions)
	return Output{
		PolicyResolutionSnapshot: snapshot,
		AllowedAdaptiveActions:   normalizedActions,
	}
}

func normalizeAllowedAdaptiveActions(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(in))
	for _, action := range in {
		seen[action] = struct{}{}
	}

	out := make([]string, 0, len(allowedActionOrder))
	for _, action := range allowedActionOrder {
		if _, ok := seen[action]; ok {
			out = append(out, action)
		}
	}
	return out
}
