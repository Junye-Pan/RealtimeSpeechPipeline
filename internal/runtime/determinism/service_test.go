package determinism

import "testing"

func TestIssueContextDeterministic(t *testing.T) {
	t.Parallel()

	svc := NewService()
	first, err := svc.IssueContext("abc123", 10)
	if err != nil {
		t.Fatalf("unexpected issue error: %v", err)
	}
	second, err := svc.IssueContext("abc123", 10)
	if err != nil {
		t.Fatalf("unexpected second issue error: %v", err)
	}
	if first.Seed != second.Seed {
		t.Fatalf("expected deterministic seed, got %d and %d", first.Seed, second.Seed)
	}
	if first.MergeRuleID == "" || first.MergeRuleVersion == "" {
		t.Fatalf("expected merge rule metadata to be set, got %+v", first)
	}
}

func TestIssueContextRejectsEmptyPlanHash(t *testing.T) {
	t.Parallel()

	svc := NewService()
	if _, err := svc.IssueContext("", 1); err == nil {
		t.Fatalf("expected empty plan hash to fail")
	}
}
