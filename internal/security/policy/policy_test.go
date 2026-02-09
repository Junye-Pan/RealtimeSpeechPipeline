package policy

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestValidateAutomatedRecordingLevelTransition(t *testing.T) {
	t.Parallel()

	if err := ValidateAutomatedRecordingLevelTransition(LevelL1, LevelL0); err != nil {
		t.Fatalf("expected downgrade transition to pass: %v", err)
	}
	if err := ValidateAutomatedRecordingLevelTransition(LevelL0, LevelL0); err != nil {
		t.Fatalf("expected same-level transition to pass: %v", err)
	}
	if err := ValidateAutomatedRecordingLevelTransition(LevelL0, LevelL1); err == nil {
		t.Fatalf("expected auto-upgrade transition to fail")
	}
}

func TestResolveDefaultRedactionAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		surface  ReplaySurface
		level    RecordingLevel
		class    eventabi.PayloadClass
		expected eventabi.RedactionAction
	}{
		{
			name:     "OR01 drops audio raw",
			surface:  SurfaceOR01,
			level:    LevelL0,
			class:    eventabi.PayloadAudioRaw,
			expected: eventabi.RedactionDrop,
		},
		{
			name:     "OR02 masks text raw at L1",
			surface:  SurfaceOR02,
			level:    LevelL1,
			class:    eventabi.PayloadTextRaw,
			expected: eventabi.RedactionMask,
		},
		{
			name:     "OR03 tokenizes PII",
			surface:  SurfaceOR03,
			level:    LevelL2,
			class:    eventabi.PayloadPII,
			expected: eventabi.RedactionTokenize,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveDefaultRedactionAction(tc.surface, tc.level, tc.class)
			if err != nil {
				t.Fatalf("unexpected resolve error: %v", err)
			}
			if got != tc.expected {
				t.Fatalf("expected action %s, got %s", tc.expected, got)
			}
		})
	}
}

func TestBuildDefaultRedactionDecisions(t *testing.T) {
	t.Parallel()

	decisions, err := BuildDefaultRedactionDecisions(
		SurfaceOR02,
		LevelL0,
		[]eventabi.PayloadClass{
			eventabi.PayloadMetadata,
			eventabi.PayloadTextRaw,
			eventabi.PayloadMetadata,
		},
	)
	if err != nil {
		t.Fatalf("unexpected build decisions error: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected deduped decisions length 2, got %d", len(decisions))
	}
	if decisions[0].PayloadClass != eventabi.PayloadMetadata || decisions[0].Action != eventabi.RedactionAllow {
		t.Fatalf("unexpected first decision: %+v", decisions[0])
	}
	if decisions[1].PayloadClass != eventabi.PayloadTextRaw || decisions[1].Action != eventabi.RedactionHash {
		t.Fatalf("unexpected second decision: %+v", decisions[1])
	}
}
