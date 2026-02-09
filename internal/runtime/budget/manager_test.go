package budget

import "testing"

func TestEvaluateBudgetPaths(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	spec := BudgetSpec{WarnAtMS: 100, ExhaustAtMS: 200}

	within, err := manager.Evaluate(spec, Usage{ElapsedMS: 50})
	if err != nil {
		t.Fatalf("unexpected within-budget error: %v", err)
	}
	if within.Action != ActionContinue || within.Reason != "within_budget" {
		t.Fatalf("expected continue/within_budget, got %+v", within)
	}

	warn, err := manager.Evaluate(spec, Usage{ElapsedMS: 150})
	if err != nil {
		t.Fatalf("unexpected warning error: %v", err)
	}
	if warn.Action != ActionContinue || !warn.EmitWarning {
		t.Fatalf("expected warning continue path, got %+v", warn)
	}

	terminate, err := manager.Evaluate(spec, Usage{ElapsedMS: 200})
	if err != nil {
		t.Fatalf("unexpected terminate error: %v", err)
	}
	if terminate.Action != ActionTerminate || !terminate.EmitExhausted {
		t.Fatalf("expected terminate exhausted path, got %+v", terminate)
	}
}

func TestEvaluateBudgetAdaptiveExhausted(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	degrade, err := manager.Evaluate(BudgetSpec{
		WarnAtMS:     10,
		ExhaustAtMS:  20,
		AllowDegrade: true,
	}, Usage{ElapsedMS: 20})
	if err != nil {
		t.Fatalf("unexpected degrade error: %v", err)
	}
	if degrade.Action != ActionDegrade {
		t.Fatalf("expected degrade action, got %+v", degrade)
	}

	fallback, err := manager.Evaluate(BudgetSpec{
		WarnAtMS:      10,
		ExhaustAtMS:   20,
		AllowFallback: true,
	}, Usage{ElapsedMS: 20})
	if err != nil {
		t.Fatalf("unexpected fallback error: %v", err)
	}
	if fallback.Action != ActionFallback {
		t.Fatalf("expected fallback action, got %+v", fallback)
	}
}
