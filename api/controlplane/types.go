package controlplane

import (
	"fmt"
	"regexp"
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
		if !inStringSet(mode, []string{"re_simulate_nodes", "playback_recorded_provider_outputs", "replay_decisions", "recompute_decisions"}) {
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

// ResolvedTurnPlan is the immutable turn-start artifact.
type ResolvedTurnPlan struct {
	TurnID                 string                      `json:"turn_id"`
	PipelineVersion        string                      `json:"pipeline_version"`
	PlanHash               string                      `json:"plan_hash"`
	GraphDefinitionRef     string                      `json:"graph_definition_ref"`
	ExecutionProfile       string                      `json:"execution_profile"`
	AuthorityEpoch         int64                       `json:"authority_epoch"`
	Budgets                Budgets                     `json:"budgets"`
	ProviderBindings       map[string]string           `json:"provider_bindings"`
	EdgeBufferPolicies     map[string]EdgeBufferPolicy `json:"edge_buffer_policies"`
	FlowControl            FlowControl                 `json:"flow_control"`
	AllowedAdaptiveActions []string                    `json:"allowed_adaptive_actions"`
	SnapshotProvenance     SnapshotProvenance          `json:"snapshot_provenance"`
	RecordingPolicy        RecordingPolicy             `json:"recording_policy"`
	Determinism            Determinism                 `json:"determinism"`
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
