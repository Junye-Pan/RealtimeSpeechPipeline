package executor

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
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
}

// SchedulingDecision reports deterministic allow/shed outcomes at scheduling points.
type SchedulingDecision struct {
	Allowed       bool
	Outcome       *controlplane.DecisionOutcome
	ControlSignal *eventabi.ControlSignal
}

// Scheduler is a minimal RK-07 execution-path stub wired to RK-25 local admission.
type Scheduler struct {
	admission localadmission.Evaluator
}

func NewScheduler(admission localadmission.Evaluator) Scheduler {
	return Scheduler{admission: admission}
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
		return SchedulingDecision{Allowed: true}, nil
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

	pipelineVersion := in.PipelineVersion
	if pipelineVersion == "" {
		pipelineVersion = "pipeline-v1"
	}

	transportSequence := in.TransportSequence
	if transportSequence < 0 {
		transportSequence = 0
	}

	runtimeSequence := in.RuntimeSequence
	if runtimeSequence < 0 {
		runtimeSequence = 0
	}

	authorityEpoch := in.AuthorityEpoch
	if authorityEpoch < 0 {
		authorityEpoch = 0
	}

	control := &eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    pipelineVersion,
		EventID:            in.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transportSequence,
		RuntimeSequence:    runtimeSequence,
		AuthorityEpoch:     authorityEpoch,
		RuntimeTimestampMS: in.RuntimeTimestampMS,
		WallClockMS:        in.WallClockTimestampMS,
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
