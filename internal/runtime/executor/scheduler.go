package executor

import (
	"fmt"
	"strconv"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
	runtimeexecutionpool "github.com/tiger/realtime-speech-pipeline/internal/runtime/executionpool"
	runtimeidentity "github.com/tiger/realtime-speech-pipeline/internal/runtime/identity"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/lanes"
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
	Modality                 contracts.Modality
	PreferredProvider        string
	AllowedAdaptiveActions   []string
	ProviderInvocationID     string
	CancelRequested          bool
	EnableStreaming          bool
	DisableProviderStreaming bool
	StreamHooks              invocation.StreamEventHooks
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
	ProviderInvocationID  string
	Modality              contracts.Modality
	SelectedProvider      string
	OutcomeClass          contracts.OutcomeClass
	Retryable             bool
	RetryDecision         string
	Attempts              int
	FinalAttemptLatencyMS int64
	TotalLatencyMS        int64
	FirstChunkLatencyMS   int64
	ChunkCount            int
	BytesOut              int64
	StreamingUsed         bool
	Signals               []eventabi.ControlSignal
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
		ProviderInvocationID:     d.ProviderInvocationID,
		Modality:                 modality,
		ProviderID:               d.SelectedProvider,
		OutcomeClass:             string(d.OutcomeClass),
		Retryable:                d.Retryable,
		RetryDecision:            retryDecision,
		AttemptCount:             attempts,
		FinalAttemptLatencyMS:    0,
		TotalInvocationLatencyMS: 0,
	}
}

// ProviderInvoker defines the scheduler-to-provider invocation seam.
type ProviderInvoker interface {
	Invoke(in invocation.InvocationInput) (invocation.InvocationResult, error)
}

// ProviderAttemptAppender is the scheduler-to-observability seam for attempt-level evidence.
type ProviderAttemptAppender interface {
	AppendProviderInvocationAttempts([]timeline.ProviderAttemptEvidence) error
}

// ProviderInvocationSnapshotAppender appends optional non-terminal invocation snapshots.
type ProviderInvocationSnapshotAppender interface {
	AppendInvocationSnapshot(timeline.InvocationSnapshotEvidence) error
}

// HandoffEdgeAppender appends orchestration-level handoff evidence.
type HandoffEdgeAppender interface {
	AppendHandoffEdges([]timeline.HandoffEdgeEvidence) error
}

type eventIdentityService interface {
	NewEventContext(sessionID, turnID string) (runtimeidentity.Context, error)
}

type dispatchPool interface {
	Submit(task runtimeexecutionpool.Task) error
}

// Scheduler is a minimal RK-07 execution-path stub wired to RK-25 local admission.
type Scheduler struct {
	admission        localadmission.Evaluator
	providerInvoker  ProviderInvoker
	attemptAppender  ProviderAttemptAppender
	snapshotAppender ProviderInvocationSnapshotAppender
	handoffAppender  HandoffEdgeAppender
	router           lanes.Router
	identity         eventIdentityService
	executionPool    dispatchPool
}

func NewScheduler(admission localadmission.Evaluator) Scheduler {
	return Scheduler{
		admission: admission,
		router:    lanes.NewDefaultRouter(),
		identity:  runtimeidentity.NewService(),
	}
}

// NewSchedulerWithProviderInvoker wires optional RK-11 provider invocation support.
func NewSchedulerWithProviderInvoker(admission localadmission.Evaluator, providerInvoker ProviderInvoker) Scheduler {
	return Scheduler{
		admission:       admission,
		providerInvoker: providerInvoker,
		router:          lanes.NewDefaultRouter(),
		identity:        runtimeidentity.NewService(),
	}
}

// NewSchedulerWithProviderInvokerAndAttemptAppender wires RK-11 invocation and OR-02 attempt recording.
func NewSchedulerWithProviderInvokerAndAttemptAppender(
	admission localadmission.Evaluator,
	providerInvoker ProviderInvoker,
	attemptAppender ProviderAttemptAppender,
) Scheduler {
	return Scheduler{
		admission:        admission,
		providerInvoker:  providerInvoker,
		attemptAppender:  attemptAppender,
		snapshotAppender: toSnapshotAppender(attemptAppender),
		handoffAppender:  toHandoffAppender(attemptAppender),
		router:           lanes.NewDefaultRouter(),
		identity:         runtimeidentity.NewService(),
	}
}

// NewSchedulerWithExecutionPool wires optional RK-26 execution pool support.
func NewSchedulerWithExecutionPool(
	admission localadmission.Evaluator,
	executionPool *runtimeexecutionpool.Manager,
) Scheduler {
	return Scheduler{
		admission:     admission,
		router:        lanes.NewDefaultRouter(),
		identity:      runtimeidentity.NewService(),
		executionPool: executionPool,
	}
}

// NewSchedulerWithDependencies wires explicit scheduler dependencies for advanced runtime paths.
func NewSchedulerWithDependencies(
	admission localadmission.Evaluator,
	providerInvoker ProviderInvoker,
	attemptAppender ProviderAttemptAppender,
	router lanes.Router,
) Scheduler {
	if router == nil {
		defaultRouter := lanes.NewDefaultRouter()
		router = defaultRouter
	}
	identitySvc := runtimeidentity.NewService()
	return Scheduler{
		admission:        admission,
		providerInvoker:  providerInvoker,
		attemptAppender:  attemptAppender,
		snapshotAppender: toSnapshotAppender(attemptAppender),
		handoffAppender:  toHandoffAppender(attemptAppender),
		router:           router,
		identity:         identitySvc,
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
	if in.EventID == "" {
		if s.identity == nil {
			return SchedulingDecision{}, fmt.Errorf("identity service is not configured")
		}
		ctx, err := s.identity.NewEventContext(in.SessionID, in.TurnID)
		if err != nil {
			return SchedulingDecision{}, err
		}
		in.EventID = ctx.EventID
	}

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
	correlation := telemetry.Correlation{
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		EventID:            in.EventID,
		PipelineVersion:    defaultPipelineVersion(in.PipelineVersion),
		AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
		Lane:               string(eventabi.LaneTelemetry),
		EmittedBy:          "OR-01",
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
	}
	telemetry.DefaultEmitter().EmitMetric(
		telemetry.MetricShedRate,
		boolToFloat(in.Shed),
		"ratio",
		map[string]string{
			"scope": string(scope),
		},
		correlation,
	)

	if result.Allowed {
		decision := SchedulingDecision{Allowed: true}
		if in.ProviderInvocation != nil {
			if s.providerInvoker == nil {
				return SchedulingDecision{}, fmt.Errorf("provider invocation requested but provider invoker is not configured")
			}
			invocationResult, err := s.providerInvoker.Invoke(invocation.InvocationInput{
				SessionID:                in.SessionID,
				TurnID:                   in.TurnID,
				PipelineVersion:          defaultPipelineVersion(in.PipelineVersion),
				EventID:                  in.EventID,
				Modality:                 in.ProviderInvocation.Modality,
				PreferredProvider:        in.ProviderInvocation.PreferredProvider,
				AllowedAdaptiveActions:   in.ProviderInvocation.AllowedAdaptiveActions,
				ProviderInvocationID:     in.ProviderInvocation.ProviderInvocationID,
				TransportSequence:        nonNegative(in.TransportSequence),
				RuntimeSequence:          nonNegative(in.RuntimeSequence),
				AuthorityEpoch:           nonNegative(in.AuthorityEpoch),
				RuntimeTimestampMS:       nonNegative(in.RuntimeTimestampMS),
				WallClockTimestampMS:     nonNegative(in.WallClockTimestampMS),
				CancelRequested:          in.ProviderInvocation.CancelRequested,
				EnableStreaming:          in.ProviderInvocation.EnableStreaming,
				DisableProviderStreaming: in.ProviderInvocation.DisableProviderStreaming,
				StreamHooks:              in.ProviderInvocation.StreamHooks,
			})
			if err != nil {
				return SchedulingDecision{}, err
			}
			normalizedSignals, err := runtimeeventabi.ValidateAndNormalizeControlSignals(invocationResult.Signals)
			if err != nil {
				return SchedulingDecision{}, err
			}
			attemptEvidence := buildAttemptEvidence(in, in.ProviderInvocation.Modality, invocationResult)
			finalAttemptLatencyMS := int64(0)
			if len(attemptEvidence) > 0 {
				finalAttemptLatencyMS = attemptEvidence[len(attemptEvidence)-1].AttemptLatencyMS
			}
			telemetry.DefaultEmitter().EmitMetric(
				telemetry.MetricProviderRTTMS,
				float64(finalAttemptLatencyMS),
				"ms",
				map[string]string{
					"scope":       string(scope),
					"modality":    string(in.ProviderInvocation.Modality),
					"attempts":    strconv.Itoa(len(invocationResult.Attempts)),
					"retry_logic": invocationResult.RetryDecision,
				},
				correlation,
			)

			if s.attemptAppender != nil {
				if err := s.attemptAppender.AppendProviderInvocationAttempts(attemptEvidence); err != nil {
					return SchedulingDecision{}, err
				}
			}
			if s.snapshotAppender != nil {
				snapshot, ok, err := buildInvocationSnapshotEvidence(in, invocationResult, attemptEvidence)
				if err != nil {
					return SchedulingDecision{}, err
				}
				if ok {
					if err := s.snapshotAppender.AppendInvocationSnapshot(snapshot); err != nil {
						return SchedulingDecision{}, err
					}
				}
			}

			finalAttemptLatencyMS, totalLatencyMS, firstChunkLatencyMS, totalChunks, bytesOut := summarizeAttempts(invocationResult.Attempts)
			decision.Provider = &ProviderDecision{
				ProviderInvocationID:  invocationResult.ProviderInvocationID,
				Modality:              in.ProviderInvocation.Modality,
				SelectedProvider:      invocationResult.SelectedProvider,
				OutcomeClass:          invocationResult.Outcome.Class,
				Retryable:             invocationResult.Outcome.Retryable,
				RetryDecision:         invocationResult.RetryDecision,
				Attempts:              len(invocationResult.Attempts),
				FinalAttemptLatencyMS: finalAttemptLatencyMS,
				TotalLatencyMS:        totalLatencyMS,
				FirstChunkLatencyMS:   firstChunkLatencyMS,
				ChunkCount:            totalChunks,
				BytesOut:              bytesOut,
				StreamingUsed:         invocationResult.StreamingUsed,
				Signals:               append([]eventabi.ControlSignal(nil), normalizedSignals...),
			}
			decision.Allowed = invocationResult.Outcome.Class == contracts.OutcomeSuccess
		}
		telemetry.DefaultEmitter().EmitSpan(
			"node_span",
			"node_span",
			nonNegative(in.RuntimeTimestampMS),
			nonNegative(in.RuntimeTimestampMS)+1,
			map[string]string{
				"scope":   string(scope),
				"allowed": strconv.FormatBool(decision.Allowed),
			},
			correlation,
		)
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
	telemetry.DefaultEmitter().EmitLog(
		"scheduling_shed",
		"warn",
		"scheduling point shed triggered",
		map[string]string{
			"scope":   string(scope),
			"outcome": string(result.Outcome.OutcomeKind),
			"reason":  result.Outcome.Reason,
		},
		correlation,
	)
	telemetry.DefaultEmitter().EmitSpan(
		"node_span",
		"node_span",
		nonNegative(in.RuntimeTimestampMS),
		nonNegative(in.RuntimeTimestampMS)+1,
		map[string]string{
			"scope":   string(scope),
			"allowed": "false",
			"outcome": string(result.Outcome.OutcomeKind),
		},
		correlation,
	)

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

func boolToFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func buildAttemptEvidence(
	in SchedulingInput,
	modality contracts.Modality,
	result invocation.InvocationResult,
) []timeline.ProviderAttemptEvidence {
	retryDecision := result.RetryDecision
	if retryDecision == "" {
		retryDecision = "none"
	}
	attempts := make([]timeline.ProviderAttemptEvidence, 0, len(result.Attempts))
	var previousWallClockMS int64
	hasPrevious := false
	for idx, attempt := range result.Attempts {
		offset := int64(idx)
		wallClockMS := nonNegative(in.WallClockTimestampMS) + offset
		attemptLatencyMS := nonNegative(attempt.AttemptLatencyMS)
		if attemptLatencyMS > 0 {
			if hasPrevious {
				wallClockMS = previousWallClockMS + attemptLatencyMS
			} else {
				wallClockMS = nonNegative(in.WallClockTimestampMS) + attemptLatencyMS
			}
		} else if hasPrevious && wallClockMS > previousWallClockMS {
			attemptLatencyMS = wallClockMS - previousWallClockMS
		}
		attempts = append(attempts, timeline.ProviderAttemptEvidence{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			PipelineVersion:      defaultPipelineVersion(in.PipelineVersion),
			EventID:              in.EventID,
			ProviderInvocationID: result.ProviderInvocationID,
			Modality:             string(modality),
			ProviderID:           attempt.ProviderID,
			Attempt:              attempt.Attempt,
			OutcomeClass:         string(attempt.Outcome.Class),
			Retryable:            attempt.Outcome.Retryable,
			RetryDecision:        retryDecision,
			AttemptLatencyMS:     attemptLatencyMS,
			TransportSequence:    nonNegative(in.TransportSequence) + offset,
			RuntimeSequence:      nonNegative(in.RuntimeSequence) + offset,
			AuthorityEpoch:       nonNegative(in.AuthorityEpoch),
			RuntimeTimestampMS:   nonNegative(in.RuntimeTimestampMS) + offset,
			WallClockTimestampMS: wallClockMS,
			OutcomeReason:        attempt.Outcome.Reason,
			CircuitOpen:          attempt.Outcome.CircuitOpen,
			BackoffMS:            attempt.Outcome.BackoffMS,
			InputPayload:         attempt.Outcome.InputPayload,
			OutputPayload:        attempt.Outcome.OutputPayload,
			OutputStatusCode:     attempt.Outcome.OutputStatusCode,
			PayloadTruncated:     attempt.Outcome.PayloadTruncated,
			StreamingUsed:        attempt.StreamingUsed,
			ChunkCount:           attempt.ChunkCount,
			BytesOut:             nonNegative(attempt.BytesOut),
			FirstChunkLatencyMS:  nonNegative(attempt.FirstChunkLatencyMS),
		})
		previousWallClockMS = wallClockMS
		hasPrevious = true
	}
	return attempts
}

func toSnapshotAppender(appender ProviderAttemptAppender) ProviderInvocationSnapshotAppender {
	if appender == nil {
		return nil
	}
	snapshotAppender, ok := appender.(ProviderInvocationSnapshotAppender)
	if !ok {
		return nil
	}
	return snapshotAppender
}

func toHandoffAppender(appender ProviderAttemptAppender) HandoffEdgeAppender {
	if appender == nil {
		return nil
	}
	handoffAppender, ok := appender.(HandoffEdgeAppender)
	if !ok {
		return nil
	}
	return handoffAppender
}

func buildInvocationSnapshotEvidence(
	in SchedulingInput,
	result invocation.InvocationResult,
	attemptEvidence []timeline.ProviderAttemptEvidence,
) (timeline.InvocationSnapshotEvidence, bool, error) {
	if len(attemptEvidence) == 0 {
		return timeline.InvocationSnapshotEvidence{}, false, nil
	}

	outcomes, err := timeline.InvocationOutcomesFromProviderAttempts(attemptEvidence)
	if err != nil {
		return timeline.InvocationSnapshotEvidence{}, false, err
	}
	if len(outcomes) == 0 {
		return timeline.InvocationSnapshotEvidence{}, false, nil
	}

	outcome := outcomes[len(outcomes)-1]
	for _, candidate := range outcomes {
		if candidate.ProviderInvocationID == result.ProviderInvocationID {
			outcome = candidate
			break
		}
	}

	finalAttempt := attemptEvidence[len(attemptEvidence)-1]
	runtimeTimestampMS := nonNegative(in.RuntimeTimestampMS)
	if finalAttempt.RuntimeTimestampMS > runtimeTimestampMS {
		runtimeTimestampMS = finalAttempt.RuntimeTimestampMS
	}
	wallClockTimestampMS := nonNegative(in.WallClockTimestampMS)
	if finalAttempt.WallClockTimestampMS > wallClockTimestampMS {
		wallClockTimestampMS = finalAttempt.WallClockTimestampMS
	}

	return timeline.InvocationSnapshotEvidence{
		SessionID:                in.SessionID,
		TurnID:                   in.TurnID,
		PipelineVersion:          defaultPipelineVersion(in.PipelineVersion),
		EventID:                  in.EventID,
		ProviderInvocationID:     outcome.ProviderInvocationID,
		Modality:                 outcome.Modality,
		ProviderID:               outcome.ProviderID,
		OutcomeClass:             outcome.OutcomeClass,
		Retryable:                outcome.Retryable,
		RetryDecision:            outcome.RetryDecision,
		AttemptCount:             outcome.AttemptCount,
		FinalAttemptLatencyMS:    outcome.FinalAttemptLatencyMS,
		TotalInvocationLatencyMS: outcome.TotalInvocationLatencyMS,
		RuntimeTimestampMS:       runtimeTimestampMS,
		WallClockTimestampMS:     wallClockTimestampMS,
	}, true, nil
}

func summarizeAttempts(attempts []invocation.InvocationAttempt) (finalLatencyMS int64, totalLatencyMS int64, firstChunkMS int64, totalChunks int, bytesOut int64) {
	firstChunkSet := false
	for i := range attempts {
		attempt := attempts[i]
		latency := nonNegative(attempt.AttemptLatencyMS)
		totalLatencyMS += latency
		if i == len(attempts)-1 {
			finalLatencyMS = latency
		}
		totalChunks += maxInt(attempt.ChunkCount, 0)
		bytesOut += nonNegative(attempt.BytesOut)
		candidateFirstChunk := nonNegative(attempt.FirstChunkLatencyMS)
		if candidateFirstChunk > 0 && (!firstChunkSet || candidateFirstChunk < firstChunkMS) {
			firstChunkMS = candidateFirstChunk
			firstChunkSet = true
		}
	}
	return finalLatencyMS, totalLatencyMS, firstChunkMS, totalChunks, bytesOut
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
