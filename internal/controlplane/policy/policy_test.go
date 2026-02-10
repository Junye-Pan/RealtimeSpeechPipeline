package policy

import (
	"errors"
	"reflect"
	"testing"
)

func TestEvaluate(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.DefaultAllowedAdaptiveActions = []string{"fallback", "retry", "retry", "provider_switch", "invalid"}

	out, err := service.Evaluate(Input{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("unexpected policy evaluation error: %v", err)
	}
	if out.PolicyResolutionSnapshot == "" {
		t.Fatalf("expected non-empty policy snapshot")
	}
	expectedActions := []string{"retry", "provider_switch", "fallback"}
	if !reflect.DeepEqual(out.AllowedAdaptiveActions, expectedActions) {
		t.Fatalf("expected normalized allowed actions %v, got %v", expectedActions, out.AllowedAdaptiveActions)
	}
}

func TestEvaluateRequiresSessionID(t *testing.T) {
	t.Parallel()

	if _, err := NewService().Evaluate(Input{}); err == nil {
		t.Fatalf("expected session_id validation error")
	}
}

func TestEvaluateUsesBackendWhenConfigured(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubPolicyBackend{
		evalFn: func(Input) (Output, error) {
			return Output{
				PolicyResolutionSnapshot: "policy-resolution/backend",
				AllowedAdaptiveActions:   []string{"degrade", "fallback", "retry"},
			}, nil
		},
	}

	out, err := service.Evaluate(Input{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("unexpected backend evaluation error: %v", err)
	}
	expectedActions := []string{"retry", "degrade", "fallback"}
	if out.PolicyResolutionSnapshot != "policy-resolution/backend" || !reflect.DeepEqual(out.AllowedAdaptiveActions, expectedActions) {
		t.Fatalf("unexpected backend policy output: %+v", out)
	}
}

func TestEvaluateBackendError(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubPolicyBackend{
		evalFn: func(Input) (Output, error) {
			return Output{}, errors.New("backend unavailable")
		},
	}

	if _, err := service.Evaluate(Input{SessionID: "sess-1"}); err == nil {
		t.Fatalf("expected backend error")
	}
}

type stubPolicyBackend struct {
	evalFn func(in Input) (Output, error)
}

func (s stubPolicyBackend) Evaluate(in Input) (Output, error) {
	return s.evalFn(in)
}
