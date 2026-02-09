package turnarbiter

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/guard"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/planresolver"
)

// LifecycleEvent is a compact, ordered event emitted by the arbiter.
type LifecycleEvent struct {
	Name   string
	Reason string
}

// OpenRequest drives Idle -> Opening -> (Idle|Active) resolution.
type OpenRequest struct {
	SessionID             string
	TurnID                string
	EventID               string
	RuntimeTimestampMS    int64
	WallClockTimestampMS  int64
	PipelineVersion       string
	AuthorityEpoch        int64
	SnapshotValid         bool
	CapacityDisposition   localadmission.CapacityDisposition
	AuthorityEpochValid   bool
	AuthorityAuthorized   bool
	SnapshotFailurePolicy controlplane.OutcomeKind
	PlanFailurePolicy     controlplane.OutcomeKind
	PlanShouldFail        bool
}

// OpenResult includes deterministic outputs and transitions.
type OpenResult struct {
	State       controlplane.TurnState
	Transitions []controlplane.TurnTransition
	Decision    *controlplane.DecisionOutcome
	Plan        *controlplane.ResolvedTurnPlan
	Events      []LifecycleEvent
	ControlLane []eventabi.ControlSignal
}

// ActiveInput drives Active turn handling with precedence rules.
type ActiveInput struct {
	SessionID                    string
	TurnID                       string
	EventID                      string
	PipelineVersion              string
	TransportSequence            int64
	RuntimeSequence              int64
	RuntimeTimestampMS           int64
	WallClockTimestampMS         int64
	AuthorityEpoch               int64
	AuthorityRevoked             bool
	CancelAccepted               bool
	BaselineEvidenceAppendFailed bool
	NoLegalContinueOrFallback    bool
	TerminalSuccessReady         bool
}

// ActiveResult returns ordered terminal outputs when a terminal path is selected.
type ActiveResult struct {
	State       controlplane.TurnState
	Transitions []controlplane.TurnTransition
	Decision    *controlplane.DecisionOutcome
	Events      []LifecycleEvent
	ControlLane []eventabi.ControlSignal
}

// ApplyInput provides a unified dispatch interface for deterministic arbiter operations.
type ApplyInput struct {
	Open   *OpenRequest
	Active *ActiveInput
}

// ApplyResult is the unified result for TurnArbiter.Apply.
type ApplyResult struct {
	Open   *OpenResult
	Active *ActiveResult
}

// Arbiter composes RK-24/RK-25 guard checks and RK-04 plan resolution.
type Arbiter struct {
	admission localadmission.Evaluator
	guard     guard.Evaluator
	resolver  planresolver.Resolver
}

func New() Arbiter {
	return Arbiter{
		admission: localadmission.Evaluator{},
		guard:     guard.Evaluator{},
		resolver:  planresolver.Resolver{},
	}
}

// Apply dispatches either pre-turn or active-turn handling.
func (a Arbiter) Apply(in ApplyInput) (ApplyResult, error) {
	if in.Open != nil && in.Active != nil {
		return ApplyResult{}, fmt.Errorf("apply input must set only one of Open or Active")
	}
	if in.Open == nil && in.Active == nil {
		return ApplyResult{}, fmt.Errorf("apply input must set Open or Active")
	}

	if in.Open != nil {
		out, err := a.HandleTurnOpenProposed(*in.Open)
		if err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Open: &out}, nil
	}

	out, err := a.HandleActive(*in.Active)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Active: &out}, nil
}

// HandleTurnOpenProposed executes deterministic pre-turn gating and plan freeze.
func (a Arbiter) HandleTurnOpenProposed(in OpenRequest) (OpenResult, error) {
	result := OpenResult{State: controlplane.TurnOpening}

	result.Transitions = append(result.Transitions, controlplane.TurnTransition{
		FromState:     controlplane.TurnIdle,
		Trigger:       controlplane.TriggerTurnOpenProposed,
		ToState:       controlplane.TurnOpening,
		Deterministic: true,
	})

	admission := a.admission.EvaluatePreTurn(localadmission.PreTurnInput{
		SessionID:             in.SessionID,
		TurnID:                in.TurnID,
		EventID:               in.EventID,
		RuntimeTimestampMS:    in.RuntimeTimestampMS,
		WallClockTimestampMS:  in.WallClockTimestampMS,
		SnapshotValid:         in.SnapshotValid,
		CapacityDisposition:   in.CapacityDisposition,
		SnapshotFailurePolicy: in.SnapshotFailurePolicy,
	})

	if !admission.Allowed {
		if admission.Outcome == nil {
			return OpenResult{}, fmt.Errorf("admission denied but no outcome produced")
		}
		if err := admission.Outcome.Validate(); err != nil {
			return OpenResult{}, err
		}

		trigger, err := triggerFromOutcome(admission.Outcome.OutcomeKind)
		if err != nil {
			return OpenResult{}, err
		}

		result.Transitions = append(result.Transitions, controlplane.TurnTransition{
			FromState:     controlplane.TurnOpening,
			Trigger:       trigger,
			ToState:       controlplane.TurnIdle,
			Deterministic: true,
		})
		result.Decision = admission.Outcome
		result.State = controlplane.TurnIdle
		result.Events = append(result.Events, LifecycleEvent{Name: string(admission.Outcome.OutcomeKind), Reason: admission.Outcome.Reason})
		return result, validateOpenTransitions(result.Transitions)
	}

	gate := a.guard.Evaluate(guard.PreTurnInput{
		SessionID:            in.SessionID,
		TurnID:               in.TurnID,
		EventID:              in.EventID,
		RuntimeTimestampMS:   in.RuntimeTimestampMS,
		WallClockTimestampMS: in.WallClockTimestampMS,
		AuthorityEpoch:       in.AuthorityEpoch,
		AuthorityEpochValid:  in.AuthorityEpochValid,
		AuthorityAuthorized:  in.AuthorityAuthorized,
	})

	if !gate.Allowed {
		if gate.Outcome == nil {
			return OpenResult{}, fmt.Errorf("authority gate denied but no outcome produced")
		}
		if err := gate.Outcome.Validate(); err != nil {
			return OpenResult{}, err
		}

		trigger, err := triggerFromOutcome(gate.Outcome.OutcomeKind)
		if err != nil {
			return OpenResult{}, err
		}

		result.Transitions = append(result.Transitions, controlplane.TurnTransition{
			FromState:     controlplane.TurnOpening,
			Trigger:       trigger,
			ToState:       controlplane.TurnIdle,
			Deterministic: true,
		})
		result.Decision = gate.Outcome
		result.State = controlplane.TurnIdle
		result.Events = append(result.Events, LifecycleEvent{Name: string(gate.Outcome.OutcomeKind), Reason: gate.Outcome.Reason})
		return result, validateOpenTransitions(result.Transitions)
	}

	plan, err := a.resolver.Resolve(planresolver.Input{
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     in.AuthorityEpoch,
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		AllowedAdaptiveActions: []string{},
		FailMaterialization:    in.PlanShouldFail,
	})
	if err != nil {
		kind := in.PlanFailurePolicy
		if kind != controlplane.OutcomeReject {
			kind = controlplane.OutcomeDefer
		}
		outcome := controlplane.DecisionOutcome{
			OutcomeKind:        kind,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeTurn,
			SessionID:          in.SessionID,
			TurnID:             in.TurnID,
			EventID:            in.EventID,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "plan_materialization_failed",
		}
		if vErr := outcome.Validate(); vErr != nil {
			return OpenResult{}, vErr
		}
		trigger, trigErr := triggerFromOutcome(outcome.OutcomeKind)
		if trigErr != nil {
			return OpenResult{}, trigErr
		}
		result.Transitions = append(result.Transitions, controlplane.TurnTransition{
			FromState:     controlplane.TurnOpening,
			Trigger:       trigger,
			ToState:       controlplane.TurnIdle,
			Deterministic: true,
		})
		result.Decision = &outcome
		result.State = controlplane.TurnIdle
		result.Events = append(result.Events, LifecycleEvent{Name: string(outcome.OutcomeKind), Reason: outcome.Reason})
		return result, validateOpenTransitions(result.Transitions)
	}

	if err := plan.Validate(); err != nil {
		return OpenResult{}, err
	}
	result.Transitions = append(result.Transitions, controlplane.TurnTransition{
		FromState:     controlplane.TurnOpening,
		Trigger:       controlplane.TriggerTurnOpen,
		ToState:       controlplane.TurnActive,
		Deterministic: true,
	})
	result.Plan = &plan
	result.State = controlplane.TurnActive
	result.Events = append(result.Events, LifecycleEvent{Name: string(controlplane.TriggerTurnOpen)})

	return result, validateOpenTransitions(result.Transitions)
}

// HandleActive applies deterministic precedence while in Active state.
func (a Arbiter) HandleActive(in ActiveInput) (ActiveResult, error) {
	result := ActiveResult{State: controlplane.TurnActive}

	if in.AuthorityRevoked {
		pipelineVersion := in.PipelineVersion
		if pipelineVersion == "" {
			pipelineVersion = "pipeline-v1"
		}
		transportSequence := in.TransportSequence
		decision := a.guard.ActiveTurnRevokeOutcome(in.SessionID, in.TurnID, in.EventID, in.RuntimeTimestampMS, in.WallClockTimestampMS, in.AuthorityEpoch)
		if err := decision.Validate(); err != nil {
			return ActiveResult{}, err
		}
		result.Decision = &decision
		result.ControlLane = append(result.ControlLane, eventabi.ControlSignal{
			SchemaVersion:      "v1.0",
			EventScope:         eventabi.ScopeTurn,
			Signal:             "deauthorized_drain",
			EmittedBy:          "RK-24",
			SessionID:          in.SessionID,
			TurnID:             in.TurnID,
			PipelineVersion:    pipelineVersion,
			EventID:            in.EventID,
			Lane:               eventabi.LaneControl,
			TransportSequence:  &transportSequence,
			RuntimeSequence:    in.RuntimeSequence,
			AuthorityEpoch:     in.AuthorityEpoch,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			PayloadClass:       eventabi.PayloadMetadata,
			Reason:             decision.Reason,
		})
		result.Events = append(result.Events,
			LifecycleEvent{Name: "deauthorized_drain", Reason: decision.Reason},
			LifecycleEvent{Name: "abort", Reason: "authority_loss"},
			LifecycleEvent{Name: "close"},
		)
		result.State = controlplane.TurnClosed
		appendTerminalTransitions(&result, controlplane.TriggerAbort)
		return result, validateActiveResult(result)
	}

	if in.CancelAccepted {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "cancelled"},
			LifecycleEvent{Name: "close"},
		)
		result.State = controlplane.TurnClosed
		appendTerminalTransitions(&result, controlplane.TriggerAbort)
		return result, validateActiveResult(result)
	}

	if in.BaselineEvidenceAppendFailed {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "recording_evidence_unavailable"},
			LifecycleEvent{Name: "close"},
		)
		result.State = controlplane.TurnClosed
		appendTerminalTransitions(&result, controlplane.TriggerAbort)
		return result, validateActiveResult(result)
	}

	if in.NoLegalContinueOrFallback {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "deterministic_reason"},
			LifecycleEvent{Name: "close"},
		)
		result.State = controlplane.TurnClosed
		appendTerminalTransitions(&result, controlplane.TriggerAbort)
		return result, validateActiveResult(result)
	}

	if in.TerminalSuccessReady {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "commit"},
			LifecycleEvent{Name: "close"},
		)
		result.State = controlplane.TurnClosed
		appendTerminalTransitions(&result, controlplane.TriggerCommit)
		return result, validateActiveResult(result)
	}

	return result, nil
}

func appendTerminalTransitions(result *ActiveResult, terminal controlplane.TransitionTrigger) {
	result.Transitions = append(result.Transitions,
		controlplane.TurnTransition{FromState: controlplane.TurnActive, Trigger: terminal, ToState: controlplane.TurnTerminal, Deterministic: true},
		controlplane.TurnTransition{FromState: controlplane.TurnTerminal, Trigger: controlplane.TriggerClose, ToState: controlplane.TurnClosed, Deterministic: true},
	)
}

func validateOpenTransitions(transitions []controlplane.TurnTransition) error {
	for _, tr := range transitions {
		if err := tr.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateActiveResult(result ActiveResult) error {
	for _, tr := range result.Transitions {
		if err := tr.Validate(); err != nil {
			return err
		}
	}
	for _, sig := range result.ControlLane {
		if err := sig.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func triggerFromOutcome(kind controlplane.OutcomeKind) (controlplane.TransitionTrigger, error) {
	switch kind {
	case controlplane.OutcomeReject:
		return controlplane.TriggerReject, nil
	case controlplane.OutcomeDefer:
		return controlplane.TriggerDefer, nil
	case controlplane.OutcomeStaleEpochReject:
		return controlplane.TriggerStaleEpochReject, nil
	case controlplane.OutcomeDeauthorized:
		return controlplane.TriggerDeauthorized, nil
	default:
		return "", fmt.Errorf("outcome %s is not a pre-turn transition trigger", kind)
	}
}
