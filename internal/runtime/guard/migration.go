package guard

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// MigrationInput defines deterministic authority-handoff signal context for F8.
type MigrationInput struct {
	SessionID             string
	TurnID                string
	PipelineVersion       string
	EventIDBase           string
	TransportSequence     int64
	RuntimeSequence       int64
	AuthorityEpoch        int64
	RuntimeTimestampMS    int64
	WallClockTimestampMS  int64
	IncludeSessionHandoff bool
}

// BuildMigrationSignals emits deterministic lease/migration/handoff signals.
func BuildMigrationSignals(in MigrationInput) ([]eventabi.ControlSignal, error) {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EventIDBase == "" {
		return nil, fmt.Errorf("session_id, pipeline_version, and event_id_base are required")
	}

	sigs := make([]eventabi.ControlSignal, 0, 4)
	ordered := []struct {
		signal  string
		emitter string
		reason  string
	}{
		{signal: "lease_rotated", emitter: "CP-07", reason: "authority_epoch_rotation"},
		{signal: "migration_start", emitter: "CP-08", reason: "region_failover_start"},
		{signal: "migration_finish", emitter: "CP-08", reason: "region_failover_complete"},
	}

	for i, s := range ordered {
		sig, err := buildMigrationSignal(in, s.signal, s.emitter, s.reason, int64(i))
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, sig)
	}

	if in.IncludeSessionHandoff {
		sig, err := buildMigrationSignal(in, "session_handoff", "CP-08", "authoritative_handoff", int64(len(sigs)))
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, sig)
	}

	return sigs, nil
}

// RejectOldPlacementOutput returns deterministic stale-epoch rejection artifacts.
func RejectOldPlacementOutput(sessionID, turnID, eventID string, runtimeTS, wallTS, authorityEpoch int64) (controlplane.DecisionOutcome, eventabi.ControlSignal, error) {
	epoch := authorityEpoch
	outcome := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeStaleEpochReject,
		Phase:              controlplane.PhaseScheduling,
		Scope:              controlplane.ScopeNodeDispatch,
		SessionID:          sessionID,
		TurnID:             turnID,
		EventID:            eventID,
		RuntimeTimestampMS: runtimeTS,
		WallClockMS:        wallTS,
		EmittedBy:          controlplane.EmitterRK24,
		AuthorityEpoch:     &epoch,
		Reason:             "authority_epoch_mismatch",
	}
	if err := outcome.Validate(); err != nil {
		return controlplane.DecisionOutcome{}, eventabi.ControlSignal{}, err
	}

	eventScope := eventabi.ScopeSession
	scope := "session"
	if turnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transportSequence := int64(0)
	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          sessionID,
		TurnID:             turnID,
		PipelineVersion:    "pipeline-v1",
		EventID:            eventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transportSequence,
		RuntimeSequence:    0,
		AuthorityEpoch:     authorityEpoch,
		RuntimeTimestampMS: runtimeTS,
		WallClockMS:        wallTS,
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "stale_epoch_reject",
		EmittedBy:          "RK-24",
		Reason:             "authority_epoch_mismatch",
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return controlplane.DecisionOutcome{}, eventabi.ControlSignal{}, err
	}

	return outcome, sig, nil
}

func buildMigrationSignal(in MigrationInput, signal, emitter, reason string, offset int64) (eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transport := safeNonNegativeGuard(in.TransportSequence + offset)
	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		EventID:            fmt.Sprintf("%s-%d", in.EventIDBase, offset),
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transport,
		RuntimeSequence:    safeNonNegativeGuard(in.RuntimeSequence + offset),
		AuthorityEpoch:     safeNonNegativeGuard(in.AuthorityEpoch),
		RuntimeTimestampMS: safeNonNegativeGuard(in.RuntimeTimestampMS + offset),
		WallClockMS:        safeNonNegativeGuard(in.WallClockTimestampMS + offset),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             signal,
		EmittedBy:          emitter,
		Reason:             reason,
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}

func safeNonNegativeGuard(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
