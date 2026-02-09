package budget

import "fmt"

// Action is the deterministic budget decision.
type Action string

const (
	ActionContinue  Action = "continue"
	ActionDegrade   Action = "degrade"
	ActionFallback  Action = "fallback"
	ActionTerminate Action = "terminate"
)

// BudgetSpec defines deterministic budget thresholds and allowed adaptation.
type BudgetSpec struct {
	WarnAtMS      int64
	ExhaustAtMS   int64
	AllowDegrade  bool
	AllowFallback bool
}

// Usage captures observed execution usage.
type Usage struct {
	ElapsedMS int64
}

// Decision is the deterministic outcome for budget evaluation.
type Decision struct {
	Action        Action
	Reason        string
	EmitWarning   bool
	EmitExhausted bool
}

// Manager evaluates budgets.
type Manager struct{}

// NewManager returns a deterministic budget manager.
func NewManager() Manager {
	return Manager{}
}

// Evaluate returns deterministic continue/degrade/fallback/terminate decisions.
func (Manager) Evaluate(spec BudgetSpec, usage Usage) (Decision, error) {
	if spec.WarnAtMS < 0 || spec.ExhaustAtMS < 0 {
		return Decision{}, fmt.Errorf("budget thresholds must be >=0")
	}
	if usage.ElapsedMS < 0 {
		return Decision{}, fmt.Errorf("usage elapsed_ms must be >=0")
	}
	if spec.ExhaustAtMS == 0 {
		spec.ExhaustAtMS = 1
	}
	if spec.WarnAtMS == 0 {
		spec.WarnAtMS = spec.ExhaustAtMS
	}
	if spec.WarnAtMS > spec.ExhaustAtMS {
		return Decision{}, fmt.Errorf("warn_at_ms must be <= exhaust_at_ms")
	}

	if usage.ElapsedMS < spec.WarnAtMS {
		return Decision{Action: ActionContinue, Reason: "within_budget"}, nil
	}
	if usage.ElapsedMS < spec.ExhaustAtMS {
		return Decision{
			Action:      ActionContinue,
			Reason:      "budget_warning",
			EmitWarning: true,
		}, nil
	}

	decision := Decision{
		Reason:        "budget_exhausted",
		EmitWarning:   true,
		EmitExhausted: true,
	}
	if spec.AllowDegrade {
		decision.Action = ActionDegrade
		return decision, nil
	}
	if spec.AllowFallback {
		decision.Action = ActionFallback
		return decision, nil
	}
	decision.Action = ActionTerminate
	return decision, nil
}
