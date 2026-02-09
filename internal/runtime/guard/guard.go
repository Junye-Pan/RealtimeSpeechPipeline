package guard

import "github.com/tiger/realtime-speech-pipeline/api/controlplane"

// PreTurnInput is the deterministic authority gate input before turn_open.
type PreTurnInput struct {
	SessionID            string
	TurnID               string
	EventID              string
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	AuthorityEpoch       int64
	AuthorityEpochValid  bool
	AuthorityAuthorized  bool
}

// PreTurnResult holds either an allow decision or a deterministic outcome.
type PreTurnResult struct {
	Allowed bool
	Outcome *controlplane.DecisionOutcome
}

// Evaluator implements RK-24 authority validation behavior.
type Evaluator struct{}

// Evaluate is the public RK-24 pre-turn authority evaluation entry point.
func (Evaluator) Evaluate(in PreTurnInput) PreTurnResult {
	return Evaluator{}.EvaluatePreTurn(in)
}

func (Evaluator) EvaluatePreTurn(in PreTurnInput) PreTurnResult {
	if !in.AuthorityEpochValid {
		epoch := in.AuthorityEpoch
		scope := controlplane.ScopeSession
		if in.TurnID != "" {
			scope = controlplane.ScopeTurn
		}
		outcome := controlplane.DecisionOutcome{
			OutcomeKind:        controlplane.OutcomeStaleEpochReject,
			Phase:              controlplane.PhasePreTurn,
			Scope:              scope,
			SessionID:          in.SessionID,
			TurnID:             in.TurnID,
			EventID:            in.EventID,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			EmittedBy:          controlplane.EmitterRK24,
			AuthorityEpoch:     &epoch,
			Reason:             "authority_epoch_mismatch",
		}
		return PreTurnResult{Allowed: false, Outcome: &outcome}
	}

	if !in.AuthorityAuthorized {
		epoch := in.AuthorityEpoch
		scope := controlplane.ScopeSession
		if in.TurnID != "" {
			scope = controlplane.ScopeTurn
		}
		outcome := controlplane.DecisionOutcome{
			OutcomeKind:        controlplane.OutcomeDeauthorized,
			Phase:              controlplane.PhasePreTurn,
			Scope:              scope,
			SessionID:          in.SessionID,
			TurnID:             in.TurnID,
			EventID:            in.EventID,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			EmittedBy:          controlplane.EmitterRK24,
			AuthorityEpoch:     &epoch,
			Reason:             "authority_revoked_before_open",
		}
		return PreTurnResult{Allowed: false, Outcome: &outcome}
	}

	return PreTurnResult{Allowed: true}
}

// ActiveTurnRevokeOutcome builds the in-turn authority-loss decision artifact.
func (Evaluator) ActiveTurnRevokeOutcome(sessionID, turnID, eventID string, runtimeTS, wallTS, authorityEpoch int64) controlplane.DecisionOutcome {
	epoch := authorityEpoch
	return controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeDeauthorized,
		Phase:              controlplane.PhaseActiveTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          sessionID,
		TurnID:             turnID,
		EventID:            eventID,
		RuntimeTimestampMS: runtimeTS,
		WallClockMS:        wallTS,
		EmittedBy:          controlplane.EmitterRK24,
		AuthorityEpoch:     &epoch,
		Reason:             "authority_revoked_in_turn",
	}
}
