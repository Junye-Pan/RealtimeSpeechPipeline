package timeline

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestDefaultOR02RedactionDecisions(t *testing.T) {
	t.Parallel()

	decisions, err := DefaultOR02RedactionDecisions("L0", []eventabi.PayloadClass{
		eventabi.PayloadMetadata,
		eventabi.PayloadTextRaw,
	})
	if err != nil {
		t.Fatalf("unexpected default decision error: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	if decisions[0].PayloadClass != eventabi.PayloadMetadata || decisions[0].Action != eventabi.RedactionAllow {
		t.Fatalf("unexpected first decision: %+v", decisions[0])
	}
	if decisions[1].PayloadClass != eventabi.PayloadTextRaw || decisions[1].Action != eventabi.RedactionHash {
		t.Fatalf("unexpected second decision: %+v", decisions[1])
	}
}

func TestEnsureOR02RedactionDecisionsPrefersExisting(t *testing.T) {
	t.Parallel()

	existing := []eventabi.RedactionDecision{
		{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow},
	}
	out, err := EnsureOR02RedactionDecisions("L0", []eventabi.PayloadClass{eventabi.PayloadMetadata}, existing)
	if err != nil {
		t.Fatalf("unexpected ensure error: %v", err)
	}
	if len(out) != 1 || out[0] != existing[0] {
		t.Fatalf("expected existing decisions to be preserved, got %+v", out)
	}
}

func TestValidateCompletenessRequiresRedactionDecisions(t *testing.T) {
	t.Parallel()

	baseline := minimalBaseline("turn-redaction-missing")
	baseline.RedactionDecisions = nil
	if err := baseline.ValidateCompleteness(); err == nil {
		t.Fatalf("expected completeness validation failure for missing redaction decisions")
	}
}
