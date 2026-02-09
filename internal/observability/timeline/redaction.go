package timeline

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	securitypolicy "github.com/tiger/realtime-speech-pipeline/internal/security/policy"
)

// DefaultOR02RedactionDecisions resolves default OR-02 redaction decisions for payload tags.
func DefaultOR02RedactionDecisions(recordingLevel string, payloadTags []eventabi.PayloadClass) ([]eventabi.RedactionDecision, error) {
	level, err := securitypolicy.ParseRecordingLevel(recordingLevel)
	if err != nil {
		return nil, err
	}
	return securitypolicy.BuildDefaultRedactionDecisions(securitypolicy.SurfaceOR02, level, payloadTags)
}

// EnsureOR02RedactionDecisions preserves explicit decisions, else fills deterministic defaults.
func EnsureOR02RedactionDecisions(recordingLevel string, payloadTags []eventabi.PayloadClass, existing []eventabi.RedactionDecision) ([]eventabi.RedactionDecision, error) {
	if len(existing) > 0 {
		return existing, nil
	}
	decisions, err := DefaultOR02RedactionDecisions(recordingLevel, payloadTags)
	if err != nil {
		return nil, fmt.Errorf("resolve default OR-02 redaction decisions: %w", err)
	}
	return decisions, nil
}
