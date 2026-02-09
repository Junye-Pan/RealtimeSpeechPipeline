package executor

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
)

// SchedulingInput captures runtime scheduling-point context.
type SchedulingInput struct {
	SessionID            string
	TurnID               string
	EventID              string
	PipelineVersion      string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	Shed                 bool
	Reason               string
	ProviderInvocation   *ProviderInvocationInput
}

// ProviderInvocationInput supplies optional RK-11 invocation context.
type ProviderInvocationInput struct {
	Modality               contracts.Modality
	PreferredProvider      string
	AllowedAdaptiveActions []string
	ProviderInvocationID   string
	CancelRequested        bool
}

// SchedulingDecision reports deterministic allow/shed outcomes at scheduling points.
type SchedulingDecision struct {
	Allowed       bool
	Outcome       *controlplane.DecisionOutcome
	ControlSignal *eventabi.ControlSignal
	Provider      *ProviderDecision
}

// ProviderDecision captures RK-11 invocation outputs for the scheduling point.
type ProviderDecision struct {
	ProviderInvocationID string
	Modality             contracts.Modality
	SelectedProvider     string
	OutcomeClass         contracts.OutcomeClass
	Retryable            bool
	RetryDecision        string
	Attempts             int
	Signals              []eventabi.ControlSignal
}

// ToInvocationOutcomeEvidence maps provider decision output into OR-02 evidence shape.
func (d ProviderDecision) ToInvocationOutcomeEvidence() timeline.InvocationOutcomeEvidence {
	retryDecision := d.RetryDecision
	if retryDecision == "" {
		retryDecision = "none"
	}
	modality := string(d.Modality)
	if modality == "" {
		modality = "external"
	}
	attempts := d.Attempts
	if attempts < 1 {
		attempts = 1
	}
	return timeline.InvocationOutcomeEvidence{
		ProviderInvocationID: d.ProviderInvocationID,
		Modality:             modality,
		ProviderID:           d.SelectedProvider,
		OutcomeClass:         string(d.OutcomeClass),
		Retryable:            d.Retryable,
		RetryDecision:        retryDecision,
		AttemptCount:         attempts,
	}
}

// ProviderInvoker defines the scheduler-to-provider invocation seam.
type ProviderInvoker interface {
	Invoke(in invocation.InvocationInput) (invocation.InvocationResult, error)
}

// Scheduler is a minimal RK-07 execution-path stub wired to RK-25 local admission.
type Scheduler struct {
	admission       localadmission.Evaluator
	providerInvoker ProviderInvoker
}

func NewScheduler(admission localadmission.Evaluator) Scheduler {
	return Scheduler{admission: admission}
}

// NewSchedulerWithProviderInvoker wires optional RK-11 provider invocation support.
func NewSchedulerWithProviderInvoker(admission localadmission.Evaluator, providerInvoker ProviderInvoker) Scheduler {
	return Scheduler{
		admission:       admission,
		providerInvoker: providerInvoker,
	}
}

// EdgeEnqueue applies deterministic admission enforcement at edge enqueue.
func (s Scheduler) EdgeEnqueue(in SchedulingInput) (SchedulingDecision, error) {
	return s.evaluate(controlplane.ScopeEdgeEnqueue, in)
}

// EdgeDequeue applies deterministic admission enforcement at edge dequeue.
func (s Scheduler) EdgeDequeue(in SchedulingInput) (SchedulingDecision, error) {
	return s.evaluate(controlplane.ScopeEdgeDequeue, in)
}

// NodeDispatch applies deterministic admission enforcement at node dispatch.
func (s Scheduler) NodeDispatch(in SchedulingInput) (SchedulingDecision, error) {
	return s.evaluate(controlplane.ScopeNodeDispatch, in)
}

func (s Scheduler) evaluate(scope controlplane.OutcomeScope, in SchedulingInput) (SchedulingDecision, error) {
	result := s.admission.EvaluateSchedulingPoint(localadmission.SchedulingPointInput{
		SessionID:            in.SessionID,
		TurnID:               in.TurnID,
		EventID:              in.EventID,
		RuntimeTimestampMS:   in.RuntimeTimestampMS,
		WallClockTimestampMS: in.WallClockTimestampMS,
		Scope:                scope,
		Shed:                 in.Shed,
		Reason:               in.Reason,
	})

	if result.Allowed {
		decision := SchedulingDecision{Allowed: true}
		if in.ProviderInvocation != nil {
			if s.providerInvoker == nil {
				return SchedulingDecision{}, fmt.Errorf("provider invocation requested but provider invoker is not configured")
			}
			invocationResult, err := s.providerInvoker.Invoke(invocation.InvocationInput{
				SessionID:              in.SessionID,
				TurnID:                 in.TurnID,
				PipelineVersion:        defaultPipelineVersion(in.PipelineVersion),
				EventID:                in.EventID,
				Modality:               in.ProviderInvocation.Modality,
				PreferredProvider:      in.ProviderInvocation.PreferredProvider,
				AllowedAdaptiveActions: in.ProviderInvocation.AllowedAdaptiveActions,
				ProviderInvocationID:   in.ProviderInvocation.ProviderInvocationID,
				TransportSequence:      nonNegative(in.TransportSequence),
				RuntimeSequence:        nonNegative(in.RuntimeSequence),
				AuthorityEpoch:         nonNegative(in.AuthorityEpoch),
				RuntimeTimestampMS:     nonNegative(in.RuntimeTimestampMS),
				WallClockTimestampMS:   nonNegative(in.WallClockTimestampMS),
				CancelRequested:        in.ProviderInvocation.CancelRequested,
			})
			if err != nil {
				return SchedulingDecision{}, err
			}

			decision.Provider = &ProviderDecision{
				ProviderInvocationID: invocationResult.ProviderInvocationID,
				Modality:             in.ProviderInvocation.Modality,
				SelectedProvider:     invocationResult.SelectedProvider,
				OutcomeClass:         invocationResult.Outcome.Class,
				Retryable:            invocationResult.Outcome.Retryable,
				RetryDecision:        invocationResult.RetryDecision,
				Attempts:             len(invocationResult.Attempts),
				Signals:              append([]eventabi.ControlSignal(nil), invocationResult.Signals...),
			}
			decision.Allowed = invocationResult.Outcome.Class == contracts.OutcomeSuccess
		}
		return decision, nil
	}

	if result.Outcome == nil {
		return SchedulingDecision{}, fmt.Errorf("scheduling-point denied without outcome")
	}
	if err := result.Outcome.Validate(); err != nil {
		return SchedulingDecision{}, err
	}
	if result.Outcome.OutcomeKind != controlplane.OutcomeShed {
		return SchedulingDecision{}, fmt.Errorf("unexpected scheduling-point outcome kind: %s", result.Outcome.OutcomeKind)
	}

	controlSignal, err := buildShedControlSignal(in, result.Outcome.Reason)
	if err != nil {
		return SchedulingDecision{}, err
	}

	return SchedulingDecision{Allowed: false, Outcome: result.Outcome, ControlSignal: controlSignal}, nil
}

func buildShedControlSignal(in SchedulingInput, reason string) (*eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}

	control := &eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    defaultPipelineVersion(in.PipelineVersion),
		EventID:            in.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  int64Ptr(nonNegative(in.TransportSequence)),
		RuntimeSequence:    nonNegative(in.RuntimeSequence),
		AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
		WallClockMS:        nonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "shed",
		EmittedBy:          "RK-25",
		Reason:             reason,
		Scope:              scope,
	}
	if err := control.Validate(); err != nil {
		return nil, err
	}
	return control, nil
}

func defaultPipelineVersion(version string) string {
	if version == "" {
		return "pipeline-v1"
	}
	return version
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func int64Ptr(v int64) *int64 {
	return &v
}
