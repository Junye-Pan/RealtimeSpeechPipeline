package localadmission

import "github.com/tiger/realtime-speech-pipeline/api/controlplane"

// CapacityDisposition controls deterministic RK-25 admission behavior.
type CapacityDisposition string

const (
	CapacityAllow  CapacityDisposition = "allow"
	CapacityDefer  CapacityDisposition = "defer"
	CapacityReject CapacityDisposition = "reject"
)

// PreTurnInput contains deterministic admission inputs before authority checks.
type PreTurnInput struct {
	SessionID             string
	TurnID                string
	EventID               string
	RuntimeTimestampMS    int64
	WallClockTimestampMS  int64
	SnapshotValid         bool
	SnapshotFailurePolicy controlplane.OutcomeKind // reject|defer
	CapacityDisposition   CapacityDisposition
}

// PreTurnResult includes either allow or deterministic RK-25 outcome.
type PreTurnResult struct {
	Allowed bool
	Outcome *controlplane.DecisionOutcome
}

// SchedulingPointInput models RK-25 scheduling-point decisions.
type SchedulingPointInput struct {
	SessionID            string
	TurnID               string
	EventID              string
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	Scope                controlplane.OutcomeScope // edge_enqueue|edge_dequeue|node_dispatch
	Shed                 bool
	Reason               string
}

// SchedulingPointResult includes either allow or a shed outcome.
type SchedulingPointResult struct {
	Allowed bool
	Outcome *controlplane.DecisionOutcome
}

// Evaluator implements RK-25 local admission and scheduling enforcement.
type Evaluator struct{}

func (Evaluator) EvaluatePreTurn(in PreTurnInput) PreTurnResult {
	if !in.SnapshotValid {
		kind := in.SnapshotFailurePolicy
		if kind != controlplane.OutcomeReject {
			kind = controlplane.OutcomeDefer
		}
		scope := controlplane.ScopeSession
		if in.TurnID != "" {
			scope = controlplane.ScopeTurn
		}
		outcome := controlplane.DecisionOutcome{
			OutcomeKind:        kind,
			Phase:              controlplane.PhasePreTurn,
			Scope:              scope,
			SessionID:          in.SessionID,
			TurnID:             in.TurnID,
			EventID:            in.EventID,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "snapshot_invalid_or_missing",
		}
		return PreTurnResult{Allowed: false, Outcome: &outcome}
	}

	switch in.CapacityDisposition {
	case CapacityReject:
		scope := controlplane.ScopeSession
		if in.TurnID != "" {
			scope = controlplane.ScopeTurn
		}
		outcome := controlplane.DecisionOutcome{
			OutcomeKind:        controlplane.OutcomeReject,
			Phase:              controlplane.PhasePreTurn,
			Scope:              scope,
			SessionID:          in.SessionID,
			TurnID:             in.TurnID,
			EventID:            in.EventID,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_reject",
		}
		return PreTurnResult{Allowed: false, Outcome: &outcome}
	case CapacityDefer:
		scope := controlplane.ScopeSession
		if in.TurnID != "" {
			scope = controlplane.ScopeTurn
		}
		outcome := controlplane.DecisionOutcome{
			OutcomeKind:        controlplane.OutcomeDefer,
			Phase:              controlplane.PhasePreTurn,
			Scope:              scope,
			SessionID:          in.SessionID,
			TurnID:             in.TurnID,
			EventID:            in.EventID,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_defer",
		}
		return PreTurnResult{Allowed: false, Outcome: &outcome}
	default:
		return PreTurnResult{Allowed: true}
	}
}

func (Evaluator) EvaluateSchedulingPoint(in SchedulingPointInput) SchedulingPointResult {
	if !in.Shed {
		return SchedulingPointResult{Allowed: true}
	}

	reason := in.Reason
	if reason == "" {
		reason = "scheduling_point_shed"
	}
	scope := in.Scope
	if scope != controlplane.ScopeEdgeEnqueue && scope != controlplane.ScopeEdgeDequeue && scope != controlplane.ScopeNodeDispatch {
		scope = controlplane.ScopeEdgeEnqueue
	}

	outcome := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeShed,
		Phase:              controlplane.PhaseScheduling,
		Scope:              scope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		EventID:            in.EventID,
		RuntimeTimestampMS: in.RuntimeTimestampMS,
		WallClockMS:        in.WallClockTimestampMS,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             reason,
	}
	return SchedulingPointResult{Allowed: false, Outcome: &outcome}
}
