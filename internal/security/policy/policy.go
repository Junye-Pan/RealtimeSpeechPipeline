package policy

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// RecordingLevel mirrors MVP recording fidelity levels.
type RecordingLevel string

const (
	LevelL0 RecordingLevel = "L0"
	LevelL1 RecordingLevel = "L1"
	LevelL2 RecordingLevel = "L2"
)

// ReplaySurface maps redaction decisions to OR module surfaces.
type ReplaySurface string

const (
	SurfaceOR01 ReplaySurface = "OR-01"
	SurfaceOR02 ReplaySurface = "OR-02"
	SurfaceOR03 ReplaySurface = "OR-03"
)

// ParseRecordingLevel parses/validates fidelity level strings.
func ParseRecordingLevel(value string) (RecordingLevel, error) {
	level := RecordingLevel(value)
	if err := level.Validate(); err != nil {
		return "", err
	}
	return level, nil
}

// Validate enforces supported recording levels.
func (l RecordingLevel) Validate() error {
	switch l {
	case LevelL0, LevelL1, LevelL2:
		return nil
	default:
		return fmt.Errorf("invalid recording level: %q", l)
	}
}

// Validate enforces supported replay surfaces.
func (s ReplaySurface) Validate() error {
	switch s {
	case SurfaceOR01, SurfaceOR02, SurfaceOR03:
		return nil
	default:
		return fmt.Errorf("invalid replay surface: %q", s)
	}
}

// ValidateAutomatedRecordingLevelTransition enforces deterministic transition controls.
// Runtime may downgrade under pressure, but must not auto-upgrade fidelity.
func ValidateAutomatedRecordingLevelTransition(current RecordingLevel, next RecordingLevel) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if rank(next) > rank(current) {
		return fmt.Errorf("recording level auto-upgrade forbidden: %s -> %s", current, next)
	}
	return nil
}

// ResolveDefaultRedactionAction encodes the baseline default redaction matrix.
func ResolveDefaultRedactionAction(surface ReplaySurface, level RecordingLevel, class eventabi.PayloadClass) (eventabi.RedactionAction, error) {
	if err := surface.Validate(); err != nil {
		return "", err
	}
	if err := level.Validate(); err != nil {
		return "", err
	}
	if err := (eventabi.RedactionDecision{PayloadClass: class, Action: eventabi.RedactionAllow}).Validate(); err != nil {
		return "", err
	}

	switch surface {
	case SurfaceOR01:
		switch class {
		case eventabi.PayloadAudioRaw:
			return eventabi.RedactionDrop, nil
		case eventabi.PayloadTextRaw:
			return eventabi.RedactionMask, nil
		case eventabi.PayloadPII, eventabi.PayloadPHI:
			return eventabi.RedactionDrop, nil
		case eventabi.PayloadDerivedSummary, eventabi.PayloadMetadata:
			return eventabi.RedactionAllow, nil
		}
	case SurfaceOR02:
		switch class {
		case eventabi.PayloadAudioRaw:
			switch level {
			case LevelL0:
				return eventabi.RedactionDrop, nil
			case LevelL1:
				return eventabi.RedactionMask, nil
			default:
				return eventabi.RedactionHash, nil
			}
		case eventabi.PayloadTextRaw:
			switch level {
			case LevelL0:
				return eventabi.RedactionHash, nil
			default:
				return eventabi.RedactionMask, nil
			}
		case eventabi.PayloadPII:
			return eventabi.RedactionHash, nil
		case eventabi.PayloadPHI:
			if level == LevelL0 {
				return eventabi.RedactionDrop, nil
			}
			return eventabi.RedactionHash, nil
		case eventabi.PayloadDerivedSummary, eventabi.PayloadMetadata:
			return eventabi.RedactionAllow, nil
		}
	case SurfaceOR03:
		switch class {
		case eventabi.PayloadAudioRaw:
			return eventabi.RedactionDrop, nil
		case eventabi.PayloadTextRaw:
			return eventabi.RedactionMask, nil
		case eventabi.PayloadPII, eventabi.PayloadPHI:
			return eventabi.RedactionTokenize, nil
		case eventabi.PayloadDerivedSummary, eventabi.PayloadMetadata:
			return eventabi.RedactionAllow, nil
		}
	}

	return "", fmt.Errorf("unsupported redaction matrix entry for surface=%s class=%s level=%s", surface, class, level)
}

// BuildDefaultRedactionDecisions resolves default decisions for tagged payload classes.
func BuildDefaultRedactionDecisions(surface ReplaySurface, level RecordingLevel, tags []eventabi.PayloadClass) ([]eventabi.RedactionDecision, error) {
	if len(tags) == 0 {
		return nil, fmt.Errorf("at least one payload tag is required")
	}

	seen := map[eventabi.PayloadClass]struct{}{}
	decisions := make([]eventabi.RedactionDecision, 0, len(tags))
	for _, tag := range tags {
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		action, err := ResolveDefaultRedactionAction(surface, level, tag)
		if err != nil {
			return nil, err
		}
		decision := eventabi.RedactionDecision{PayloadClass: tag, Action: action}
		if err := decision.Validate(); err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func rank(level RecordingLevel) int {
	switch level {
	case LevelL0:
		return 0
	case LevelL1:
		return 1
	case LevelL2:
		return 2
	default:
		return -1
	}
}
