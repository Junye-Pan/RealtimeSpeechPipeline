package regression

import (
	"testing"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

func TestEvaluateDivergencesUnexplainedPlanFails(t *testing.T) {
	t.Parallel()

	eval := EvaluateDivergences([]obs.ReplayDivergence{{
		Class:   obs.PlanDivergence,
		Scope:   "turn:t1",
		Message: "plan hash mismatch",
	}}, DivergencePolicy{TimingToleranceMS: 10})

	if len(eval.Failing) != 1 {
		t.Fatalf("expected one failing divergence, got %+v", eval.Failing)
	}
	if len(eval.Unexplained) != 1 {
		t.Fatalf("expected one unexplained divergence, got %+v", eval.Unexplained)
	}
}

func TestEvaluateDivergencesOrderingRequiresApprovedExpectation(t *testing.T) {
	t.Parallel()

	ordering := obs.ReplayDivergence{Class: obs.OrderingDivergence, Scope: "turn:t2", Message: "ordering mismatch"}
	approved := EvaluateDivergences([]obs.ReplayDivergence{ordering}, DivergencePolicy{
		Expected: []ExpectedDivergence{{Class: obs.OrderingDivergence, Scope: "turn:t2", Approved: true}},
	})
	if len(approved.Failing) != 0 {
		t.Fatalf("expected approved ordering divergence to pass, got %+v", approved.Failing)
	}

	notApproved := EvaluateDivergences([]obs.ReplayDivergence{ordering}, DivergencePolicy{
		Expected: []ExpectedDivergence{{Class: obs.OrderingDivergence, Scope: "turn:t2", Approved: false}},
	})
	if len(notApproved.Failing) != 1 {
		t.Fatalf("expected unapproved ordering divergence to fail, got %+v", notApproved.Failing)
	}
}

func TestEvaluateDivergencesTimingTolerance(t *testing.T) {
	t.Parallel()

	within := int64(12)
	outside := int64(35)
	pass := EvaluateDivergences([]obs.ReplayDivergence{{
		Class:   obs.TimingDivergence,
		Scope:   "turn:t3",
		Message: "timing mismatch",
		DiffMS:  &within,
	}}, DivergencePolicy{TimingToleranceMS: 15})
	if len(pass.Failing) != 0 {
		t.Fatalf("expected timing divergence within tolerance to pass, got %+v", pass.Failing)
	}

	fail := EvaluateDivergences([]obs.ReplayDivergence{{
		Class:   obs.TimingDivergence,
		Scope:   "turn:t3",
		Message: "timing mismatch",
		DiffMS:  &outside,
	}}, DivergencePolicy{TimingToleranceMS: 15})
	if len(fail.Failing) != 1 {
		t.Fatalf("expected timing divergence over tolerance to fail, got %+v", fail.Failing)
	}
}

func TestEvaluateDivergencesAuthorityAlwaysFails(t *testing.T) {
	t.Parallel()

	epochDiff := EvaluateDivergences([]obs.ReplayDivergence{{
		Class:   obs.AuthorityDivergence,
		Scope:   "turn:t4",
		Message: "authority epoch mismatch",
	}}, DivergencePolicy{
		Expected: []ExpectedDivergence{{Class: obs.AuthorityDivergence, Scope: "turn:t4", Approved: true}},
	})
	if len(epochDiff.Failing) != 1 {
		t.Fatalf("expected authority divergence to always fail, got %+v", epochDiff.Failing)
	}
}

func TestEvaluateDivergencesProviderChoiceRequiresExpectation(t *testing.T) {
	t.Parallel()

	divergence := obs.ReplayDivergence{
		Class:   obs.ProviderChoiceDivergence,
		Scope:   "turn:t-provider",
		Message: "provider/model mismatch",
	}

	unexpected := EvaluateDivergences([]obs.ReplayDivergence{divergence}, DivergencePolicy{})
	if len(unexpected.Failing) != 1 || len(unexpected.Unexplained) != 1 {
		t.Fatalf("expected unexplained provider choice divergence to fail, got %+v", unexpected)
	}

	expected := EvaluateDivergences([]obs.ReplayDivergence{divergence}, DivergencePolicy{
		Expected: []ExpectedDivergence{{
			Class: obs.ProviderChoiceDivergence,
			Scope: "turn:t-provider",
		}},
	})
	if len(expected.Failing) != 0 || len(expected.Unexplained) != 0 {
		t.Fatalf("expected configured provider choice divergence to pass, got %+v", expected)
	}
}

func TestEvaluateDivergencesMissingExpectedFails(t *testing.T) {
	t.Parallel()

	eval := EvaluateDivergences(nil, DivergencePolicy{
		Expected: []ExpectedDivergence{{Class: obs.OutcomeDivergence, Scope: "turn:missing", Approved: true}},
	})
	if len(eval.MissingExpected) != 1 {
		t.Fatalf("expected one missing expected divergence, got %+v", eval.MissingExpected)
	}
	if len(eval.Failing) != 1 {
		t.Fatalf("expected missing expected divergence to fail, got %+v", eval.Failing)
	}
}

func TestEvaluateDivergencesInvocationLatencyTimingAlwaysFails(t *testing.T) {
	t.Parallel()

	diff := int64(1)
	eval := EvaluateDivergences([]obs.ReplayDivergence{
		{
			Class:   obs.TimingDivergence,
			Scope:   "invocation_latency_total:turn:t5",
			Message: "total invocation latency exceeded threshold",
			DiffMS:  &diff,
		},
		{
			Class:   obs.TimingDivergence,
			Scope:   "invocation_latency_final:turn:t5",
			Message: "final invocation latency exceeded threshold",
			DiffMS:  &diff,
		},
	}, DivergencePolicy{TimingToleranceMS: 15})

	if len(eval.Failing) != 2 {
		t.Fatalf("expected invocation latency timing divergence to fail regardless of tolerance, got %+v", eval.Failing)
	}
}
