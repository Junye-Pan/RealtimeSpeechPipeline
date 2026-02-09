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
)

// StageAConfig defines bounded Stage-A append capacities.
type StageAConfig struct {
	BaselineCapacity int
	DetailCapacity   int
}

// BaselineEvidence holds replay-critical OR-02 Stage-A evidence.
type BaselineEvidence struct {
	SessionID                string
	TurnID                   string
	PipelineVersion          string
	EventID                  string
	EnvelopeSnapshot         string
	PayloadTags              []eventabi.PayloadClass
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
	ProviderInvocationID string
	Modality             string
	ProviderID           string
	OutcomeClass         string
	Retryable            bool
	RetryDecision        string
	AttemptCount         int
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
