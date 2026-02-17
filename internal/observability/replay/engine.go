package replay

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/observability"
)

var (
	ErrReplayArtifactResolverRequired = errors.New("replay artifact resolver is required")
	ErrReplayCursorNotFound           = errors.New("replay cursor boundary not found in baseline trace")
)

// ReplayArtifact contains replay-comparable evidence resolved from artifact refs.
type ReplayArtifact struct {
	TraceArtifacts   []TraceArtifact
	DecisionOutcomes []controlplane.DecisionOutcome
	LineageRecords   []LineageRecord
}

// Clone returns a detached copy of replay artifact slices.
func (a ReplayArtifact) Clone() ReplayArtifact {
	cloned := ReplayArtifact{
		TraceArtifacts:   append([]TraceArtifact(nil), a.TraceArtifacts...),
		DecisionOutcomes: append([]controlplane.DecisionOutcome(nil), a.DecisionOutcomes...),
		LineageRecords:   append([]LineageRecord(nil), a.LineageRecords...),
	}
	return cloned
}

// ReplayArtifactResolver resolves replay artifacts from stable references.
type ReplayArtifactResolver interface {
	ResolveReplayArtifact(ref string) (ReplayArtifact, error)
}

// StaticReplayArtifactResolver resolves refs from an in-memory map.
type StaticReplayArtifactResolver struct {
	Artifacts map[string]ReplayArtifact
}

// ResolveReplayArtifact resolves replay artifacts deterministically by key.
func (r StaticReplayArtifactResolver) ResolveReplayArtifact(ref string) (ReplayArtifact, error) {
	artifact, ok := r.Artifacts[ref]
	if !ok {
		return ReplayArtifact{}, fmt.Errorf("resolve replay artifact %q: not found", ref)
	}
	return artifact.Clone(), nil
}

// Engine executes replay runs with explicit mode and deterministic cursor behavior.
type Engine struct {
	ArtifactResolver ReplayArtifactResolver
	CompareConfig    CompareConfig
	NowMS            func() int64
	NewRunID         func() string
}

// Run executes replay using resolved baseline/candidate artifacts.
func (e Engine) Run(req observability.ReplayRunRequest) (observability.ReplayRunResult, error) {
	if err := req.Validate(); err != nil {
		return observability.ReplayRunResult{}, fmt.Errorf("invalid replay run request: %w", err)
	}
	if e.ArtifactResolver == nil {
		return observability.ReplayRunResult{}, ErrReplayArtifactResolverRequired
	}

	baselineArtifact, err := e.ArtifactResolver.ResolveReplayArtifact(req.BaselineRef)
	if err != nil {
		return observability.ReplayRunResult{}, fmt.Errorf("resolve baseline artifact: %w", err)
	}
	candidateArtifact, err := e.ArtifactResolver.ResolveReplayArtifact(req.CandidatePlanRef)
	if err != nil {
		return observability.ReplayRunResult{}, fmt.Errorf("resolve candidate artifact: %w", err)
	}

	baselineTrace := normalizeTraceArtifacts(baselineArtifact.TraceArtifacts)
	candidateTrace := normalizeTraceArtifacts(candidateArtifact.TraceArtifacts)

	baselineStart, candidateStart, cursorDivergences, err := resolveCursorStartOffsets(req.Cursor, baselineTrace, candidateTrace)
	if err != nil {
		return observability.ReplayRunResult{}, err
	}
	if baselineStart > len(baselineTrace) {
		baselineStart = len(baselineTrace)
	}
	if candidateStart > len(candidateTrace) {
		candidateStart = len(candidateTrace)
	}
	baselineTrace = baselineTrace[baselineStart:]
	candidateTrace = candidateTrace[candidateStart:]

	divergences := append([]observability.ReplayDivergence(nil), cursorDivergences...)
	switch req.Mode {
	case observability.ReplayModeReplayDecisions:
		divergences = append(divergences, CompareDecisionOutcomes(
			selectDecisionOutcomes(baselineArtifact, baselineTrace, baselineStart),
			selectDecisionOutcomes(candidateArtifact, candidateTrace, candidateStart),
		)...)
	case observability.ReplayModeRecomputeDecisions, observability.ReplayModeReSimulateNodes, observability.ReplayModePlaybackRecordedProvider:
		divergences = append(divergences, CompareTraceArtifacts(baselineTrace, candidateTrace, e.CompareConfig)...)
	default:
		return observability.ReplayRunResult{}, fmt.Errorf("unsupported replay mode: %s", req.Mode)
	}

	if len(baselineArtifact.LineageRecords) > 0 || len(candidateArtifact.LineageRecords) > 0 {
		divergences = append(divergences, CompareLineageRecords(
			baselineArtifact.LineageRecords,
			candidateArtifact.LineageRecords,
		)...)
	}

	result := observability.ReplayRunResult{
		RunID:       e.newRunID(),
		Mode:        req.Mode,
		Divergences: divergences,
		Cursor:      resultCursorForTrace(req.Cursor, baselineTrace),
	}
	if err := result.Validate(); err != nil {
		return observability.ReplayRunResult{}, fmt.Errorf("invalid replay run result: %w", err)
	}
	return result, nil
}

func normalizeTraceArtifacts(trace []TraceArtifact) []TraceArtifact {
	if len(trace) == 0 {
		return nil
	}
	normalized := make([]TraceArtifact, len(trace))
	for i, artifact := range trace {
		entry := artifact
		entry.Lane = normalizeReplayLane(entry.Lane)
		entry.RuntimeSequence = traceRuntimeSequence(entry, int64(i))
		if strings.TrimSpace(entry.OrderingMarker) == "" {
			entry.OrderingMarker = fmt.Sprintf("runtime_sequence:%d", entry.RuntimeSequence)
		}
		normalized[i] = entry
	}
	return normalized
}

func selectDecisionOutcomes(artifact ReplayArtifact, trace []TraceArtifact, start int) []controlplane.DecisionOutcome {
	if len(trace) > 0 {
		outcomes := make([]controlplane.DecisionOutcome, 0, len(trace))
		for _, item := range trace {
			outcomes = append(outcomes, item.Decision)
		}
		return outcomes
	}
	if len(artifact.DecisionOutcomes) == 0 {
		return nil
	}
	if start >= len(artifact.DecisionOutcomes) {
		return nil
	}
	return append([]controlplane.DecisionOutcome(nil), artifact.DecisionOutcomes[start:]...)
}

func resultCursorForTrace(requestCursor *observability.ReplayCursor, trace []TraceArtifact) *observability.ReplayCursor {
	if len(trace) == 0 {
		if requestCursor == nil {
			return nil
		}
		cursor := *requestCursor
		return &cursor
	}
	last := trace[len(trace)-1]
	cursor := observability.ReplayCursor{
		SessionID:       last.Decision.SessionID,
		TurnID:          last.Decision.TurnID,
		Lane:            normalizeReplayLane(last.Lane),
		RuntimeSequence: traceRuntimeSequence(last, int64(len(trace)-1)),
		EventID:         last.Decision.EventID,
	}
	if err := cursor.Validate(); err != nil {
		return nil
	}
	return &cursor
}

func resolveCursorStartOffsets(
	cursor *observability.ReplayCursor,
	baseline []TraceArtifact,
	candidate []TraceArtifact,
) (int, int, []observability.ReplayDivergence, error) {
	if cursor == nil {
		return 0, 0, nil, nil
	}

	baselineIndex, ok := findTraceCursorIndex(baseline, *cursor)
	if !ok {
		return 0, 0, nil, fmt.Errorf("%w: session=%s turn=%s lane=%s runtime_sequence=%d event_id=%s", ErrReplayCursorNotFound, cursor.SessionID, cursor.TurnID, cursor.Lane, cursor.RuntimeSequence, cursor.EventID)
	}
	candidateIndex, ok := findTraceCursorIndex(candidate, *cursor)
	if ok {
		return baselineIndex, candidateIndex, nil, nil
	}
	if baselineIndex <= len(candidate) {
		return baselineIndex, baselineIndex, []observability.ReplayDivergence{{
			Class:   observability.OrderingDivergence,
			Scope:   "turn:" + cursor.TurnID,
			Message: "replay cursor boundary missing in candidate trace; applied baseline ordinal resume boundary",
		}}, nil
	}
	return baselineIndex, len(candidate), []observability.ReplayDivergence{{
		Class:   observability.OrderingDivergence,
		Scope:   "turn:" + cursor.TurnID,
		Message: "replay cursor boundary missing in candidate trace; candidate trace exhausted at resume boundary",
	}}, nil
}

func findTraceCursorIndex(trace []TraceArtifact, cursor observability.ReplayCursor) (int, bool) {
	for i, entry := range trace {
		if entry.Decision.SessionID != cursor.SessionID {
			continue
		}
		if entry.Decision.TurnID != cursor.TurnID {
			continue
		}
		if entry.Decision.EventID != cursor.EventID {
			continue
		}
		if normalizeReplayLane(entry.Lane) != cursor.Lane {
			continue
		}
		if traceRuntimeSequence(entry, int64(i)) != cursor.RuntimeSequence {
			continue
		}
		return i, true
	}
	return 0, false
}

func (e Engine) newRunID() string {
	if e.NewRunID != nil {
		return e.NewRunID()
	}
	return fmt.Sprintf("replay-run-%d", e.nowMS())
}

func (e Engine) nowMS() int64 {
	if e.NowMS != nil {
		return e.NowMS()
	}
	return time.Now().UnixMilli()
}
