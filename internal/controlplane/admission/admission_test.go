package admission

import (
	"errors"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

func TestEvaluateDefaults(t *testing.T) {
	t.Parallel()

	out, err := NewService().Evaluate(Input{SessionID: "sess-1", TurnID: "turn-1", PipelineVersion: "pipeline-v1"})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	if out.OutcomeKind != controlplane.OutcomeAdmit {
		t.Fatalf("expected default admit outcome, got %+v", out)
	}
	if out.Scope != controlplane.ScopeSession {
		t.Fatalf("expected session scope for default output, got %+v", out)
	}
	if out.AdmissionPolicySnapshot != defaultAdmissionPolicySnapshot {
		t.Fatalf("unexpected default admission snapshot: %+v", out)
	}
	if out.Reason != ReasonAllowed {
		t.Fatalf("unexpected default reason: %+v", out)
	}
}

func TestEvaluateDefaultsTenantScope(t *testing.T) {
	t.Parallel()

	out, err := NewService().Evaluate(Input{TenantID: "tenant-1", SessionID: "sess-1", PipelineVersion: "pipeline-v1"})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	if out.Scope != controlplane.ScopeTenant {
		t.Fatalf("expected tenant scope when tenant is set, got %+v", out)
	}
}

func TestEvaluateUsesBackendAndNormalizes(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubBackend{
		evalFn: func(Input) (Output, error) {
			return Output{
				AdmissionPolicySnapshot: "admission-policy/backend",
				OutcomeKind:             controlplane.OutcomeKind("bad_outcome"),
				Scope:                   controlplane.ScopeTurn,
			}, nil
		},
	}

	out, err := service.Evaluate(Input{SessionID: "sess-1", TurnID: "turn-1", PipelineVersion: "pipeline-v1"})
	if err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	if out.AdmissionPolicySnapshot != "admission-policy/backend" {
		t.Fatalf("expected backend snapshot override, got %+v", out)
	}
	if out.OutcomeKind != controlplane.OutcomeReject {
		t.Fatalf("expected invalid backend outcome to normalize to reject, got %+v", out)
	}
	if out.Scope != controlplane.ScopeSession {
		t.Fatalf("expected invalid backend scope to normalize to session scope, got %+v", out)
	}
	if out.Reason != ReasonRejectPolicy {
		t.Fatalf("expected reject reason default, got %+v", out)
	}
}

func TestEvaluateWrapsBackendError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("backend unavailable")
	service := NewService()
	service.Backend = stubBackend{evalFn: func(Input) (Output, error) { return Output{}, sentinel }}
	_, err := service.Evaluate(Input{SessionID: "sess-1"})
	if err == nil {
		t.Fatalf("expected backend error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped backend error, got %v", err)
	}
}

func TestEvaluateRequiresSessionID(t *testing.T) {
	t.Parallel()

	_, err := NewService().Evaluate(Input{})
	if err == nil {
		t.Fatalf("expected missing session error")
	}
}

type stubBackend struct {
	evalFn func(in Input) (Output, error)
}

func (s stubBackend) Evaluate(in Input) (Output, error) {
	return s.evalFn(in)
}
