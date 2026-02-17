package lanes

import (
	"fmt"
	"sort"
	"sync"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	telemetrycontext "github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry/context"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/buffering"
	runtimeflowcontrol "github.com/tiger/realtime-speech-pipeline/internal/runtime/flowcontrol"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
)

// QueueLimits defines bounded per-lane capacities.
type QueueLimits struct {
	Control   int
	Data      int
	Telemetry int
}

// QueueWatermarks defines high/low queue thresholds per lane.
type QueueWatermarks struct {
	Control   controlplane.WatermarkThreshold
	Data      controlplane.WatermarkThreshold
	Telemetry controlplane.WatermarkThreshold
}

// DispatchSchedulerConfig configures deterministic lane dispatch behavior.
type DispatchSchedulerConfig struct {
	SessionID       string
	TurnID          string
	PipelineVersion string
	AuthorityEpoch  int64
	EdgeIDPrefix    string
	QueueLimits     QueueLimits
	Watermarks      QueueWatermarks
}

// EnqueueInput is a lane-scheduler enqueue request.
type EnqueueInput struct {
	ItemID               string
	Lane                 eventabi.Lane
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	PayloadClass         eventabi.PayloadClass
}

// DispatchItem is one queued lane-dispatch unit.
type DispatchItem struct {
	ItemID               string
	Lane                 eventabi.Lane
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	PayloadClass         eventabi.PayloadClass
}

// EnqueueResult summarizes enqueue admission and pressure outputs.
type EnqueueResult struct {
	Accepted    bool
	Dropped     bool
	QueueDepth  int
	Signals     []eventabi.ControlSignal
	ShedOutcome *controlplane.DecisionOutcome
}

// DispatchResult is the next dispatch item with optional recovery signals.
type DispatchResult struct {
	Item    DispatchItem
	Signals []eventabi.ControlSignal
}

// PriorityScheduler enforces Control > Data > Telemetry dispatch with bounded queues.
type PriorityScheduler struct {
	mu        sync.Mutex
	admission localadmission.Evaluator
	flow      runtimeflowcontrol.Controller
	cfg       DispatchSchedulerConfig
	queues    map[eventabi.Lane][]DispatchItem
	saturated map[eventabi.Lane]bool
}

// ConfigFromResolvedTurnPlan builds scheduler config from an immutable resolved turn plan.
func ConfigFromResolvedTurnPlan(sessionID string, turnID string, plan controlplane.ResolvedTurnPlan) (DispatchSchedulerConfig, error) {
	if sessionID == "" {
		return DispatchSchedulerConfig{}, fmt.Errorf("session_id is required")
	}
	if err := plan.Validate(); err != nil {
		return DispatchSchedulerConfig{}, err
	}

	policy, err := selectPlanEdgePolicy(plan.EdgeBufferPolicies)
	if err != nil {
		return DispatchSchedulerConfig{}, err
	}

	limits := QueueLimits{
		Control:   maxInt(policy.MaxQueueItems, plan.FlowControl.Watermarks.ControlLane.High),
		Data:      maxInt(policy.MaxQueueItems, plan.FlowControl.Watermarks.DataLane.High),
		Telemetry: maxInt(policy.MaxQueueItems, plan.FlowControl.Watermarks.TelemetryLane.High),
	}

	cfg := DispatchSchedulerConfig{
		SessionID:       sessionID,
		TurnID:          turnID,
		PipelineVersion: plan.PipelineVersion,
		AuthorityEpoch:  plan.AuthorityEpoch,
		EdgeIDPrefix:    fmt.Sprintf("plan/%s", plan.PlanHash[:12]),
		QueueLimits:     limits,
		Watermarks: QueueWatermarks{
			Control:   plan.FlowControl.Watermarks.ControlLane,
			Data:      plan.FlowControl.Watermarks.DataLane,
			Telemetry: plan.FlowControl.Watermarks.TelemetryLane,
		},
	}
	return normalizeSchedulerConfig(cfg)
}

// NewPriorityScheduler builds a deterministic lane scheduler.
func NewPriorityScheduler(admission localadmission.Evaluator, cfg DispatchSchedulerConfig) (*PriorityScheduler, error) {
	normalized, err := normalizeSchedulerConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &PriorityScheduler{
		admission: admission,
		flow:      runtimeflowcontrol.NewController(),
		cfg:       normalized,
		queues: map[eventabi.Lane][]DispatchItem{
			eventabi.LaneControl:   {},
			eventabi.LaneData:      {},
			eventabi.LaneTelemetry: {},
		},
		saturated: map[eventabi.Lane]bool{
			eventabi.LaneControl:   false,
			eventabi.LaneData:      false,
			eventabi.LaneTelemetry: false,
		},
	}, nil
}

// Enqueue enqueues one item into the lane queue with bounded/shed/drop handling.
func (s *PriorityScheduler) Enqueue(in EnqueueInput) (EnqueueResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateLane(in.Lane); err != nil {
		return EnqueueResult{}, err
	}
	if in.EventID == "" {
		return EnqueueResult{}, fmt.Errorf("event_id is required")
	}
	correlation, err := telemetrycontext.Resolve(telemetrycontext.ResolveInput{
		SessionID:            s.cfg.SessionID,
		TurnID:               s.cfg.TurnID,
		EventID:              in.EventID,
		PipelineVersion:      s.cfg.PipelineVersion,
		EdgeID:               s.edgeIDForLane(in.Lane),
		AuthorityEpoch:       safeNonNegative(s.cfg.AuthorityEpoch),
		Lane:                 eventabi.LaneTelemetry,
		EmittedBy:            "OR-01",
		RuntimeTimestampMS:   safeNonNegative(in.RuntimeTimestampMS),
		WallClockTimestampMS: safeNonNegative(in.WallClockTimestampMS),
	})
	if err != nil {
		return EnqueueResult{}, err
	}
	if in.ItemID == "" {
		in.ItemID = in.EventID
	}
	item := DispatchItem{
		ItemID:               in.ItemID,
		Lane:                 in.Lane,
		EventID:              in.EventID,
		TransportSequence:    safeNonNegative(in.TransportSequence),
		RuntimeSequence:      safeNonNegative(in.RuntimeSequence),
		RuntimeTimestampMS:   safeNonNegative(in.RuntimeTimestampMS),
		WallClockTimestampMS: safeNonNegative(in.WallClockTimestampMS),
		PayloadClass:         in.PayloadClass,
	}

	queue := s.queues[in.Lane]
	limit := s.limitFor(in.Lane)
	result := EnqueueResult{
		Signals: make([]eventabi.ControlSignal, 0, 4),
	}

	if len(queue) >= limit {
		s.saturated[in.Lane] = true
		result.Dropped = true
		result.QueueDepth = len(queue)
		telemetry.DefaultEmitter().EmitMetric(
			telemetry.MetricEdgeQueueDepth,
			float64(result.QueueDepth),
			"count",
			map[string]string{
				"lane":    string(in.Lane),
				"edge_id": s.edgeIDForLane(in.Lane),
				"status":  "saturated",
			},
			correlation,
		)
		if in.Lane == eventabi.LaneTelemetry {
			dropSignal, err := s.buildDropNotice(in, "telemetry_best_effort_drop")
			if err != nil {
				return EnqueueResult{}, err
			}
			result.Signals = append(result.Signals, dropSignal)
			telemetry.DefaultEmitter().EmitMetric(
				telemetry.MetricEdgeDropsTotal,
				1,
				"count",
				map[string]string{
					"lane":     string(in.Lane),
					"edge_id":  s.edgeIDForLane(in.Lane),
					"strategy": "best_effort_drop",
				},
				correlation,
			)
			return result, nil
		}

		overflowSignals, shedOutcome, err := s.buildOverflowSignalsAndOutcome(in)
		if err != nil {
			return EnqueueResult{}, err
		}
		result.Signals = append(result.Signals, overflowSignals...)
		result.ShedOutcome = shedOutcome
		telemetry.DefaultEmitter().EmitMetric(
			telemetry.MetricEdgeDropsTotal,
			1,
			"count",
			map[string]string{
				"lane":     string(in.Lane),
				"edge_id":  s.edgeIDForLane(in.Lane),
				"strategy": "shed",
			},
			correlation,
		)
		return result, nil
	}

	s.queues[in.Lane] = append(queue, item)
	result.Accepted = true
	result.QueueDepth = len(s.queues[in.Lane])

	if !s.saturated[in.Lane] && result.QueueDepth >= s.highWatermarkFor(in.Lane) {
		s.saturated[in.Lane] = true
		highWatermarkSignals, err := s.buildHighWatermarkSignals(in)
		if err != nil {
			return EnqueueResult{}, err
		}
		result.Signals = append(result.Signals, highWatermarkSignals...)
	}
	telemetry.DefaultEmitter().EmitMetric(
		telemetry.MetricEdgeQueueDepth,
		float64(result.QueueDepth),
		"count",
		map[string]string{
			"lane":    string(in.Lane),
			"edge_id": s.edgeIDForLane(in.Lane),
			"status":  "accepted",
		},
		correlation,
	)

	return result, nil
}

// NextDispatch returns the next item by lane priority with optional recovery signals.
func (s *PriorityScheduler) NextDispatch() (DispatchResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ordered := []eventabi.Lane{eventabi.LaneControl, eventabi.LaneData, eventabi.LaneTelemetry}
	for _, lane := range ordered {
		queue := s.queues[lane]
		if len(queue) == 0 {
			continue
		}
		item := queue[0]
		s.queues[lane] = queue[1:]

		signals := make([]eventabi.ControlSignal, 0, 1)
		if s.saturated[lane] && len(s.queues[lane]) <= s.lowWatermarkFor(lane) {
			recoverySignals, err := s.flow.Evaluate(runtimeflowcontrol.Input{
				SessionID:            s.cfg.SessionID,
				TurnID:               s.cfg.TurnID,
				PipelineVersion:      s.cfg.PipelineVersion,
				EdgeID:               s.edgeIDForLane(lane),
				EventID:              item.EventID + "-flow-recovery",
				TargetLane:           lane,
				TransportSequence:    item.TransportSequence,
				RuntimeSequence:      item.RuntimeSequence,
				AuthorityEpoch:       s.cfg.AuthorityEpoch,
				RuntimeTimestampMS:   item.RuntimeTimestampMS,
				WallClockTimestampMS: item.WallClockTimestampMS,
				EmitRecovery:         true,
				Mode:                 runtimeflowcontrol.ModeSignal,
			})
			if err != nil {
				return DispatchResult{}, false, err
			}
			signals = append(signals, recoverySignals...)
			s.saturated[lane] = false
		}
		correlation, err := telemetrycontext.Resolve(telemetrycontext.ResolveInput{
			SessionID:            s.cfg.SessionID,
			TurnID:               s.cfg.TurnID,
			EventID:              item.EventID,
			PipelineVersion:      s.cfg.PipelineVersion,
			EdgeID:               s.edgeIDForLane(lane),
			AuthorityEpoch:       safeNonNegative(s.cfg.AuthorityEpoch),
			Lane:                 eventabi.LaneTelemetry,
			EmittedBy:            "OR-01",
			RuntimeTimestampMS:   safeNonNegative(item.RuntimeTimestampMS),
			WallClockTimestampMS: safeNonNegative(item.WallClockTimestampMS),
		})
		if err != nil {
			return DispatchResult{}, false, err
		}
		telemetry.DefaultEmitter().EmitMetric(
			telemetry.MetricEdgeQueueDepth,
			float64(len(s.queues[lane])),
			"count",
			map[string]string{
				"lane":    string(lane),
				"edge_id": s.edgeIDForLane(lane),
				"status":  "dequeue",
			},
			correlation,
		)

		return DispatchResult{
			Item:    item,
			Signals: signals,
		}, true, nil
	}

	return DispatchResult{}, false, nil
}

// QueueDepth returns the current queue depth for a lane.
func (s *PriorityScheduler) QueueDepth(lane eventabi.Lane) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queues[lane])
}

func (s *PriorityScheduler) limitFor(lane eventabi.Lane) int {
	switch lane {
	case eventabi.LaneControl:
		return s.cfg.QueueLimits.Control
	case eventabi.LaneData:
		return s.cfg.QueueLimits.Data
	default:
		return s.cfg.QueueLimits.Telemetry
	}
}

func (s *PriorityScheduler) highWatermarkFor(lane eventabi.Lane) int {
	switch lane {
	case eventabi.LaneControl:
		return s.cfg.Watermarks.Control.High
	case eventabi.LaneData:
		return s.cfg.Watermarks.Data.High
	default:
		return s.cfg.Watermarks.Telemetry.High
	}
}

func (s *PriorityScheduler) lowWatermarkFor(lane eventabi.Lane) int {
	switch lane {
	case eventabi.LaneControl:
		return s.cfg.Watermarks.Control.Low
	case eventabi.LaneData:
		return s.cfg.Watermarks.Data.Low
	default:
		return s.cfg.Watermarks.Telemetry.Low
	}
}

func (s *PriorityScheduler) edgeIDForLane(lane eventabi.Lane) string {
	return fmt.Sprintf("%s/%s", s.cfg.EdgeIDPrefix, laneIDFor(lane))
}

func (s *PriorityScheduler) buildDropNotice(in EnqueueInput, reason string) (eventabi.ControlSignal, error) {
	return buffering.BuildDropNotice(buffering.DropNoticeInput{
		SessionID:            s.cfg.SessionID,
		TurnID:               s.cfg.TurnID,
		PipelineVersion:      s.cfg.PipelineVersion,
		EdgeID:               s.edgeIDForLane(in.Lane),
		EventID:              in.EventID + "-drop",
		TransportSequence:    safeNonNegative(in.TransportSequence),
		RuntimeSequence:      safeNonNegative(in.RuntimeSequence),
		AuthorityEpoch:       safeNonNegative(s.cfg.AuthorityEpoch),
		RuntimeTimestampMS:   safeNonNegative(in.RuntimeTimestampMS),
		WallClockTimestampMS: safeNonNegative(in.WallClockTimestampMS),
		TargetLane:           in.Lane,
		RangeStart:           safeNonNegative(in.TransportSequence),
		RangeEnd:             safeNonNegative(in.TransportSequence),
		Reason:               reason,
		EmittedBy:            "RK-12",
	})
}

func (s *PriorityScheduler) buildHighWatermarkSignals(in EnqueueInput) ([]eventabi.ControlSignal, error) {
	watermarkSignal, err := s.buildWatermarkSignal(in, "lane_queue_high_watermark")
	if err != nil {
		return nil, err
	}
	flowSignals, err := s.flow.Evaluate(runtimeflowcontrol.Input{
		SessionID:            s.cfg.SessionID,
		TurnID:               s.cfg.TurnID,
		PipelineVersion:      s.cfg.PipelineVersion,
		EdgeID:               s.edgeIDForLane(in.Lane),
		EventID:              in.EventID + "-flow",
		TargetLane:           in.Lane,
		TransportSequence:    safeNonNegative(in.TransportSequence),
		RuntimeSequence:      safeNonNegative(in.RuntimeSequence),
		AuthorityEpoch:       safeNonNegative(s.cfg.AuthorityEpoch),
		RuntimeTimestampMS:   safeNonNegative(in.RuntimeTimestampMS),
		WallClockTimestampMS: safeNonNegative(in.WallClockTimestampMS),
		HighWatermark:        true,
		Mode:                 runtimeflowcontrol.ModeSignal,
	})
	if err != nil {
		return nil, err
	}
	out := make([]eventabi.ControlSignal, 0, 1+len(flowSignals))
	out = append(out, watermarkSignal)
	out = append(out, flowSignals...)
	return out, nil
}

func (s *PriorityScheduler) buildOverflowSignalsAndOutcome(in EnqueueInput) ([]eventabi.ControlSignal, *controlplane.DecisionOutcome, error) {
	signals, err := s.buildHighWatermarkSignals(in)
	if err != nil {
		return nil, nil, err
	}
	admit := s.admission.EvaluateSchedulingPoint(localadmission.SchedulingPointInput{
		SessionID:            s.cfg.SessionID,
		TurnID:               s.cfg.TurnID,
		EventID:              in.EventID + "-shed",
		RuntimeTimestampMS:   safeNonNegative(in.RuntimeTimestampMS),
		WallClockTimestampMS: safeNonNegative(in.WallClockTimestampMS),
		Scope:                controlplane.ScopeEdgeEnqueue,
		Shed:                 true,
		Reason:               "lane_queue_capacity_exceeded",
	})
	if admit.Outcome == nil {
		return nil, nil, fmt.Errorf("lane overflow shed outcome missing")
	}
	if err := admit.Outcome.Validate(); err != nil {
		return nil, nil, err
	}
	shedSignal, err := s.buildShedSignal(in, admit.Outcome.Reason)
	if err != nil {
		return nil, nil, err
	}
	signals = append(signals, shedSignal)
	return signals, admit.Outcome, nil
}

func (s *PriorityScheduler) buildShedSignal(in EnqueueInput, reason string) (eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if s.cfg.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transport := safeNonNegative(in.TransportSequence)
	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          s.cfg.SessionID,
		TurnID:             s.cfg.TurnID,
		PipelineVersion:    s.cfg.PipelineVersion,
		EventID:            in.EventID + "-shed",
		Lane:               eventabi.LaneControl,
		TargetLane:         in.Lane,
		TransportSequence:  &transport,
		RuntimeSequence:    safeNonNegative(in.RuntimeSequence),
		AuthorityEpoch:     safeNonNegative(s.cfg.AuthorityEpoch),
		RuntimeTimestampMS: safeNonNegative(in.RuntimeTimestampMS),
		WallClockMS:        safeNonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "shed",
		EmittedBy:          "RK-25",
		Reason:             reason,
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}

func (s *PriorityScheduler) buildWatermarkSignal(in EnqueueInput, reason string) (eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if s.cfg.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transport := safeNonNegative(in.TransportSequence)
	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          s.cfg.SessionID,
		TurnID:             s.cfg.TurnID,
		PipelineVersion:    s.cfg.PipelineVersion,
		EdgeID:             s.edgeIDForLane(in.Lane),
		EventID:            in.EventID + "-watermark",
		Lane:               eventabi.LaneControl,
		TargetLane:         in.Lane,
		TransportSequence:  &transport,
		RuntimeSequence:    safeNonNegative(in.RuntimeSequence),
		AuthorityEpoch:     safeNonNegative(s.cfg.AuthorityEpoch),
		RuntimeTimestampMS: safeNonNegative(in.RuntimeTimestampMS),
		WallClockMS:        safeNonNegative(in.WallClockTimestampMS),
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "watermark",
		EmittedBy:          "RK-13",
		Reason:             reason,
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}

func normalizeSchedulerConfig(cfg DispatchSchedulerConfig) (DispatchSchedulerConfig, error) {
	if cfg.SessionID == "" || cfg.PipelineVersion == "" {
		return DispatchSchedulerConfig{}, fmt.Errorf("session_id and pipeline_version are required")
	}
	if cfg.AuthorityEpoch < 0 {
		return DispatchSchedulerConfig{}, fmt.Errorf("authority_epoch must be >=0")
	}
	if cfg.EdgeIDPrefix == "" {
		cfg.EdgeIDPrefix = "lane-dispatch"
	}
	if cfg.QueueLimits.Control < 1 {
		cfg.QueueLimits.Control = 32
	}
	if cfg.QueueLimits.Data < 1 {
		cfg.QueueLimits.Data = 128
	}
	if cfg.QueueLimits.Telemetry < 1 {
		cfg.QueueLimits.Telemetry = 128
	}
	if cfg.Watermarks.Control.High < 1 {
		cfg.Watermarks.Control = controlplane.WatermarkThreshold{High: maxInt(1, cfg.QueueLimits.Control/2), Low: maxInt(0, cfg.QueueLimits.Control/4)}
	}
	if cfg.Watermarks.Data.High < 1 {
		cfg.Watermarks.Data = controlplane.WatermarkThreshold{High: maxInt(1, cfg.QueueLimits.Data/2), Low: maxInt(0, cfg.QueueLimits.Data/4)}
	}
	if cfg.Watermarks.Telemetry.High < 1 {
		cfg.Watermarks.Telemetry = controlplane.WatermarkThreshold{High: maxInt(1, cfg.QueueLimits.Telemetry/2), Low: maxInt(0, cfg.QueueLimits.Telemetry/4)}
	}
	if err := validateWatermarkBounds(cfg.Watermarks.Control, cfg.QueueLimits.Control); err != nil {
		return DispatchSchedulerConfig{}, fmt.Errorf("control watermark invalid: %w", err)
	}
	if err := validateWatermarkBounds(cfg.Watermarks.Data, cfg.QueueLimits.Data); err != nil {
		return DispatchSchedulerConfig{}, fmt.Errorf("data watermark invalid: %w", err)
	}
	if err := validateWatermarkBounds(cfg.Watermarks.Telemetry, cfg.QueueLimits.Telemetry); err != nil {
		return DispatchSchedulerConfig{}, fmt.Errorf("telemetry watermark invalid: %w", err)
	}
	return cfg, nil
}

func validateWatermarkBounds(w controlplane.WatermarkThreshold, limit int) error {
	if err := w.Validate(); err != nil {
		return err
	}
	if w.High > limit {
		return fmt.Errorf("high watermark %d exceeds queue limit %d", w.High, limit)
	}
	if w.Low >= w.High {
		return fmt.Errorf("low watermark %d must be < high watermark %d", w.Low, w.High)
	}
	return nil
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func safeNonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func selectPlanEdgePolicy(policies map[string]controlplane.EdgeBufferPolicy) (controlplane.EdgeBufferPolicy, error) {
	if len(policies) == 0 {
		return controlplane.EdgeBufferPolicy{}, fmt.Errorf("edge_buffer_policies requires at least one entry")
	}
	if policy, ok := policies["default"]; ok {
		return policy, nil
	}

	keys := make([]string, 0, len(policies))
	for key := range policies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return policies[keys[0]], nil
}
