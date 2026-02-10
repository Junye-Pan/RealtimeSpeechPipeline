package providerhealth

import "fmt"

const defaultProviderHealthSnapshot = "provider-health/v1"

// Input selects provider health snapshot scope.
type Input struct {
	Scope           string
	PipelineVersion string
}

// Output returns deterministic provider health snapshot reference.
type Output struct {
	ProviderHealthSnapshot string
}

// Backend resolves provider-health snapshots from a snapshot-fed control-plane source.
type Backend interface {
	GetSnapshot(in Input) (Output, error)
}

// Service resolves CP-10 provider health snapshots for runtime turn-start freeze.
type Service struct {
	DefaultProviderHealthSnapshot string
	Backend                       Backend
}

// NewService returns the baseline CP-10 provider health resolver.
func NewService() Service {
	return Service{DefaultProviderHealthSnapshot: defaultProviderHealthSnapshot}
}

// GetSnapshot resolves a provider health snapshot reference.
func (s Service) GetSnapshot(in Input) (Output, error) {
	if in.Scope == "" {
		return Output{}, fmt.Errorf("scope is required")
	}

	if s.Backend != nil {
		out, err := s.Backend.GetSnapshot(in)
		if err != nil {
			return Output{}, fmt.Errorf("resolve provider health snapshot backend: %w", err)
		}
		return s.normalizeOutput(out), nil
	}

	return s.normalizeOutput(Output{}), nil
}

func (s Service) normalizeOutput(out Output) Output {
	snapshot := s.DefaultProviderHealthSnapshot
	if snapshot == "" {
		snapshot = defaultProviderHealthSnapshot
	}
	if out.ProviderHealthSnapshot != "" {
		snapshot = out.ProviderHealthSnapshot
	}
	return Output{ProviderHealthSnapshot: snapshot}
}
