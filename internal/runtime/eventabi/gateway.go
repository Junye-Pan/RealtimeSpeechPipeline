package eventabi

import (
	"fmt"

	apieventabi "github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// ValidateAndNormalizeControlSignals performs runtime-side normalization and validation
// for control signals before persistence or egress.
func ValidateAndNormalizeControlSignals(in []apieventabi.ControlSignal) ([]apieventabi.ControlSignal, error) {
	out := make([]apieventabi.ControlSignal, len(in))
	for i, sig := range in {
		normalized, err := normalizeSignal(sig)
		if err != nil {
			return nil, fmt.Errorf("normalize control signal[%d]: %w", i, err)
		}
		if err := normalized.Validate(); err != nil {
			return nil, fmt.Errorf("validate control signal[%d]: %w", i, err)
		}
		if i > 0 {
			prev := out[i-1]
			if normalized.RuntimeSequence < prev.RuntimeSequence {
				return nil, fmt.Errorf(
					"runtime_sequence regression at index %d: %d < %d",
					i,
					normalized.RuntimeSequence,
					prev.RuntimeSequence,
				)
			}
			if normalized.TransportSequence != nil && prev.TransportSequence != nil &&
				*normalized.TransportSequence < *prev.TransportSequence {
				return nil, fmt.Errorf(
					"transport_sequence regression at index %d: %d < %d",
					i,
					*normalized.TransportSequence,
					*prev.TransportSequence,
				)
			}
		}
		out[i] = normalized
	}
	return out, nil
}

func normalizeSignal(sig apieventabi.ControlSignal) (apieventabi.ControlSignal, error) {
	if sig.SchemaVersion == "" {
		sig.SchemaVersion = "v1.0"
	}
	if sig.Scope == "" {
		if sig.EventScope == apieventabi.ScopeTurn {
			sig.Scope = "turn"
		} else {
			sig.Scope = "session"
		}
	}
	if sig.TransportSequence == nil {
		zero := int64(0)
		sig.TransportSequence = &zero
	}
	if sig.RuntimeSequence < 0 {
		sig.RuntimeSequence = 0
	}
	if sig.AuthorityEpoch < 0 {
		sig.AuthorityEpoch = 0
	}
	if sig.RuntimeTimestampMS < 0 {
		sig.RuntimeTimestampMS = 0
	}
	if sig.WallClockMS < 0 {
		sig.WallClockMS = 0
	}
	if sig.EventScope == apieventabi.ScopeTurn && sig.TurnID == "" {
		return apieventabi.ControlSignal{}, fmt.Errorf("turn scope signal requires turn_id")
	}
	return sig, nil
}
