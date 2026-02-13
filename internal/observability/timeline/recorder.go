package timeline

import (
	"fmt"
	"sort"
	"sync"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

var (
	// ErrBaselineCapacityExhausted indicates Stage-A reserved capacity is depleted.
	ErrBaselineCapacityExhausted = fmt.Errorf("timeline baseline stage-a capacity exhausted")
	// ErrProviderAttemptCapacityExhausted indicates Stage-A provider attempt capacity is depleted.
	ErrProviderAttemptCapacityExhausted = fmt.Errorf("timeline provider attempt stage-a capacity exhausted")
	// ErrHandoffCapacityExhausted indicates Stage-A handoff evidence capacity is depleted.
	ErrHandoffCapacityExhausted = fmt.Errorf("timeline handoff stage-a capacity exhausted")
	// ErrInvocationSnapshotCapacityExhausted indicates Stage-A invocation snapshot capacity is depleted.
	ErrInvocationSnapshotCapacityExhausted = fmt.Errorf("timeline invocation snapshot stage-a capacity exhausted")
)

// StageAConfig defines bounded Stage-A append capacities.
type StageAConfig struct {
	BaselineCapacity         int
	DetailCapacity           int
	AttemptCapacity          int
	HandoffCapacity          int
	InvocationSnapshotCap    int
	EnableInvocationSnapshot bool
}

// BaselineEvidence holds replay-critical OR-02 Stage-A evidence.
type BaselineEvidence struct {
	SessionID                string
	TurnID                   string
	PipelineVersion          string
	EventID                  string
	EnvelopeSnapshot         string
	PayloadTags              []eventabi.PayloadClass
	RedactionDecisions       []eventabi.RedactionDecision
	PlanHash                 string
	SnapshotProvenance       controlplane.SnapshotProvenance
	DecisionOutcomes         []controlplane.DecisionOutcome
	InvocationOutcomes       []InvocationOutcomeEvidence
	DeterminismSeed          int64
	OrderingMarkers          []string
	MergeRuleID              string
	MergeRuleVersion         string
	AuthorityEpoch           int64
	MigrationMarkers         []string
	TerminalOutcome          string
	TerminalReason           string
	CloseEmitted             bool
	TurnOpenProposedAtMS     *int64
	TurnOpenAtMS             *int64
	FirstOutputAtMS          *int64
	CancelAcceptedAtMS       *int64
	CancelFenceAppliedAtMS   *int64
	CancelSentAtMS           *int64
	CancelAckAtMS            *int64
	AcceptedStaleEpochOutput bool
}

// InvocationOutcomeEvidence records normalized provider/external invocation outcomes.
type InvocationOutcomeEvidence struct {
	ProviderInvocationID     string
	Modality                 string
	ProviderID               string
	OutcomeClass             string
	Retryable                bool
	RetryDecision            string
	AttemptCount             int
	FinalAttemptLatencyMS    int64
	TotalInvocationLatencyMS int64
}

// Validate enforces invocation evidence normalization fields.
func (e InvocationOutcomeEvidence) Validate() error {
	if e.ProviderInvocationID == "" || e.Modality == "" || e.ProviderID == "" {
		return fmt.Errorf("provider_invocation_id, modality, and provider_id are required")
	}
	if !inStringSet(e.Modality, []string{"stt", "llm", "tts", "external"}) {
		return fmt.Errorf("invalid invocation modality: %s", e.Modality)
	}
	if !inStringSet(e.OutcomeClass, []string{"success", "timeout", "overload", "blocked", "infrastructure_failure", "cancelled"}) {
		return fmt.Errorf("invalid invocation outcome_class: %s", e.OutcomeClass)
	}
	if !inStringSet(e.RetryDecision, []string{"none", "retry", "provider_switch", "fallback"}) {
		return fmt.Errorf("invalid invocation retry_decision: %s", e.RetryDecision)
	}
	if e.AttemptCount < 1 {
		return fmt.Errorf("invocation attempt_count must be >=1")
	}
	if e.FinalAttemptLatencyMS < 0 {
		return fmt.Errorf("invocation final_attempt_latency_ms must be >=0")
	}
	if e.TotalInvocationLatencyMS < 0 {
		return fmt.Errorf("invocation total_invocation_latency_ms must be >=0")
	}
	if e.TotalInvocationLatencyMS < e.FinalAttemptLatencyMS {
		return fmt.Errorf("invocation total_invocation_latency_ms must be >= final_attempt_latency_ms")
	}
	return nil
}

// ProviderAttemptEvidence captures per-attempt RK-11 invocation evidence.
type ProviderAttemptEvidence struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	ProviderInvocationID string
	Modality             string
	ProviderID           string
	Attempt              int
	OutcomeClass         string
	Retryable            bool
	RetryDecision        string
	AttemptLatencyMS     int64
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	OutcomeReason        string
	CircuitOpen          bool
	BackoffMS            int64
	InputPayload         string
	OutputPayload        string
	OutputStatusCode     int
	PayloadTruncated     bool
	StreamingUsed        bool
	ChunkCount           int
	BytesOut             int64
	FirstChunkLatencyMS  int64
}

// HandoffEdgeEvidence captures orchestration-level streaming handoff timings.
type HandoffEdgeEvidence struct {
	SessionID             string
	TurnID                string
	PipelineVersion       string
	EventID               string
	HandoffID             string
	Edge                  string
	UpstreamRevision      int
	Action                string
	PartialAcceptedAtMS   int64
	DownstreamStartedAtMS int64
	HandoffLatencyMS      int64
	QueueDepth            int
	WatermarkHigh         bool
	RuntimeTimestampMS    int64
	WallClockTimestampMS  int64
}

// Validate enforces streaming handoff evidence invariants.
func (e HandoffEdgeEvidence) Validate() error {
	if e.SessionID == "" || e.PipelineVersion == "" || e.EventID == "" || e.HandoffID == "" {
		return fmt.Errorf("session_id, pipeline_version, event_id, and handoff_id are required")
	}
	if !inStringSet(e.Edge, []string{"stt_to_llm", "llm_to_tts"}) {
		return fmt.Errorf("invalid handoff edge: %s", e.Edge)
	}
	if !inStringSet(e.Action, []string{"forward", "coalesce", "supersede", "final_fallback"}) {
		return fmt.Errorf("invalid handoff action: %s", e.Action)
	}
	if e.UpstreamRevision < 1 {
		return fmt.Errorf("upstream_revision must be >=1")
	}
	if e.PartialAcceptedAtMS < 0 || e.DownstreamStartedAtMS < 0 || e.HandoffLatencyMS < 0 {
		return fmt.Errorf("handoff timestamps and latency must be >=0")
	}
	if e.DownstreamStartedAtMS < e.PartialAcceptedAtMS {
		return fmt.Errorf("downstream_started_at_ms must be >= partial_accepted_at_ms")
	}
	if e.QueueDepth < 0 {
		return fmt.Errorf("queue_depth must be >=0")
	}
	if e.RuntimeTimestampMS < 0 || e.WallClockTimestampMS < 0 {
		return fmt.Errorf("handoff runtime/wall timestamps must be >=0")
	}
	return nil
}

// Validate enforces per-attempt evidence invariants.
func (e ProviderAttemptEvidence) Validate() error {
	if e.SessionID == "" || e.PipelineVersion == "" || e.EventID == "" || e.ProviderInvocationID == "" {
		return fmt.Errorf("session_id, pipeline_version, event_id, and provider_invocation_id are required")
	}
	if e.ProviderID == "" {
		return fmt.Errorf("provider_id is required")
	}
	if !inStringSet(e.Modality, []string{"stt", "llm", "tts", "external"}) {
		return fmt.Errorf("invalid provider attempt modality: %s", e.Modality)
	}
	if !inStringSet(e.OutcomeClass, []string{"success", "timeout", "overload", "blocked", "infrastructure_failure", "cancelled"}) {
		return fmt.Errorf("invalid provider attempt outcome_class: %s", e.OutcomeClass)
	}
	if !inStringSet(e.RetryDecision, []string{"none", "retry", "provider_switch", "fallback"}) {
		return fmt.Errorf("invalid provider attempt retry_decision: %s", e.RetryDecision)
	}
	if e.AttemptLatencyMS < 0 {
		return fmt.Errorf("provider attempt latency_ms must be >=0")
	}
	if e.Attempt < 1 {
		return fmt.Errorf("provider attempt must be >=1")
	}
	if e.TransportSequence < 0 || e.RuntimeSequence < 0 || e.AuthorityEpoch < 0 {
		return fmt.Errorf("provider attempt sequence fields must be >=0")
	}
	if e.RuntimeTimestampMS < 0 || e.WallClockTimestampMS < 0 {
		return fmt.Errorf("provider attempt timestamps must be >=0")
	}
	if e.BackoffMS < 0 {
		return fmt.Errorf("provider attempt backoff_ms must be >=0")
	}
	if e.OutputStatusCode < 0 {
		return fmt.Errorf("provider attempt output_status_code must be >=0")
	}
	if e.ChunkCount < 0 {
		return fmt.Errorf("provider attempt chunk_count must be >=0")
	}
	if e.BytesOut < 0 {
		return fmt.Errorf("provider attempt bytes_out must be >=0")
	}
	if e.FirstChunkLatencyMS < 0 {
		return fmt.Errorf("provider attempt first_chunk_latency_ms must be >=0")
	}
	return nil
}

// InvocationSnapshotEvidence captures optional non-terminal invocation snapshots.
type InvocationSnapshotEvidence struct {
	SessionID                string
	TurnID                   string
	PipelineVersion          string
	EventID                  string
	ProviderInvocationID     string
	Modality                 string
	ProviderID               string
	OutcomeClass             string
	Retryable                bool
	RetryDecision            string
	AttemptCount             int
	FinalAttemptLatencyMS    int64
	TotalInvocationLatencyMS int64
	RuntimeTimestampMS       int64
	WallClockTimestampMS     int64
}

// Validate enforces invocation snapshot invariants.
func (e InvocationSnapshotEvidence) Validate() error {
	if e.SessionID == "" || e.PipelineVersion == "" || e.EventID == "" {
		return fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	invocation := InvocationOutcomeEvidence{
		ProviderInvocationID:     e.ProviderInvocationID,
		Modality:                 e.Modality,
		ProviderID:               e.ProviderID,
		OutcomeClass:             e.OutcomeClass,
		Retryable:                e.Retryable,
		RetryDecision:            e.RetryDecision,
		AttemptCount:             e.AttemptCount,
		FinalAttemptLatencyMS:    e.FinalAttemptLatencyMS,
		TotalInvocationLatencyMS: e.TotalInvocationLatencyMS,
	}
	if err := invocation.Validate(); err != nil {
		return err
	}
	if e.RuntimeTimestampMS < 0 || e.WallClockTimestampMS < 0 {
		return fmt.Errorf("invocation snapshot timestamps must be >=0")
	}
	return nil
}

// ValidateCompleteness enforces the L0 baseline requirements.
func (b BaselineEvidence) ValidateCompleteness() error {
	if b.SessionID == "" || b.TurnID == "" || b.PipelineVersion == "" || b.EventID == "" {
		return fmt.Errorf("session_id, turn_id, pipeline_version, and event_id are required")
	}
	if b.EnvelopeSnapshot == "" {
		return fmt.Errorf("envelope snapshot is required")
	}
	if len(b.PayloadTags) < 1 {
		return fmt.Errorf("at least one payload classification tag is required")
	}
	for _, tag := range b.PayloadTags {
		if !isPayloadTag(tag) {
			return fmt.Errorf("invalid payload classification tag: %s", tag)
		}
	}
	if len(b.RedactionDecisions) < 1 {
		return fmt.Errorf("at least one redaction decision is required")
	}
	decisionClasses := map[eventabi.PayloadClass]struct{}{}
	for _, decision := range b.RedactionDecisions {
		if err := decision.Validate(); err != nil {
			return err
		}
		if _, exists := decisionClasses[decision.PayloadClass]; exists {
			return fmt.Errorf("duplicate redaction decision for payload class: %s", decision.PayloadClass)
		}
		decisionClasses[decision.PayloadClass] = struct{}{}
	}
	for _, tag := range b.PayloadTags {
		if _, ok := decisionClasses[tag]; !ok {
			return fmt.Errorf("missing redaction decision for payload class: %s", tag)
		}
	}
	if b.PlanHash == "" {
		return fmt.Errorf("plan hash is required")
	}
	if err := b.SnapshotProvenance.Validate(); err != nil {
		return err
	}
	if len(b.DecisionOutcomes) < 1 {
		return fmt.Errorf("at least one decision outcome is required")
	}
	for _, out := range b.DecisionOutcomes {
		if err := out.Validate(); err != nil {
			return err
		}
	}
	for _, invocation := range b.InvocationOutcomes {
		if err := invocation.Validate(); err != nil {
			return err
		}
	}
	if len(b.OrderingMarkers) < 1 {
		return fmt.Errorf("at least one ordering marker is required")
	}
	seen := map[string]struct{}{}
	for _, marker := range b.OrderingMarkers {
		if marker == "" {
			return fmt.Errorf("ordering markers cannot contain empty values")
		}
		if _, exists := seen[marker]; exists {
			return fmt.Errorf("ordering markers must be unique")
		}
		seen[marker] = struct{}{}
	}
	if b.MergeRuleID == "" || b.MergeRuleVersion == "" {
		return fmt.Errorf("merge rule id/version are required")
	}
	if b.AuthorityEpoch < 0 {
		return fmt.Errorf("authority epoch must be >= 0")
	}
	if b.TerminalOutcome != "commit" && b.TerminalOutcome != "abort" {
		return fmt.Errorf("terminal outcome must be commit or abort")
	}
	if !b.CloseEmitted {
		return fmt.Errorf("close marker is required")
	}
	if b.TerminalOutcome == "abort" && b.TerminalReason == "" {
		return fmt.Errorf("terminal reason is required for abort outcomes")
	}
	if b.CancelAcceptedAtMS != nil {
		if b.CancelFenceAppliedAtMS == nil {
			return fmt.Errorf("cancel fence timestamp is required when cancel was accepted")
		}
		if b.CancelSentAtMS == nil {
			return fmt.Errorf("cancel_sent_at is required when cancel was accepted")
		}
	}
	return nil
}

// IsAcceptedTurn returns true when the turn passed pre-turn gating.
func (b BaselineEvidence) IsAcceptedTurn() bool {
	return b.TurnOpenAtMS != nil
}

func isPayloadTag(tag eventabi.PayloadClass) bool {
	switch tag {
	case eventabi.PayloadPII, eventabi.PayloadPHI, eventabi.PayloadAudioRaw, eventabi.PayloadTextRaw, eventabi.PayloadDerivedSummary, eventabi.PayloadMetadata:
		return true
	default:
		return false
	}
}

func inStringSet(value string, set []string) bool {
	for _, candidate := range set {
		if value == candidate {
			return true
		}
	}
	return false
}

// DetailEvent captures non-critical Stage-A detail that may be dropped.
type DetailEvent struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	Reason               string
}

// DetailAppendResult describes detail-append behavior under pressure.
type DetailAppendResult struct {
	Stored             bool
	Dropped            bool
	DowngradeEmitted   bool
	DowngradeSignal    *eventabi.ControlSignal
	DroppedDetailCount int
}

// CompletenessReport summarizes accepted-turn baseline coverage.
type CompletenessReport struct {
	TotalAcceptedTurns        int
	CompleteAcceptedTurns     int
	IncompleteAcceptedTurnIDs []string
	CompletenessRatio         float64
}

// Recorder is the OR-02 Stage-A in-memory append recorder.
type Recorder struct {
	cfg             StageAConfig
	mu              sync.Mutex
	baselineEntries []BaselineEvidence
	detailEntries   []DetailEvent
	attemptEntries  []ProviderAttemptEvidence
	handoffEntries  []HandoffEdgeEvidence
	snapshotEntries []InvocationSnapshotEvidence
	droppedDetails  int
	downgradeByTurn map[string]bool
}

// NewRecorder constructs a recorder with bounded capacities.
func NewRecorder(cfg StageAConfig) Recorder {
	if cfg.BaselineCapacity < 1 {
		cfg.BaselineCapacity = 128
	}
	if cfg.DetailCapacity < 1 {
		cfg.DetailCapacity = 512
	}
	if cfg.AttemptCapacity < 1 {
		cfg.AttemptCapacity = 1024
	}
	if cfg.HandoffCapacity < 1 {
		cfg.HandoffCapacity = 1024
	}
	if cfg.InvocationSnapshotCap < 1 {
		cfg.InvocationSnapshotCap = 1024
	}
	return Recorder{
		cfg:             cfg,
		downgradeByTurn: make(map[string]bool),
	}
}

// AppendBaseline appends replay-critical evidence without blocking.
func (r *Recorder) AppendBaseline(evidence BaselineEvidence) error {
	if err := evidence.ValidateCompleteness(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.baselineEntries) >= r.cfg.BaselineCapacity {
		return ErrBaselineCapacityExhausted
	}
	r.baselineEntries = append(r.baselineEntries, evidence)
	return nil
}

// AppendProviderInvocationAttempts appends RK-11 per-attempt evidence.
func (r *Recorder) AppendProviderInvocationAttempts(attempts []ProviderAttemptEvidence) error {
	if len(attempts) == 0 {
		return nil
	}

	for _, attempt := range attempts {
		if err := attempt.Validate(); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.attemptEntries)+len(attempts) > r.cfg.AttemptCapacity {
		return ErrProviderAttemptCapacityExhausted
	}
	r.attemptEntries = append(r.attemptEntries, attempts...)
	return nil
}

// AppendHandoffEdges appends orchestration-level streaming handoff evidence.
func (r *Recorder) AppendHandoffEdges(edges []HandoffEdgeEvidence) error {
	if len(edges) == 0 {
		return nil
	}
	for _, edge := range edges {
		if err := edge.Validate(); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.handoffEntries)+len(edges) > r.cfg.HandoffCapacity {
		return ErrHandoffCapacityExhausted
	}
	r.handoffEntries = append(r.handoffEntries, edges...)
	return nil
}

// AppendInvocationSnapshot appends optional non-terminal invocation snapshot evidence.
func (r *Recorder) AppendInvocationSnapshot(snapshot InvocationSnapshotEvidence) error {
	if !r.cfg.EnableInvocationSnapshot {
		return nil
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.snapshotEntries) >= r.cfg.InvocationSnapshotCap {
		return ErrInvocationSnapshotCapacityExhausted
	}
	r.snapshotEntries = append(r.snapshotEntries, snapshot)
	return nil
}

// AppendDetail appends best-effort Stage-A detail and deterministically drops on overflow.
func (r *Recorder) AppendDetail(detail DetailEvent) (DetailAppendResult, error) {
	if detail.SessionID == "" || detail.PipelineVersion == "" || detail.EventID == "" {
		return DetailAppendResult{}, fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	if detail.RuntimeTimestampMS < 0 || detail.WallClockTimestampMS < 0 {
		return DetailAppendResult{}, fmt.Errorf("timestamps must be >= 0")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.detailEntries) < r.cfg.DetailCapacity {
		r.detailEntries = append(r.detailEntries, detail)
		return DetailAppendResult{Stored: true, DroppedDetailCount: r.droppedDetails}, nil
	}

	r.droppedDetails++
	key := detail.SessionID + "/" + detail.TurnID
	if !r.downgradeByTurn[key] {
		r.downgradeByTurn[key] = true
		signal, err := buildRecordingDowngradeSignal(detail)
		if err != nil {
			return DetailAppendResult{}, err
		}
		return DetailAppendResult{
			Dropped:            true,
			DowngradeEmitted:   true,
			DowngradeSignal:    &signal,
			DroppedDetailCount: r.droppedDetails,
		}, nil
	}

	return DetailAppendResult{Dropped: true, DroppedDetailCount: r.droppedDetails}, nil
}

func buildRecordingDowngradeSignal(detail DetailEvent) (eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if detail.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transportSequence := detail.TransportSequence
	if transportSequence < 0 {
		transportSequence = 0
	}
	runtimeSequence := detail.RuntimeSequence
	if runtimeSequence < 0 {
		runtimeSequence = 0
	}
	authorityEpoch := detail.AuthorityEpoch
	if authorityEpoch < 0 {
		authorityEpoch = 0
	}

	signal := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          detail.SessionID,
		TurnID:             detail.TurnID,
		PipelineVersion:    detail.PipelineVersion,
		EventID:            detail.EventID,
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transportSequence,
		RuntimeSequence:    runtimeSequence,
		AuthorityEpoch:     authorityEpoch,
		RuntimeTimestampMS: detail.RuntimeTimestampMS,
		WallClockMS:        detail.WallClockTimestampMS,
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "recording_level_downgraded",
		EmittedBy:          "OR-02",
		Reason:             fallbackReason(detail.Reason),
		Scope:              scope,
	}
	if err := signal.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return signal, nil
}

func fallbackReason(reason string) string {
	if reason == "" {
		return "stage_a_detail_overflow"
	}
	return reason
}

// BaselineEntries returns a stable copy of baseline entries.
func (r *Recorder) BaselineEntries() []BaselineEvidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]BaselineEvidence, len(r.baselineEntries))
	copy(out, r.baselineEntries)
	return out
}

// DetailEntries returns a stable copy of detail entries.
func (r *Recorder) DetailEntries() []DetailEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]DetailEvent, len(r.detailEntries))
	copy(out, r.detailEntries)
	return out
}

// ProviderAttemptEntries returns a stable copy of provider attempt entries.
func (r *Recorder) ProviderAttemptEntries() []ProviderAttemptEvidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ProviderAttemptEvidence, len(r.attemptEntries))
	copy(out, r.attemptEntries)
	return out
}

// ProviderAttemptEntriesForTurn returns provider attempts for one session/turn pair.
func (r *Recorder) ProviderAttemptEntriesForTurn(sessionID, turnID string) []ProviderAttemptEvidence {
	if sessionID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := make([]ProviderAttemptEvidence, 0)
	for _, entry := range r.attemptEntries {
		if entry.SessionID != sessionID {
			continue
		}
		if turnID != "" && entry.TurnID != turnID {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// HandoffEntries returns a stable copy of orchestration-level handoff entries.
func (r *Recorder) HandoffEntries() []HandoffEdgeEvidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]HandoffEdgeEvidence, len(r.handoffEntries))
	copy(out, r.handoffEntries)
	return out
}

// HandoffEntriesForTurn returns handoff entries for one session/turn pair.
func (r *Recorder) HandoffEntriesForTurn(sessionID, turnID string) []HandoffEdgeEvidence {
	if sessionID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := make([]HandoffEdgeEvidence, 0)
	for _, entry := range r.handoffEntries {
		if entry.SessionID != sessionID {
			continue
		}
		if turnID != "" && entry.TurnID != turnID {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// InvocationSnapshotEntries returns a stable copy of invocation snapshots.
func (r *Recorder) InvocationSnapshotEntries() []InvocationSnapshotEvidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]InvocationSnapshotEvidence, len(r.snapshotEntries))
	copy(out, r.snapshotEntries)
	return out
}

// InvocationSnapshotEntriesForTurn returns invocation snapshots for one session/turn pair.
func (r *Recorder) InvocationSnapshotEntriesForTurn(sessionID, turnID string) []InvocationSnapshotEvidence {
	if sessionID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := make([]InvocationSnapshotEvidence, 0)
	for _, entry := range r.snapshotEntries {
		if entry.SessionID != sessionID {
			continue
		}
		if turnID != "" && entry.TurnID != turnID {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// DroppedDetailCount reports deterministic detail drop count.
func (r *Recorder) DroppedDetailCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.droppedDetails
}

// BaselineCompleteness computes accepted-turn L0 completeness ratio.
func BaselineCompleteness(entries []BaselineEvidence) CompletenessReport {
	report := CompletenessReport{}
	incompleteIDs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsAcceptedTurn() {
			continue
		}
		report.TotalAcceptedTurns++
		if err := entry.ValidateCompleteness(); err != nil {
			incompleteIDs = append(incompleteIDs, entry.TurnID)
			continue
		}
		report.CompleteAcceptedTurns++
	}
	sort.Strings(incompleteIDs)
	report.IncompleteAcceptedTurnIDs = incompleteIDs
	if report.TotalAcceptedTurns > 0 {
		report.CompletenessRatio = float64(report.CompleteAcceptedTurns) / float64(report.TotalAcceptedTurns)
	}
	return report
}

// InvocationOutcomesFromProviderAttempts synthesizes OR-02 invocation outcomes from RK-11 attempt evidence.
func InvocationOutcomesFromProviderAttempts(attempts []ProviderAttemptEvidence) ([]InvocationOutcomeEvidence, error) {
	if len(attempts) == 0 {
		return nil, nil
	}

	byInvocationID := make(map[string][]ProviderAttemptEvidence, len(attempts))
	for _, attempt := range attempts {
		if err := attempt.Validate(); err != nil {
			return nil, err
		}
		byInvocationID[attempt.ProviderInvocationID] = append(byInvocationID[attempt.ProviderInvocationID], attempt)
	}

	ids := make([]string, 0, len(byInvocationID))
	for providerInvocationID := range byInvocationID {
		ids = append(ids, providerInvocationID)
	}
	sort.Strings(ids)

	outcomes := make([]InvocationOutcomeEvidence, 0, len(ids))
	for _, providerInvocationID := range ids {
		group := append([]ProviderAttemptEvidence(nil), byInvocationID[providerInvocationID]...)
		sort.SliceStable(group, func(i, j int) bool {
			return providerAttemptLess(group[i], group[j])
		})
		final := group[len(group)-1]
		retryDecision := final.RetryDecision
		if retryDecision == "" {
			retryDecision = "none"
		}

		outcome := InvocationOutcomeEvidence{
			ProviderInvocationID:     providerInvocationID,
			Modality:                 final.Modality,
			ProviderID:               final.ProviderID,
			OutcomeClass:             final.OutcomeClass,
			Retryable:                final.Retryable,
			RetryDecision:            retryDecision,
			AttemptCount:             len(group),
			FinalAttemptLatencyMS:    deriveFinalAttemptLatencyMS(group),
			TotalInvocationLatencyMS: deriveTotalInvocationLatencyMS(group),
		}
		if err := outcome.Validate(); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

func providerAttemptLess(a, b ProviderAttemptEvidence) bool {
	if a.RuntimeTimestampMS != b.RuntimeTimestampMS {
		return a.RuntimeTimestampMS < b.RuntimeTimestampMS
	}
	if a.WallClockTimestampMS != b.WallClockTimestampMS {
		return a.WallClockTimestampMS < b.WallClockTimestampMS
	}
	if a.RuntimeSequence != b.RuntimeSequence {
		return a.RuntimeSequence < b.RuntimeSequence
	}
	if a.TransportSequence != b.TransportSequence {
		return a.TransportSequence < b.TransportSequence
	}
	if a.Attempt != b.Attempt {
		return a.Attempt < b.Attempt
	}
	if a.ProviderID != b.ProviderID {
		return a.ProviderID < b.ProviderID
	}
	return a.EventID < b.EventID
}

func deriveFinalAttemptLatencyMS(group []ProviderAttemptEvidence) int64 {
	if len(group) == 0 {
		return 0
	}
	final := group[len(group)-1]
	if final.AttemptLatencyMS > 0 {
		return final.AttemptLatencyMS
	}
	if len(group) == 1 {
		return 0
	}
	prev := group[len(group)-2]
	if final.WallClockTimestampMS > prev.WallClockTimestampMS {
		return final.WallClockTimestampMS - prev.WallClockTimestampMS
	}
	return 0
}

func deriveTotalInvocationLatencyMS(group []ProviderAttemptEvidence) int64 {
	if len(group) == 0 {
		return 0
	}
	if len(group) == 1 {
		if group[0].AttemptLatencyMS < 0 {
			return 0
		}
		return group[0].AttemptLatencyMS
	}
	first := group[0]
	last := group[len(group)-1]
	if last.WallClockTimestampMS > first.WallClockTimestampMS {
		return last.WallClockTimestampMS - first.WallClockTimestampMS
	}
	return 0
}
