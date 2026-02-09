package contracts

import (
	"fmt"
	"sort"
)

// Modality defines provider families supported by runtime invocation.
type Modality string

const (
	ModalitySTT Modality = "stt"
	ModalityLLM Modality = "llm"
	ModalityTTS Modality = "tts"
)

// Validate enforces supported provider modality values.
func (m Modality) Validate() error {
	switch m {
	case ModalitySTT, ModalityLLM, ModalityTTS:
		return nil
	default:
		return fmt.Errorf("unsupported modality: %q", m)
	}
}

// OutcomeClass is the normalized invocation-outcome taxonomy.
type OutcomeClass string

const (
	OutcomeSuccess               OutcomeClass = "success"
	OutcomeTimeout               OutcomeClass = "timeout"
	OutcomeOverload              OutcomeClass = "overload"
	OutcomeBlocked               OutcomeClass = "blocked"
	OutcomeInfrastructureFailure OutcomeClass = "infrastructure_failure"
	OutcomeCancelled             OutcomeClass = "cancelled"
)

// Validate enforces supported outcome classes.
func (o OutcomeClass) Validate() error {
	switch o {
	case OutcomeSuccess, OutcomeTimeout, OutcomeOverload, OutcomeBlocked, OutcomeInfrastructureFailure, OutcomeCancelled:
		return nil
	default:
		return fmt.Errorf("unsupported outcome_class: %q", o)
	}
}

// InvocationRequest is passed to adapter implementations per attempt.
type InvocationRequest struct {
	SessionID              string
	TurnID                 string
	PipelineVersion        string
	EventID                string
	ProviderInvocationID   string
	ProviderID             string
	Modality               Modality
	Attempt                int
	TransportSequence      int64
	RuntimeSequence        int64
	AuthorityEpoch         int64
	RuntimeTimestampMS     int64
	WallClockTimestampMS   int64
	CancelRequested        bool
	AllowedAdaptiveActions []string
	RetryBudgetRemaining   int
	CandidateProviderCount int
}

// Validate enforces deterministic required fields.
func (r InvocationRequest) Validate() error {
	if r.SessionID == "" || r.PipelineVersion == "" || r.EventID == "" {
		return fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	if r.ProviderInvocationID == "" || r.ProviderID == "" {
		return fmt.Errorf("provider_invocation_id and provider_id are required")
	}
	if err := r.Modality.Validate(); err != nil {
		return err
	}
	if r.Attempt < 1 {
		return fmt.Errorf("attempt must be >=1")
	}
	if r.TransportSequence < 0 || r.RuntimeSequence < 0 || r.AuthorityEpoch < 0 {
		return fmt.Errorf("sequence and authority values must be >=0")
	}
	if r.RuntimeTimestampMS < 0 || r.WallClockTimestampMS < 0 {
		return fmt.Errorf("timestamps must be >=0")
	}
	if _, err := NormalizeAdaptiveActions(r.AllowedAdaptiveActions); err != nil {
		return err
	}
	if r.RetryBudgetRemaining < 0 {
		return fmt.Errorf("retry_budget_remaining must be >=0")
	}
	if r.CandidateProviderCount < 0 {
		return fmt.Errorf("candidate_provider_count must be >=0")
	}
	return nil
}

// NormalizeAdaptiveActions validates and returns sorted unique adaptive actions.
func NormalizeAdaptiveActions(actions []string) ([]string, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action {
		case "retry", "provider_switch", "fallback":
			if _, exists := seen[action]; exists {
				return nil, fmt.Errorf("duplicate adaptive action: %q", action)
			}
			seen[action] = struct{}{}
			out = append(out, action)
		default:
			return nil, fmt.Errorf("unsupported adaptive action: %q", action)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Outcome is an adapter-normalized invocation result.
type Outcome struct {
	Class       OutcomeClass
	Retryable   bool
	Reason      string
	CircuitOpen bool
	BackoffMS   int64
}

// Validate enforces normalized outcome invariants.
func (o Outcome) Validate() error {
	if err := o.Class.Validate(); err != nil {
		return err
	}
	if o.Class != OutcomeSuccess && o.Reason == "" {
		return fmt.Errorf("reason is required for non-success outcomes")
	}
	if o.BackoffMS < 0 {
		return fmt.Errorf("backoff_ms must be >=0")
	}
	if o.CircuitOpen && o.Class == OutcomeSuccess {
		return fmt.Errorf("circuit_open cannot be true for success")
	}
	return nil
}

// Adapter defines RK-10 provider adapter behavior.
type Adapter interface {
	ProviderID() string
	Modality() Modality
	Invoke(InvocationRequest) (Outcome, error)
}

// StaticAdapter is a small utility adapter for tests and static catalogs.
type StaticAdapter struct {
	ID       string
	Mode     Modality
	InvokeFn func(InvocationRequest) (Outcome, error)
}

func (a StaticAdapter) ProviderID() string {
	return a.ID
}

func (a StaticAdapter) Modality() Modality {
	return a.Mode
}

func (a StaticAdapter) Invoke(req InvocationRequest) (Outcome, error) {
	if a.InvokeFn != nil {
		return a.InvokeFn(req)
	}
	if err := req.Validate(); err != nil {
		return Outcome{}, err
	}
	return Outcome{Class: OutcomeSuccess}, nil
}
