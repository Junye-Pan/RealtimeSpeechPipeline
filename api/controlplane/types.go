package controlplane

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

// OutcomeKind mirrors docs/ContractArtifacts.schema.json decision_outcome.outcome_kind.
type OutcomeKind string

const (
	OutcomeAdmit            OutcomeKind = "admit"
	OutcomeReject           OutcomeKind = "reject"
	OutcomeDefer            OutcomeKind = "defer"
	OutcomeShed             OutcomeKind = "shed"
	OutcomeStaleEpochReject OutcomeKind = "stale_epoch_reject"
	OutcomeDeauthorized     OutcomeKind = "deauthorized_drain"
)

// OutcomePhase mirrors docs/ContractArtifacts.schema.json decision_outcome.phase.
type OutcomePhase string

const (
	PhasePreTurn    OutcomePhase = "pre_turn"
	PhaseScheduling OutcomePhase = "scheduling_point"
	PhaseActiveTurn OutcomePhase = "active_turn"
)

// OutcomeScope mirrors docs/ContractArtifacts.schema.json decision_outcome.scope.
type OutcomeScope string

const (
	ScopeTenant       OutcomeScope = "tenant"
	ScopeSession      OutcomeScope = "session"
	ScopeTurn         OutcomeScope = "turn"
	ScopeEdgeEnqueue  OutcomeScope = "edge_enqueue"
	ScopeEdgeDequeue  OutcomeScope = "edge_dequeue"
	ScopeNodeDispatch OutcomeScope = "node_dispatch"
)

// OutcomeEmitter mirrors docs/ContractArtifacts.schema.json decision_outcome.emitted_by.
type OutcomeEmitter string

const (
	EmitterRK24 OutcomeEmitter = "RK-24"
	EmitterRK25 OutcomeEmitter = "RK-25"
	EmitterCP05 OutcomeEmitter = "CP-05"
)

// DecisionOutcome models deterministic admission/authority outputs.
type DecisionOutcome struct {
	OutcomeKind        OutcomeKind    `json:"outcome_kind"`
	Phase              OutcomePhase   `json:"phase"`
	Scope              OutcomeScope   `json:"scope"`
	SessionID          string         `json:"session_id"`
	TurnID             string         `json:"turn_id,omitempty"`
	EventID            string         `json:"event_id"`
	RuntimeTimestampMS int64          `json:"runtime_timestamp_ms"`
	WallClockMS        int64          `json:"wall_clock_timestamp_ms"`
	TimestampMS        *int64         `json:"timestamp_ms,omitempty"`
	EmittedBy          OutcomeEmitter `json:"emitted_by"`
	AuthorityEpoch     *int64         `json:"authority_epoch,omitempty"`
	Reason             string         `json:"reason"`
}

func (d DecisionOutcome) Validate() error {
	if !isOutcomeKind(d.OutcomeKind) || !isOutcomePhase(d.Phase) || !isOutcomeScope(d.Scope) || !isOutcomeEmitter(d.EmittedBy) {
		return fmt.Errorf("invalid enum value in decision_outcome")
	}
	if d.SessionID == "" || d.EventID == "" || d.Reason == "" {
		return fmt.Errorf("session_id, event_id, and reason are required")
	}
	if d.RuntimeTimestampMS < 0 || d.WallClockMS < 0 {
		return fmt.Errorf("timestamps must be >= 0")
	}
	if d.TimestampMS != nil && *d.TimestampMS < 0 {
		return fmt.Errorf("timestamp_ms must be >=0")
	}

	switch d.OutcomeKind {
	case OutcomeAdmit, OutcomeReject, OutcomeDefer:
		if d.EmittedBy != EmitterRK25 && d.EmittedBy != EmitterCP05 {
			return fmt.Errorf("%s must be emitted by RK-25 or CP-05", d.OutcomeKind)
		}
	case OutcomeShed:
		if d.EmittedBy != EmitterRK25 || d.Phase != PhaseScheduling {
			return fmt.Errorf("shed requires emitted_by=RK-25 and phase=scheduling_point")
		}
	case OutcomeStaleEpochReject, OutcomeDeauthorized:
		if d.EmittedBy != EmitterRK24 {
			return fmt.Errorf("%s must be emitted by RK-24", d.OutcomeKind)
		}
		if d.AuthorityEpoch == nil {
			return fmt.Errorf("authority_epoch is required for authority outcomes")
		}
	}

	if d.EmittedBy == EmitterCP05 {
		if d.Phase != PhasePreTurn {
			return fmt.Errorf("CP-05 outcomes must be pre_turn")
		}
		if d.Scope != ScopeTenant && d.Scope != ScopeSession {
			return fmt.Errorf("CP-05 pre_turn scope must be tenant|session")
		}
	}

	switch d.Phase {
	case PhaseScheduling:
		if d.Scope != ScopeEdgeEnqueue && d.Scope != ScopeEdgeDequeue && d.Scope != ScopeNodeDispatch {
			return fmt.Errorf("scheduling_point scope must be edge_enqueue|edge_dequeue|node_dispatch")
		}
	case PhaseActiveTurn:
		if d.Scope != ScopeTurn && d.Scope != ScopeEdgeEnqueue && d.Scope != ScopeEdgeDequeue && d.Scope != ScopeNodeDispatch {
			return fmt.Errorf("active_turn scope must be turn or scheduling scope")
		}
		if d.TurnID == "" {
			return fmt.Errorf("turn_id is required for active_turn")
		}
	case PhasePreTurn:
		if d.Scope != ScopeTenant && d.Scope != ScopeSession && d.Scope != ScopeTurn {
			return fmt.Errorf("pre_turn scope must be tenant|session|turn")
		}
	}

	if d.Scope == ScopeTurn && d.TurnID == "" {
		return fmt.Errorf("turn_id is required when scope=turn")
	}

	if d.OutcomeKind == OutcomeDeauthorized {
		if d.Phase != PhasePreTurn && d.Phase != PhaseActiveTurn {
			return fmt.Errorf("deauthorized_drain phase must be pre_turn|active_turn")
		}
		if d.Phase == PhaseActiveTurn && d.Scope != ScopeTurn {
			return fmt.Errorf("deauthorized_drain active_turn scope must be turn")
		}
		if d.Phase == PhasePreTurn && d.Scope != ScopeSession && d.Scope != ScopeTurn {
			return fmt.Errorf("deauthorized_drain pre_turn scope must be session|turn")
		}
	}

	if d.OutcomeKind == OutcomeStaleEpochReject {
		if d.Phase != PhasePreTurn && d.Phase != PhaseScheduling {
			return fmt.Errorf("stale_epoch_reject phase must be pre_turn|scheduling_point")
		}
		if d.Phase == PhasePreTurn && d.Scope != ScopeSession && d.Scope != ScopeTurn {
			return fmt.Errorf("stale_epoch_reject pre_turn scope must be session|turn")
		}
	}

	return nil
}

// TurnState mirrors docs/ContractArtifacts.schema.json turn_state.
type TurnState string

const (
	TurnIdle     TurnState = "Idle"
	TurnOpening  TurnState = "Opening"
	TurnActive   TurnState = "Active"
	TurnTerminal TurnState = "Terminal"
	TurnClosed   TurnState = "Closed"
)

// TransitionTrigger mirrors docs/ContractArtifacts.schema.json turn_transition.trigger.
type TransitionTrigger string

const (
	TriggerTurnOpenProposed TransitionTrigger = "turn_open_proposed"
	TriggerTurnOpen         TransitionTrigger = "turn_open"
	TriggerCommit           TransitionTrigger = "commit"
	TriggerAbort            TransitionTrigger = "abort"
	TriggerClose            TransitionTrigger = "close"
	TriggerReject           TransitionTrigger = "reject"
	TriggerDefer            TransitionTrigger = "defer"
	TriggerStaleEpochReject TransitionTrigger = "stale_epoch_reject"
	TriggerDeauthorized     TransitionTrigger = "deauthorized_drain"
)

// TurnTransition models deterministic lifecycle edges.
type TurnTransition struct {
	FromState     TurnState         `json:"from_state"`
	Trigger       TransitionTrigger `json:"trigger"`
	ToState       TurnState         `json:"to_state"`
	Deterministic bool              `json:"deterministic"`
}

func (t TurnTransition) Validate() error {
	if !t.Deterministic {
		return fmt.Errorf("deterministic must be true")
	}

	isAllowed :=
		(t.FromState == TurnIdle && t.Trigger == TriggerTurnOpenProposed && t.ToState == TurnOpening) ||
			(t.FromState == TurnOpening && t.Trigger == TriggerTurnOpen && t.ToState == TurnActive) ||
			(t.FromState == TurnOpening && (t.Trigger == TriggerReject || t.Trigger == TriggerDefer || t.Trigger == TriggerStaleEpochReject || t.Trigger == TriggerDeauthorized) && t.ToState == TurnIdle) ||
			(t.FromState == TurnActive && (t.Trigger == TriggerCommit || t.Trigger == TriggerAbort) && t.ToState == TurnTerminal) ||
			(t.FromState == TurnTerminal && t.Trigger == TriggerClose && t.ToState == TurnClosed)

	if !isAllowed {
		return fmt.Errorf("illegal turn transition: %s --%s--> %s", t.FromState, t.Trigger, t.ToState)
	}
	return nil
}

type BufferStrategy string

const (
	BufferStrategyBlock      BufferStrategy = "block"
	BufferStrategyDrop       BufferStrategy = "drop"
	BufferStrategyMerge      BufferStrategy = "merge"
	BufferStrategySample     BufferStrategy = "sample"
	BufferStrategyLatestOnly BufferStrategy = "latest_only"
)

type WatermarkThreshold struct {
	High int `json:"high"`
	Low  int `json:"low"`
}

func (w WatermarkThreshold) Validate() error {
	if w.High < 1 || w.Low < 0 {
		return fmt.Errorf("watermark high>=1 and low>=0 are required")
	}
	return nil
}

type EdgeWatermarks struct {
	QueueItems *WatermarkThreshold `json:"queue_items,omitempty"`
	QueueMS    *WatermarkThreshold `json:"queue_ms,omitempty"`
}

func (w EdgeWatermarks) Validate() error {
	if w.QueueItems == nil && w.QueueMS == nil {
		return fmt.Errorf("at least one watermark domain is required")
	}
	if w.QueueItems != nil {
		if err := w.QueueItems.Validate(); err != nil {
			return err
		}
	}
	if w.QueueMS != nil {
		if err := w.QueueMS.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type LaneHandling struct {
	DataLane      string `json:"DataLane"`
	ControlLane   string `json:"ControlLane"`
	TelemetryLane string `json:"TelemetryLane"`
}

func (l LaneHandling) Validate() error {
	if !inStringSet(l.DataLane, []string{"block", "drop", "merge", "sample", "latest_only"}) {
		return fmt.Errorf("invalid lane_handling.DataLane")
	}
	if l.ControlLane != "non_blocking_priority" {
		return fmt.Errorf("lane_handling.ControlLane must be non_blocking_priority")
	}
	if !inStringSet(l.TelemetryLane, []string{"drop", "sample", "best_effort_drop"}) {
		return fmt.Errorf("invalid lane_handling.TelemetryLane")
	}
	return nil
}

type SyncDropPolicy struct {
	GroupBy    string `json:"group_by,omitempty"`
	Policy     string `json:"policy"`
	Scope      string `json:"scope"`
	SyncDomain string `json:"sync_domain,omitempty"`
}

func (s SyncDropPolicy) Validate() error {
	if !inStringSet(s.Policy, []string{"atomic_drop", "drop_with_discontinuity", "no_sync"}) {
		return fmt.Errorf("invalid sync_drop_policy.policy")
	}
	if !inStringSet(s.Scope, []string{"edge_local", "plan_wide"}) {
		return fmt.Errorf("invalid sync_drop_policy.scope")
	}
	if s.GroupBy != "" && !inStringSet(s.GroupBy, []string{"sync_id", "event_id", "custom"}) {
		return fmt.Errorf("invalid sync_drop_policy.group_by")
	}
	if (s.Policy == "atomic_drop" || s.Policy == "drop_with_discontinuity") && s.GroupBy == "" {
		return fmt.Errorf("sync_drop_policy.group_by is required for policy %s", s.Policy)
	}
	if s.Policy == "drop_with_discontinuity" && s.SyncDomain == "" {
		return fmt.Errorf("sync_drop_policy.sync_domain is required for drop_with_discontinuity")
	}
	return nil
}

type EdgeBufferPolicy struct {
	Strategy                 BufferStrategy  `json:"strategy"`
	MaxQueueItems            int             `json:"max_queue_items"`
	MaxQueueMS               int             `json:"max_queue_ms"`
	MaxQueueBytes            int             `json:"max_queue_bytes"`
	MaxLatencyContributionMS int             `json:"max_latency_contribution_ms"`
	MaxBlockTimeMS           int             `json:"max_block_time_ms,omitempty"`
	Watermarks               EdgeWatermarks  `json:"watermarks"`
	LaneHandling             LaneHandling    `json:"lane_handling"`
	FairnessKey              string          `json:"fairness_key,omitempty"`
	DefaultingSource         string          `json:"defaulting_source"`
	SyncDropPolicy           *SyncDropPolicy `json:"sync_drop_policy,omitempty"`
}

func (p EdgeBufferPolicy) Validate() error {
	if !inStringSet(string(p.Strategy), []string{"block", "drop", "merge", "sample", "latest_only"}) {
		return fmt.Errorf("invalid edge_buffer_policies.*.strategy")
	}
	if p.MaxQueueItems < 1 || p.MaxQueueMS < 1 || p.MaxQueueBytes < 1 || p.MaxLatencyContributionMS < 1 {
		return fmt.Errorf("edge buffer numeric limits must be >=1")
	}
	if p.Strategy == BufferStrategyBlock && p.MaxBlockTimeMS < 1 {
		return fmt.Errorf("max_block_time_ms is required when strategy=block")
	}
	if err := p.Watermarks.Validate(); err != nil {
		return err
	}
	if err := p.LaneHandling.Validate(); err != nil {
		return err
	}
	if !inStringSet(p.DefaultingSource, []string{"explicit_edge_config", "execution_profile_default"}) {
		return fmt.Errorf("invalid defaulting_source")
	}
	if p.SyncDropPolicy != nil {
		if err := p.SyncDropPolicy.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type Budgets struct {
	TurnBudgetMS        int `json:"turn_budget_ms"`
	NodeBudgetMSDefault int `json:"node_budget_ms_default"`
	PathBudgetMSDefault int `json:"path_budget_ms_default"`
	EdgeBudgetMSDefault int `json:"edge_budget_ms_default"`
}

func (b Budgets) Validate() error {
	if b.TurnBudgetMS < 1 || b.NodeBudgetMSDefault < 1 || b.PathBudgetMSDefault < 1 || b.EdgeBudgetMSDefault < 1 {
		return fmt.Errorf("all budget defaults must be >=1")
	}
	return nil
}

// NodeExecutionPolicy captures CP-originated node-level execution fairness/concurrency settings.
type NodeExecutionPolicy struct {
	ConcurrencyLimit int    `json:"concurrency_limit,omitempty"`
	FairnessKey      string `json:"fairness_key,omitempty"`
}

func (p NodeExecutionPolicy) Validate() error {
	if p.ConcurrencyLimit < 0 {
		return fmt.Errorf("node_execution_policies.*.concurrency_limit must be >=0")
	}
	if p.FairnessKey != "" && strings.TrimSpace(p.FairnessKey) == "" {
		return fmt.Errorf("node_execution_policies.*.fairness_key must not be whitespace")
	}
	return nil
}

type ModeByLane struct {
	DataLane      string `json:"DataLane"`
	ControlLane   string `json:"ControlLane"`
	TelemetryLane string `json:"TelemetryLane"`
}

func (m ModeByLane) Validate() error {
	if !inStringSet(m.DataLane, []string{"signal", "credit", "hybrid"}) ||
		!inStringSet(m.ControlLane, []string{"signal", "credit", "hybrid"}) ||
		!inStringSet(m.TelemetryLane, []string{"signal", "credit", "hybrid"}) {
		return fmt.Errorf("invalid flow_control.mode_by_lane")
	}
	return nil
}

type FlowWatermarks struct {
	DataLane      WatermarkThreshold `json:"DataLane"`
	ControlLane   WatermarkThreshold `json:"ControlLane"`
	TelemetryLane WatermarkThreshold `json:"TelemetryLane"`
}

func (w FlowWatermarks) Validate() error {
	if err := w.DataLane.Validate(); err != nil {
		return err
	}
	if err := w.ControlLane.Validate(); err != nil {
		return err
	}
	if err := w.TelemetryLane.Validate(); err != nil {
		return err
	}
	return nil
}

type SheddingStrategyByLane struct {
	DataLane      string `json:"DataLane"`
	ControlLane   string `json:"ControlLane"`
	TelemetryLane string `json:"TelemetryLane"`
}

func (s SheddingStrategyByLane) Validate() error {
	if !inStringSet(s.DataLane, []string{"drop", "merge", "latest_only", "degrade", "fallback"}) {
		return fmt.Errorf("invalid flow_control.shedding_strategy_by_lane.DataLane")
	}
	if s.ControlLane != "none" {
		return fmt.Errorf("flow_control.shedding_strategy_by_lane.ControlLane must be none")
	}
	if !inStringSet(s.TelemetryLane, []string{"drop", "sample"}) {
		return fmt.Errorf("invalid flow_control.shedding_strategy_by_lane.TelemetryLane")
	}
	return nil
}

type FlowControl struct {
	ModeByLane             ModeByLane             `json:"mode_by_lane"`
	Watermarks             FlowWatermarks         `json:"watermarks"`
	SheddingStrategyByLane SheddingStrategyByLane `json:"shedding_strategy_by_lane"`
}

func (f FlowControl) Validate() error {
	if err := f.ModeByLane.Validate(); err != nil {
		return err
	}
	if err := f.Watermarks.Validate(); err != nil {
		return err
	}
	if err := f.SheddingStrategyByLane.Validate(); err != nil {
		return err
	}
	return nil
}

// SnapshotProvenance captures required turn-start snapshot references.
type SnapshotProvenance struct {
	RoutingViewSnapshot       string `json:"routing_view_snapshot"`
	AdmissionPolicySnapshot   string `json:"admission_policy_snapshot"`
	ABICompatibilitySnapshot  string `json:"abi_compatibility_snapshot"`
	VersionResolutionSnapshot string `json:"version_resolution_snapshot"`
	PolicyResolutionSnapshot  string `json:"policy_resolution_snapshot"`
	ProviderHealthSnapshot    string `json:"provider_health_snapshot"`
}

func (s SnapshotProvenance) Validate() error {
	if s.RoutingViewSnapshot == "" ||
		s.AdmissionPolicySnapshot == "" ||
		s.ABICompatibilitySnapshot == "" ||
		s.VersionResolutionSnapshot == "" ||
		s.PolicyResolutionSnapshot == "" ||
		s.ProviderHealthSnapshot == "" {
		return fmt.Errorf("all snapshot_provenance refs are required")
	}
	return nil
}

type RecordingPolicy struct {
	RecordingLevel     string   `json:"recording_level"`
	AllowedReplayModes []string `json:"allowed_replay_modes"`
}

func (r RecordingPolicy) Validate() error {
	if !inStringSet(r.RecordingLevel, []string{"L0", "L1", "L2"}) {
		return fmt.Errorf("invalid recording_policy.recording_level")
	}
	if len(r.AllowedReplayModes) < 1 {
		return fmt.Errorf("recording_policy.allowed_replay_modes requires at least one mode")
	}
	seen := map[string]struct{}{}
	for _, mode := range r.AllowedReplayModes {
		if !obs.IsReplayMode(mode) {
			return fmt.Errorf("invalid replay mode: %s", mode)
		}
		if _, ok := seen[mode]; ok {
			return fmt.Errorf("duplicate replay mode: %s", mode)
		}
		seen[mode] = struct{}{}
	}
	return nil
}

type NondeterministicInput struct {
	InputKey    string `json:"input_key"`
	Source      string `json:"source"`
	EvidenceRef string `json:"evidence_ref"`
}

func (n NondeterministicInput) Validate() error {
	if n.InputKey == "" || n.EvidenceRef == "" {
		return fmt.Errorf("nondeterministic input_key and evidence_ref are required")
	}
	if !inStringSet(n.Source, []string{"external_call", "system_time", "random", "operator_override", "other"}) {
		return fmt.Errorf("invalid nondeterministic input source")
	}
	return nil
}

type Determinism struct {
	Seed                   int64                   `json:"seed"`
	OrderingMarker         string                  `json:"ordering_marker,omitempty"`
	OrderingMarkers        []string                `json:"ordering_markers"`
	MergeRuleID            string                  `json:"merge_rule_id"`
	MergeRuleVersion       string                  `json:"merge_rule_version"`
	NondeterministicInputs []NondeterministicInput `json:"nondeterministic_inputs"`
}

func (d Determinism) Validate() error {
	if d.Seed < 0 {
		return fmt.Errorf("determinism.seed must be >=0")
	}
	if len(d.OrderingMarkers) < 1 {
		return fmt.Errorf("determinism.ordering_markers requires at least one value")
	}
	if d.NondeterministicInputs == nil {
		return fmt.Errorf("determinism.nondeterministic_inputs is required")
	}
	seen := map[string]struct{}{}
	for _, marker := range d.OrderingMarkers {
		if marker == "" {
			return fmt.Errorf("ordering marker cannot be empty")
		}
		if _, ok := seen[marker]; ok {
			return fmt.Errorf("duplicate ordering marker: %s", marker)
		}
		seen[marker] = struct{}{}
	}
	if d.MergeRuleID == "" {
		return fmt.Errorf("determinism.merge_rule_id is required")
	}
	if ok, _ := regexp.MatchString(`^v?[0-9]+\.[0-9]+(?:\.[0-9]+)?$`, d.MergeRuleVersion); !ok {
		return fmt.Errorf("invalid determinism.merge_rule_version")
	}
	for _, item := range d.NondeterministicInputs {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// StreamingHandoffEdgePolicy configures one orchestration handoff edge.
type StreamingHandoffEdgePolicy struct {
	Enabled                  bool `json:"enabled"`
	MinPartialChars          int  `json:"min_partial_chars,omitempty"`
	TriggerOnPunctuation     bool `json:"trigger_on_punctuation,omitempty"`
	MaxPartialAgeMS          int  `json:"max_partial_age_ms,omitempty"`
	MaxPendingRevisions      int  `json:"max_pending_revisions,omitempty"`
	MaxPendingSegments       int  `json:"max_pending_segments,omitempty"`
	CoalesceLatestOnly       bool `json:"coalesce_latest_only,omitempty"`
	EmitFlowControlSignals   bool `json:"emit_flow_control_signals,omitempty"`
	DegradeOnThresholdBreach bool `json:"degrade_on_threshold_breach,omitempty"`
}

func (p StreamingHandoffEdgePolicy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.MinPartialChars < 1 {
		return fmt.Errorf("streaming handoff min_partial_chars must be >=1 when enabled")
	}
	if p.MaxPartialAgeMS < 1 {
		return fmt.Errorf("streaming handoff max_partial_age_ms must be >=1 when enabled")
	}
	if p.MaxPendingRevisions < 1 {
		return fmt.Errorf("streaming handoff max_pending_revisions must be >=1 when enabled")
	}
	if p.MaxPendingSegments < 1 {
		return fmt.Errorf("streaming handoff max_pending_segments must be >=1 when enabled")
	}
	return nil
}

// StreamingHandoffPolicy configures orchestration-level overlap behavior for a turn.
type StreamingHandoffPolicy struct {
	Enabled  bool                       `json:"enabled"`
	STTToLLM StreamingHandoffEdgePolicy `json:"stt_to_llm"`
	LLMToTTS StreamingHandoffEdgePolicy `json:"llm_to_tts"`
}

func (p StreamingHandoffPolicy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if err := p.STTToLLM.Validate(); err != nil {
		return err
	}
	if err := p.LLMToTTS.Validate(); err != nil {
		return err
	}
	if !p.STTToLLM.Enabled && !p.LLMToTTS.Enabled {
		return fmt.Errorf("streaming handoff requires at least one enabled edge when policy is enabled")
	}
	return nil
}

// ResolvedTurnPlan is the immutable turn-start artifact.
type ResolvedTurnPlan struct {
	TurnID                 string                         `json:"turn_id"`
	PipelineVersion        string                         `json:"pipeline_version"`
	PlanHash               string                         `json:"plan_hash"`
	GraphDefinitionRef     string                         `json:"graph_definition_ref"`
	ExecutionProfile       string                         `json:"execution_profile"`
	AuthorityEpoch         int64                          `json:"authority_epoch"`
	Budgets                Budgets                        `json:"budgets"`
	ProviderBindings       map[string]string              `json:"provider_bindings"`
	EdgeBufferPolicies     map[string]EdgeBufferPolicy    `json:"edge_buffer_policies"`
	NodeExecutionPolicies  map[string]NodeExecutionPolicy `json:"node_execution_policies,omitempty"`
	FlowControl            FlowControl                    `json:"flow_control"`
	AllowedAdaptiveActions []string                       `json:"allowed_adaptive_actions"`
	SnapshotProvenance     SnapshotProvenance             `json:"snapshot_provenance"`
	RecordingPolicy        RecordingPolicy                `json:"recording_policy"`
	Determinism            Determinism                    `json:"determinism"`
	StreamingHandoff       *StreamingHandoffPolicy        `json:"streaming_handoff,omitempty"`
}

func (p ResolvedTurnPlan) Validate() error {
	if p.TurnID == "" || p.PipelineVersion == "" || p.GraphDefinitionRef == "" || p.ExecutionProfile == "" {
		return fmt.Errorf("resolved_turn_plan identity fields are required")
	}
	if ok, _ := regexp.MatchString(`^[a-fA-F0-9]{64}$`, p.PlanHash); !ok {
		return fmt.Errorf("plan_hash must be 64 hex chars")
	}
	if p.AuthorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >= 0")
	}
	if err := p.Budgets.Validate(); err != nil {
		return err
	}
	if len(p.ProviderBindings) == 0 {
		return fmt.Errorf("provider_bindings must be non-empty")
	}
	for k, v := range p.ProviderBindings {
		if k == "" || v == "" {
			return fmt.Errorf("provider_bindings keys and values must be non-empty")
		}
	}
	if len(p.EdgeBufferPolicies) < 1 {
		return fmt.Errorf("edge_buffer_policies requires at least one entry")
	}
	for id, policy := range p.EdgeBufferPolicies {
		if id == "" {
			return fmt.Errorf("edge_buffer_policies key cannot be empty")
		}
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	for nodeID, policy := range p.NodeExecutionPolicies {
		if nodeID == "" {
			return fmt.Errorf("node_execution_policies key cannot be empty")
		}
		if err := policy.Validate(); err != nil {
			return err
		}
	}
	if err := p.FlowControl.Validate(); err != nil {
		return err
	}
	if p.AllowedAdaptiveActions == nil {
		return fmt.Errorf("allowed_adaptive_actions is required")
	}
	seenActions := map[string]struct{}{}
	for _, action := range p.AllowedAdaptiveActions {
		if !inStringSet(action, []string{"retry", "provider_switch", "degrade", "fallback"}) {
			return fmt.Errorf("invalid allowed adaptive action: %s", action)
		}
		if _, ok := seenActions[action]; ok {
			return fmt.Errorf("duplicate adaptive action: %s", action)
		}
		seenActions[action] = struct{}{}
	}
	if err := p.SnapshotProvenance.Validate(); err != nil {
		return err
	}
	if err := p.RecordingPolicy.Validate(); err != nil {
		return err
	}
	if err := p.Determinism.Validate(); err != nil {
		return err
	}
	if p.StreamingHandoff != nil {
		if err := p.StreamingHandoff.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// TransportEndpointRef describes a transport-specific runtime endpoint.
type TransportEndpointRef struct {
	TransportKind string `json:"transport_kind"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region,omitempty"`
	RuntimeID     string `json:"runtime_id,omitempty"`
}

func (t TransportEndpointRef) Validate() error {
	if t.TransportKind == "" {
		return fmt.Errorf("transport_kind is required")
	}
	if t.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	return nil
}

// LeaseTokenRef describes the lease-token metadata bound to a placement decision.
type LeaseTokenRef struct {
	TokenID      string `json:"token_id"`
	ExpiresAtUTC string `json:"expires_at_utc"`
}

func (l LeaseTokenRef) Validate() error {
	if l.TokenID == "" {
		return fmt.Errorf("token_id is required")
	}
	if l.ExpiresAtUTC == "" {
		return fmt.Errorf("expires_at_utc is required")
	}
	if _, err := time.Parse(time.RFC3339, l.ExpiresAtUTC); err != nil {
		return fmt.Errorf("invalid expires_at_utc: %w", err)
	}
	return nil
}

// PlacementLease captures authority metadata for a resolved session route.
type PlacementLease struct {
	AuthorityEpoch int64         `json:"authority_epoch"`
	Granted        bool          `json:"granted"`
	Valid          bool          `json:"valid"`
	TokenRef       LeaseTokenRef `json:"token_ref"`
}

func (p PlacementLease) Validate() error {
	if p.AuthorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >=0")
	}
	if err := p.TokenRef.Validate(); err != nil {
		return err
	}
	return nil
}

// SessionRoute is the client-consumable route contract returned at bootstrap.
type SessionRoute struct {
	TenantID                string               `json:"tenant_id"`
	SessionID               string               `json:"session_id"`
	PipelineVersion         string               `json:"pipeline_version"`
	RoutingViewSnapshot     string               `json:"routing_view_snapshot"`
	AdmissionPolicySnapshot string               `json:"admission_policy_snapshot"`
	Endpoint                TransportEndpointRef `json:"endpoint"`
	Lease                   PlacementLease       `json:"lease"`
}

func (s SessionRoute) Validate() error {
	if s.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if s.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if s.PipelineVersion == "" {
		return fmt.Errorf("pipeline_version is required")
	}
	if s.RoutingViewSnapshot == "" {
		return fmt.Errorf("routing_view_snapshot is required")
	}
	if s.AdmissionPolicySnapshot == "" {
		return fmt.Errorf("admission_policy_snapshot is required")
	}
	if err := s.Endpoint.Validate(); err != nil {
		return err
	}
	if err := s.Lease.Validate(); err != nil {
		return err
	}
	return nil
}

// SessionTokenClaims are token claims required by transport adapters/runtime.
type SessionTokenClaims struct {
	TenantID        string `json:"tenant_id"`
	SessionID       string `json:"session_id"`
	PipelineVersion string `json:"pipeline_version"`
	TransportKind   string `json:"transport_kind"`
	AuthorityEpoch  int64  `json:"authority_epoch"`
	IssuedAtUTC     string `json:"issued_at_utc"`
	ExpiresAtUTC    string `json:"expires_at_utc"`
}

func (s SessionTokenClaims) Validate() error {
	if s.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if s.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if s.PipelineVersion == "" {
		return fmt.Errorf("pipeline_version is required")
	}
	if s.TransportKind == "" {
		return fmt.Errorf("transport_kind is required")
	}
	if s.AuthorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >=0")
	}
	issuedAt, err := time.Parse(time.RFC3339, s.IssuedAtUTC)
	if err != nil {
		return fmt.Errorf("invalid issued_at_utc: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, s.ExpiresAtUTC)
	if err != nil {
		return fmt.Errorf("invalid expires_at_utc: %w", err)
	}
	if !expiresAt.After(issuedAt) {
		return fmt.Errorf("expires_at_utc must be after issued_at_utc")
	}
	return nil
}

// SignedSessionToken is the token payload returned to clients.
type SignedSessionToken struct {
	Token        string             `json:"token"`
	TokenID      string             `json:"token_id"`
	ExpiresAtUTC string             `json:"expires_at_utc"`
	Claims       SessionTokenClaims `json:"claims"`
}

func (s SignedSessionToken) Validate() error {
	if s.Token == "" {
		return fmt.Errorf("token is required")
	}
	if s.TokenID == "" {
		return fmt.Errorf("token_id is required")
	}
	if s.ExpiresAtUTC == "" {
		return fmt.Errorf("expires_at_utc is required")
	}
	if _, err := time.Parse(time.RFC3339, s.ExpiresAtUTC); err != nil {
		return fmt.Errorf("invalid expires_at_utc: %w", err)
	}
	if err := s.Claims.Validate(); err != nil {
		return err
	}
	if s.Claims.ExpiresAtUTC != s.ExpiresAtUTC {
		return fmt.Errorf("claims.expires_at_utc must match token expires_at_utc")
	}
	if s.Claims.SessionID == "" {
		return fmt.Errorf("claims.session_id is required")
	}
	return nil
}

// SessionStatus is the control-plane session lifecycle status exposed to operations.
type SessionStatus string

const (
	SessionStatusConnected SessionStatus = "connected"
	SessionStatusRunning   SessionStatus = "running"
	SessionStatusDegraded  SessionStatus = "degraded"
	SessionStatusEnded     SessionStatus = "ended"
)

func (s SessionStatus) Validate() error {
	switch s {
	case SessionStatusConnected, SessionStatusRunning, SessionStatusDegraded, SessionStatusEnded:
		return nil
	default:
		return fmt.Errorf("invalid session status: %s", s)
	}
}

// SessionStatusView is the status response contract for operations.
type SessionStatusView struct {
	TenantID        string        `json:"tenant_id"`
	SessionID       string        `json:"session_id"`
	Status          SessionStatus `json:"status"`
	PipelineVersion string        `json:"pipeline_version,omitempty"`
	AuthorityEpoch  int64         `json:"authority_epoch,omitempty"`
	UpdatedAtUTC    string        `json:"updated_at_utc"`
	LastReason      string        `json:"last_reason,omitempty"`
}

func (s SessionStatusView) Validate() error {
	if s.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if s.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if err := s.Status.Validate(); err != nil {
		return err
	}
	if s.UpdatedAtUTC == "" {
		return fmt.Errorf("updated_at_utc is required")
	}
	if _, err := time.Parse(time.RFC3339, s.UpdatedAtUTC); err != nil {
		return fmt.Errorf("invalid updated_at_utc: %w", err)
	}
	if s.AuthorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >=0")
	}
	return nil
}

// SessionRouteRequest defines control-plane request input for route resolution.
type SessionRouteRequest struct {
	TenantID                 string `json:"tenant_id"`
	SessionID                string `json:"session_id"`
	RequestedPipelineVersion string `json:"requested_pipeline_version,omitempty"`
	TransportKind            string `json:"transport_kind,omitempty"`
	RequestedAuthorityEpoch  int64  `json:"requested_authority_epoch,omitempty"`
}

func (r SessionRouteRequest) Validate() error {
	if r.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if r.RequestedAuthorityEpoch < 0 {
		return fmt.Errorf("requested_authority_epoch must be >=0")
	}
	return nil
}

// SessionTokenRequest defines control-plane request input for token issuance.
type SessionTokenRequest struct {
	TenantID                 string `json:"tenant_id"`
	SessionID                string `json:"session_id"`
	RequestedPipelineVersion string `json:"requested_pipeline_version,omitempty"`
	TransportKind            string `json:"transport_kind,omitempty"`
	RequestedAuthorityEpoch  int64  `json:"requested_authority_epoch,omitempty"`
	TTLSeconds               int    `json:"ttl_seconds,omitempty"`
}

func (r SessionTokenRequest) Validate() error {
	if r.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if r.RequestedAuthorityEpoch < 0 {
		return fmt.Errorf("requested_authority_epoch must be >=0")
	}
	if r.TTLSeconds < 0 {
		return fmt.Errorf("ttl_seconds must be >=0")
	}
	return nil
}

// SessionStatusRequest defines control-plane request input for session status lookup.
type SessionStatusRequest struct {
	TenantID  string `json:"tenant_id"`
	SessionID string `json:"session_id"`
}

func (r SessionStatusRequest) Validate() error {
	if r.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	return nil
}

func inStringSet(v string, set []string) bool {
	for _, candidate := range set {
		if v == candidate {
			return true
		}
	}
	return false
}

func isOutcomeKind(v OutcomeKind) bool {
	switch v {
	case OutcomeAdmit, OutcomeReject, OutcomeDefer, OutcomeShed, OutcomeStaleEpochReject, OutcomeDeauthorized:
		return true
	default:
		return false
	}
}

func isOutcomePhase(v OutcomePhase) bool {
	switch v {
	case PhasePreTurn, PhaseScheduling, PhaseActiveTurn:
		return true
	default:
		return false
	}
}

func isOutcomeScope(v OutcomeScope) bool {
	switch v {
	case ScopeTenant, ScopeSession, ScopeTurn, ScopeEdgeEnqueue, ScopeEdgeDequeue, ScopeNodeDispatch:
		return true
	default:
		return false
	}
}

func isOutcomeEmitter(v OutcomeEmitter) bool {
	switch v {
	case EmitterRK24, EmitterRK25, EmitterCP05:
		return true
	default:
		return false
	}
}
