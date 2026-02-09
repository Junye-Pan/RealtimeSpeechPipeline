package identity

import "testing"

func TestNewEventContextDeterministicSequence(t *testing.T) {
	t.Parallel()

	svc := NewService()
	first, err := svc.NewEventContext("sess-identity-1", "turn-identity-1")
	if err != nil {
		t.Fatalf("unexpected first context error: %v", err)
	}
	second, err := svc.NewEventContext("sess-identity-1", "turn-identity-1")
	if err != nil {
		t.Fatalf("unexpected second context error: %v", err)
	}
	if first.EventID == second.EventID {
		t.Fatalf("expected deterministic incrementing event_id, got %s and %s", first.EventID, second.EventID)
	}
	if first.CorrelationID != second.CorrelationID {
		t.Fatalf("expected stable correlation id, got %s and %s", first.CorrelationID, second.CorrelationID)
	}
}

func TestValidateContextRejectsMissingFields(t *testing.T) {
	t.Parallel()

	if err := ValidateContext(Context{}); err == nil {
		t.Fatalf("expected empty context to fail validation")
	}
}
