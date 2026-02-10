package turnarbiter

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/guard"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/planresolver"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
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
	SnapshotProvenance           controlplane.SnapshotProvenance
	TransportSequence            int64
	RuntimeSequence              int64
	RuntimeTimestampMS           int64
	WallClockTimestampMS         int64
	AuthorityEpoch               int64
	AuthorityRevoked             bool
	CancelAccepted               bool
	ProviderFailure              bool
	ProviderInvocationOutcomes   []timeline.InvocationOutcomeEvidence
	NodeTimeoutOrFailure         bool
	TransportDisconnectOrStall   bool
	BaselineEvidenceAppendFailed bool
	BaselineEvidence             *timeline.BaselineEvidence
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
	admission         localadmission.Evaluator
	guard             guard.Evaluator
	resolver          planresolver.Resolver
	baselineRecorder  *timeline.Recorder
	turnStartResolver TurnStartBundleResolver
}

func New() Arbiter {
	recorder := timeline.NewRecorder(timeline.StageAConfig{
		BaselineCapacity: 512,
		DetailCapacity:   1024,
	})
	return NewWithRecorder(&recorder)
}

// NewWithRecorder wires a runtime baseline recorder used for OR-02 append semantics.
func NewWithRecorder(recorder *timeline.Recorder) Arbiter {
	return NewWithDependencies(recorder, nil)
}

// NewWithDependencies wires deterministic runtime dependencies for testing and seams.
func NewWithDependencies(recorder *timeline.Recorder, turnStartResolver TurnStartBundleResolver) Arbiter {
	if turnStartResolver == nil {
		turnStartResolver = newControlPlaneBundleResolver()
	}
	return Arbiter{
		admission:         localadmission.Evaluator{},
		guard:             guard.Evaluator{},
		resolver:          planresolver.Resolver{},
		baselineRecorder:  recorder,
		turnStartResolver: turnStartResolver,
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

	turnStartBundle, err := a.resolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                in.SessionID,
		TurnID:                   in.TurnID,
		RequestedPipelineVersion: in.PipelineVersion,
		AuthorityEpoch:           in.AuthorityEpoch,
	})
	if err != nil {
		return a.planMaterializationFailure(result, in, "turn_start_bundle_resolution_failed")
	}

	resolvedAuthorityEpoch := in.AuthorityEpoch
	resolvedAuthorityEpochValid := in.AuthorityEpochValid
	resolvedAuthorityAuthorized := in.AuthorityAuthorized

	if turnStartBundle.HasLeaseDecision {
		if turnStartBundle.LeaseAuthorityEpoch > 0 || resolvedAuthorityEpoch == 0 {
			resolvedAuthorityEpoch = turnStartBundle.LeaseAuthorityEpoch
		}
		resolvedAuthorityEpochValid = resolvedAuthorityEpochValid && turnStartBundle.LeaseAuthorityValid
		resolvedAuthorityAuthorized = resolvedAuthorityAuthorized && turnStartBundle.LeaseAuthorityGranted

		leaseGate := a.guard.Evaluate(guard.PreTurnInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			EventID:              in.EventID,
			RuntimeTimestampMS:   in.RuntimeTimestampMS,
			WallClockTimestampMS: in.WallClockTimestampMS,
			AuthorityEpoch:       resolvedAuthorityEpoch,
			AuthorityEpochValid:  resolvedAuthorityEpochValid,
			AuthorityAuthorized:  resolvedAuthorityAuthorized,
		})
		if !leaseGate.Allowed {
			if leaseGate.Outcome == nil {
				return OpenResult{}, fmt.Errorf("lease authority gate denied but no outcome produced")
			}
			if err := leaseGate.Outcome.Validate(); err != nil {
				return OpenResult{}, err
			}

			trigger, err := triggerFromOutcome(leaseGate.Outcome.OutcomeKind)
			if err != nil {
				return OpenResult{}, err
			}

			result.Transitions = append(result.Transitions, controlplane.TurnTransition{
				FromState:     controlplane.TurnOpening,
				Trigger:       trigger,
				ToState:       controlplane.TurnIdle,
				Deterministic: true,
			})
			result.Decision = leaseGate.Outcome
			result.State = controlplane.TurnIdle
			result.Events = append(result.Events, LifecycleEvent{Name: string(leaseGate.Outcome.OutcomeKind), Reason: leaseGate.Outcome.Reason})
			return result, validateOpenTransitions(result.Transitions)
		}
	}

	if turnStartBundle.HasCPAdmissionDecision && turnStartBundle.CPAdmissionOutcomeKind != controlplane.OutcomeAdmit {
		scope := turnStartBundle.CPAdmissionScope
		if scope == "" {
			scope = controlplane.ScopeSession
		}
		outcome := controlplane.DecisionOutcome{
			OutcomeKind:        turnStartBundle.CPAdmissionOutcomeKind,
			Phase:              controlplane.PhasePreTurn,
			Scope:              scope,
			SessionID:          in.SessionID,
			EventID:            in.EventID,
			RuntimeTimestampMS: in.RuntimeTimestampMS,
			WallClockMS:        in.WallClockTimestampMS,
			EmittedBy:          controlplane.EmitterCP05,
			Reason:             turnStartBundle.CPAdmissionReason,
		}
		if err := outcome.Validate(); err != nil {
			return OpenResult{}, err
		}
		trigger, err := triggerFromOutcome(outcome.OutcomeKind)
		if err != nil {
			return OpenResult{}, err
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

	plan, err := a.resolver.Resolve(planresolver.Input{
		TurnID:                 in.TurnID,
		PipelineVersion:        turnStartBundle.PipelineVersion,
		GraphDefinitionRef:     turnStartBundle.GraphDefinitionRef,
		ExecutionProfile:       turnStartBundle.ExecutionProfile,
		AuthorityEpoch:         resolvedAuthorityEpoch,
		SnapshotProvenance:     turnStartBundle.SnapshotProvenance,
		AllowedAdaptiveActions: append([]string(nil), turnStartBundle.AllowedAdaptiveActions...),
		FailMaterialization:    in.PlanShouldFail,
	})
	if err != nil {
		return a.planMaterializationFailure(result, in, "plan_materialization_failed")
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

func (a Arbiter) resolveTurnStartBundle(in TurnStartBundleInput) (TurnStartBundle, error) {
	resolver := a.turnStartResolver
	if resolver == nil {
		resolver = newControlPlaneBundleResolver()
	}
	return resolver.ResolveTurnStartBundle(in)
}

func (a Arbiter) planMaterializationFailure(result OpenResult, in OpenRequest, reason string) (OpenResult, error) {
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
		Reason:             reason,
	}
	if err := outcome.Validate(); err != nil {
		return OpenResult{}, err
	}
	trigger, err := triggerFromOutcome(outcome.OutcomeKind)
	if err != nil {
		return OpenResult{}, err
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
		return a.finalizeTerminal(in, result, "abort", "authority_loss", controlplane.TriggerAbort)
	}

	if in.CancelAccepted {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "cancelled"},
			LifecycleEvent{Name: "close"},
		)
		return a.finalizeTerminal(in, result, "abort", "cancelled", controlplane.TriggerAbort)
	}

	if in.ProviderFailure {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "provider_failure"},
			LifecycleEvent{Name: "close"},
		)
		return a.finalizeTerminal(in, result, "abort", "provider_failure", controlplane.TriggerAbort)
	}

	if in.NodeTimeoutOrFailure {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "node_timeout_or_failure"},
			LifecycleEvent{Name: "close"},
		)
		return a.finalizeTerminal(in, result, "abort", "node_timeout_or_failure", controlplane.TriggerAbort)
	}

	if in.TransportDisconnectOrStall {
		disconnected, err := runtimetransport.BuildConnectionSignal(runtimetransport.ConnectionSignalInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			PipelineVersion:      defaultPipelineVersion(in.PipelineVersion),
			EventID:              in.EventID + "-disconnected",
			Signal:               "disconnected",
			TransportSequence:    in.TransportSequence,
			RuntimeSequence:      in.RuntimeSequence,
			AuthorityEpoch:       in.AuthorityEpoch,
			RuntimeTimestampMS:   in.RuntimeTimestampMS,
			WallClockTimestampMS: in.WallClockTimestampMS,
			Reason:               "transport_disconnect_or_stall",
		})
		if err != nil {
			return ActiveResult{}, err
		}
		stall, err := runtimetransport.BuildConnectionSignal(runtimetransport.ConnectionSignalInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			PipelineVersion:      defaultPipelineVersion(in.PipelineVersion),
			EventID:              in.EventID + "-stall",
			Signal:               "stall",
			TransportSequence:    in.TransportSequence + 1,
			RuntimeSequence:      in.RuntimeSequence + 1,
			AuthorityEpoch:       in.AuthorityEpoch,
			RuntimeTimestampMS:   in.RuntimeTimestampMS + 1,
			WallClockTimestampMS: in.WallClockTimestampMS + 1,
			Reason:               "transport_disconnect_or_stall",
		})
		if err != nil {
			return ActiveResult{}, err
		}

		result.ControlLane = append(result.ControlLane, disconnected, stall)
		result.Events = append(result.Events,
			LifecycleEvent{Name: "disconnected", Reason: "transport_disconnect_or_stall"},
			LifecycleEvent{Name: "stall", Reason: "transport_disconnect_or_stall"},
			LifecycleEvent{Name: "abort", Reason: "transport_disconnect_or_stall"},
			LifecycleEvent{Name: "close"},
		)
		return a.finalizeTerminal(in, result, "abort", "transport_disconnect_or_stall", controlplane.TriggerAbort)
	}

	if in.BaselineEvidenceAppendFailed {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "recording_evidence_unavailable"},
			LifecycleEvent{Name: "close"},
		)
		return a.finalizeTerminal(in, result, "abort", "recording_evidence_unavailable", controlplane.TriggerAbort)
	}

	if in.NoLegalContinueOrFallback {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "abort", Reason: "deterministic_reason"},
			LifecycleEvent{Name: "close"},
		)
		return a.finalizeTerminal(in, result, "abort", "deterministic_reason", controlplane.TriggerAbort)
	}

	if in.TerminalSuccessReady {
		result.Events = append(result.Events,
			LifecycleEvent{Name: "commit"},
			LifecycleEvent{Name: "close"},
		)
		return a.finalizeTerminal(in, result, "commit", "", controlplane.TriggerCommit)
	}

	return result, nil
}

func (a Arbiter) finalizeTerminal(in ActiveInput, result ActiveResult, terminalOutcome string, terminalReason string, trigger controlplane.TransitionTrigger) (ActiveResult, error) {
	result.State = controlplane.TurnClosed
	appendTerminalTransitions(&result, trigger)

	if err := a.appendBaselineEvidence(in, terminalOutcome, terminalReason); err != nil {
		result.Decision = nil
		result.Events = []LifecycleEvent{
			{Name: "abort", Reason: "recording_evidence_unavailable"},
			{Name: "close"},
		}
		result.Transitions = nil
		appendTerminalTransitions(&result, controlplane.TriggerAbort)
	}

	return result, validateActiveResult(result)
}

func (a Arbiter) appendBaselineEvidence(in ActiveInput, terminalOutcome string, terminalReason string) error {
	if in.BaselineEvidenceAppendFailed {
		return timeline.ErrBaselineCapacityExhausted
	}
	if a.baselineRecorder == nil {
		return fmt.Errorf("timeline recorder unavailable")
	}

	turnStartBundle, err := a.resolveTurnStartBundle(TurnStartBundleInput{
		SessionID:                in.SessionID,
		TurnID:                   in.TurnID,
		RequestedPipelineVersion: in.PipelineVersion,
		AuthorityEpoch:           in.AuthorityEpoch,
	})
	if err != nil {
		return err
	}

	if in.PipelineVersion == "" {
		in.PipelineVersion = turnStartBundle.PipelineVersion
	}
	if err := in.SnapshotProvenance.Validate(); err != nil {
		in.SnapshotProvenance = turnStartBundle.SnapshotProvenance
	}
	if len(in.ProviderInvocationOutcomes) == 0 {
		attempts := a.baselineRecorder.ProviderAttemptEntriesForTurn(in.SessionID, in.TurnID)
		synthesized, err := timeline.InvocationOutcomesFromProviderAttempts(attempts)
		if err != nil {
			return err
		}
		if len(synthesized) > 0 {
			in.ProviderInvocationOutcomes = synthesized
		}
	}

	evidence, err := buildBaselineEvidence(in, terminalOutcome, terminalReason, turnStartBundle.SnapshotProvenance)
	if err != nil {
		return err
	}
	return a.baselineRecorder.AppendBaseline(evidence)
}

func buildBaselineEvidence(in ActiveInput, terminalOutcome string, terminalReason string, snapshotDefaults controlplane.SnapshotProvenance) (timeline.BaselineEvidence, error) {
	eventID := in.EventID
	if eventID == "" {
		eventID = "evt-runtime-terminal"
	}
	pipelineVersion := defaultPipelineVersion(in.PipelineVersion)
	if err := snapshotDefaults.Validate(); err != nil {
		snapshotDefaults = defaultSnapshotProvenance()
	}
	snapshotProvenance := in.SnapshotProvenance
	if err := snapshotProvenance.Validate(); err != nil {
		snapshotProvenance = snapshotDefaults
	}

	decision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		EventID:            eventID + "-admit",
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
		WallClockMS:        nonNegative(in.WallClockTimestampMS),
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
	evidence := timeline.BaselineEvidence{
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    pipelineVersion,
		EventID:            eventID,
		EnvelopeSnapshot:   "eventabi/v1",
		PayloadTags:        []eventabi.PayloadClass{eventabi.PayloadMetadata},
		PlanHash:           "plan/" + fallback(in.TurnID, "unknown"),
		SnapshotProvenance: snapshotProvenance,
		DecisionOutcomes:   []controlplane.DecisionOutcome{decision},
		InvocationOutcomes: append([]timeline.InvocationOutcomeEvidence(nil), in.ProviderInvocationOutcomes...),
		DeterminismSeed:    nonNegative(in.RuntimeSequence),
		OrderingMarkers: []string{
			fmt.Sprintf("runtime_sequence:%d", nonNegative(in.RuntimeSequence)),
		},
		MergeRuleID:      "merge/default",
		MergeRuleVersion: "v1.0",
		AuthorityEpoch:   nonNegative(in.AuthorityEpoch),
		CloseEmitted:     true,
	}

	if in.BaselineEvidence != nil {
		evidence = *in.BaselineEvidence
	}

	if len(evidence.InvocationOutcomes) == 0 && len(in.ProviderInvocationOutcomes) > 0 {
		evidence.InvocationOutcomes = append([]timeline.InvocationOutcomeEvidence(nil), in.ProviderInvocationOutcomes...)
	}

	if in.ProviderFailure && len(evidence.InvocationOutcomes) == 0 {
		evidence.InvocationOutcomes = []timeline.InvocationOutcomeEvidence{
			{
				ProviderInvocationID: "pvi/" + fallback(in.TurnID, "unknown"),
				Modality:             "external",
				ProviderID:           "provider-unknown",
				OutcomeClass:         "infrastructure_failure",
				Retryable:            false,
				RetryDecision:        "none",
				AttemptCount:         1,
			},
		}
	}

	if err := normalizeBaselineEvidence(&evidence, in, terminalOutcome, terminalReason, snapshotDefaults); err != nil {
		return timeline.BaselineEvidence{}, err
	}
	if err := evidence.ValidateCompleteness(); err != nil {
		return timeline.BaselineEvidence{}, err
	}
	return evidence, nil
}

func normalizeBaselineEvidence(evidence *timeline.BaselineEvidence, in ActiveInput, terminalOutcome string, terminalReason string, snapshotDefaults controlplane.SnapshotProvenance) error {
	if err := snapshotDefaults.Validate(); err != nil {
		snapshotDefaults = defaultSnapshotProvenance()
	}

	evidence.SessionID = fallback(evidence.SessionID, in.SessionID)
	evidence.TurnID = fallback(evidence.TurnID, in.TurnID)
	evidence.PipelineVersion = fallback(evidence.PipelineVersion, defaultPipelineVersion(in.PipelineVersion))
	evidence.EventID = fallback(evidence.EventID, fallback(in.EventID, "evt-runtime-terminal"))
	evidence.EnvelopeSnapshot = fallback(evidence.EnvelopeSnapshot, "eventabi/v1")
	if len(evidence.PayloadTags) == 0 {
		evidence.PayloadTags = []eventabi.PayloadClass{eventabi.PayloadMetadata}
	}
	decisions, err := timeline.EnsureOR02RedactionDecisions("L0", evidence.PayloadTags, evidence.RedactionDecisions)
	if err != nil {
		return err
	}
	evidence.RedactionDecisions = decisions
	evidence.PlanHash = fallback(evidence.PlanHash, "plan/"+fallback(in.TurnID, "unknown"))

	evidence.SnapshotProvenance.RoutingViewSnapshot = fallback(evidence.SnapshotProvenance.RoutingViewSnapshot, snapshotDefaults.RoutingViewSnapshot)
	evidence.SnapshotProvenance.AdmissionPolicySnapshot = fallback(evidence.SnapshotProvenance.AdmissionPolicySnapshot, snapshotDefaults.AdmissionPolicySnapshot)
	evidence.SnapshotProvenance.ABICompatibilitySnapshot = fallback(evidence.SnapshotProvenance.ABICompatibilitySnapshot, snapshotDefaults.ABICompatibilitySnapshot)
	evidence.SnapshotProvenance.VersionResolutionSnapshot = fallback(evidence.SnapshotProvenance.VersionResolutionSnapshot, snapshotDefaults.VersionResolutionSnapshot)
	evidence.SnapshotProvenance.PolicyResolutionSnapshot = fallback(evidence.SnapshotProvenance.PolicyResolutionSnapshot, snapshotDefaults.PolicyResolutionSnapshot)
	evidence.SnapshotProvenance.ProviderHealthSnapshot = fallback(evidence.SnapshotProvenance.ProviderHealthSnapshot, snapshotDefaults.ProviderHealthSnapshot)

	if len(evidence.DecisionOutcomes) == 0 {
		decision := controlplane.DecisionOutcome{
			OutcomeKind:        controlplane.OutcomeAdmit,
			Phase:              controlplane.PhasePreTurn,
			Scope:              controlplane.ScopeTurn,
			SessionID:          evidence.SessionID,
			TurnID:             evidence.TurnID,
			EventID:            evidence.EventID + "-admit",
			RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
			WallClockMS:        nonNegative(in.WallClockTimestampMS),
			EmittedBy:          controlplane.EmitterRK25,
			Reason:             "admission_capacity_allow",
		}
		evidence.DecisionOutcomes = []controlplane.DecisionOutcome{decision}
	}

	evidence.OrderingMarkers = sanitizeOrderingMarkers(evidence.OrderingMarkers)
	if len(evidence.OrderingMarkers) == 0 {
		evidence.OrderingMarkers = []string{fmt.Sprintf("runtime_sequence:%d", nonNegative(in.RuntimeSequence))}
	}
	evidence.MergeRuleID = fallback(evidence.MergeRuleID, "merge/default")
	evidence.MergeRuleVersion = fallback(evidence.MergeRuleVersion, "v1.0")
	if evidence.AuthorityEpoch < 0 {
		evidence.AuthorityEpoch = 0
	}
	if evidence.AuthorityEpoch == 0 && in.AuthorityEpoch > 0 {
		evidence.AuthorityEpoch = in.AuthorityEpoch
	}

	proposed := nonNegative(in.RuntimeTimestampMS) - 1
	if proposed < 0 {
		proposed = 0
	}
	runtimeTs := nonNegative(in.RuntimeTimestampMS)
	if evidence.TurnOpenProposedAtMS == nil {
		evidence.TurnOpenProposedAtMS = &proposed
	}
	if evidence.TurnOpenAtMS == nil {
		evidence.TurnOpenAtMS = &runtimeTs
	}

	if in.CancelAccepted && evidence.CancelAcceptedAtMS == nil {
		cancelAccepted := runtimeTs
		evidence.CancelAcceptedAtMS = &cancelAccepted
	}
	if evidence.CancelAcceptedAtMS != nil && evidence.CancelFenceAppliedAtMS == nil {
		cancelFence := *evidence.CancelAcceptedAtMS + 1
		evidence.CancelFenceAppliedAtMS = &cancelFence
	}
	if evidence.CancelAcceptedAtMS != nil && evidence.CancelSentAtMS == nil {
		cancelSent := *evidence.CancelAcceptedAtMS
		evidence.CancelSentAtMS = &cancelSent
	}
	if evidence.CancelAcceptedAtMS != nil && evidence.CancelAckAtMS == nil {
		cancelAck := *evidence.CancelFenceAppliedAtMS
		evidence.CancelAckAtMS = &cancelAck
	}

	evidence.TerminalOutcome = terminalOutcome
	evidence.TerminalReason = terminalReason
	evidence.CloseEmitted = true
	return nil
}

func sanitizeOrderingMarkers(markers []string) []string {
	if len(markers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(markers))
	out := make([]string, 0, len(markers))
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		if _, ok := seen[marker]; ok {
			continue
		}
		seen[marker] = struct{}{}
		out = append(out, marker)
	}
	return out
}

func fallback(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
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

func defaultPipelineVersion(version string) string {
	if version == "" {
		return "pipeline-v1"
	}
	return version
}
