package invocation

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	telemetrycontext "github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry/context"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	providerpolicy "github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
)

// Config controls deterministic RK-11 invocation behavior.
type Config struct {
	MaxAttemptsPerProvider int
	MaxCandidateProviders  int
	EnableStreaming        bool
}

// StreamEventHooks allows callers to observe streaming chunks in real time.
// Hooks are invoked only on native streaming paths.
type StreamEventHooks struct {
	OnStart    func(contracts.StreamChunk) error
	OnChunk    func(contracts.StreamChunk) error
	OnComplete func(contracts.StreamChunk) error
	OnError    func(contracts.StreamChunk) error
}

// Controller executes deterministic provider invocation attempts.
type Controller struct {
	catalog registry.Catalog
	cfg     Config
}

// InvocationInput carries scheduler-side context into RK-11.
type InvocationInput struct {
	SessionID                string
	TurnID                   string
	PipelineVersion          string
	EventID                  string
	Modality                 contracts.Modality
	PreferredProvider        string
	AllowedAdaptiveActions   []string
	ProviderInvocationID     string
	TransportSequence        int64
	RuntimeSequence          int64
	AuthorityEpoch           int64
	RuntimeTimestampMS       int64
	WallClockTimestampMS     int64
	NodeID                   string
	EdgeID                   string
	CancelRequested          bool
	EnableStreaming          bool
	DisableProviderStreaming bool
	StreamHooks              StreamEventHooks
	ResolvedProviderPlan     *providerpolicy.ResolvedProviderPlan
}

// InvocationAttempt records one provider attempt with normalized outcome.
type InvocationAttempt struct {
	ProviderID          string
	Attempt             int
	Outcome             contracts.Outcome
	StreamingUsed       bool
	ChunkCount          int
	BytesOut            int64
	FirstChunkLatencyMS int64
	AttemptLatencyMS    int64
}

// InvocationResult summarizes deterministic invocation behavior.
type InvocationResult struct {
	ProviderInvocationID  string
	SelectedProvider      string
	Outcome               contracts.Outcome
	RetryDecision         string
	Attempts              []InvocationAttempt
	Signals               []eventabi.ControlSignal
	StreamingUsed         bool
	PolicySnapshotRef     string
	CapabilitySnapshotRef string
	RoutingReason         string
	SignalSource          string
}

// NewController returns a controller with defaults suitable for MVP.
func NewController(catalog registry.Catalog) Controller {
	return NewControllerWithConfig(catalog, Config{})
}

// NewControllerWithConfig builds a controller with explicit limits.
func NewControllerWithConfig(catalog registry.Catalog, cfg Config) Controller {
	if cfg.MaxAttemptsPerProvider < 1 {
		cfg.MaxAttemptsPerProvider = 2
	}
	if cfg.MaxCandidateProviders < 1 {
		cfg.MaxCandidateProviders = 5
	}
	return Controller{catalog: catalog, cfg: cfg}
}

// Invoke executes deterministic provider attempt/retry/switch behavior.
func (c Controller) Invoke(in InvocationInput) (InvocationResult, error) {
	if err := validateInput(in); err != nil {
		return InvocationResult{}, err
	}

	plan, err := c.resolveInvocationPlan(in)
	if err != nil {
		return InvocationResult{}, err
	}
	candidates := plan.candidates

	result := InvocationResult{
		ProviderInvocationID:  providerInvocationID(in),
		RetryDecision:         "none",
		Attempts:              make([]InvocationAttempt, 0, plan.maxAttemptsPerProvider*len(candidates)),
		Signals:               make([]eventabi.ControlSignal, 0),
		PolicySnapshotRef:     plan.policySnapshotRef,
		CapabilitySnapshotRef: plan.capabilitySnapshotRef,
		RoutingReason:         plan.routingReason,
		SignalSource:          plan.signalSource,
	}
	baseCorrelation, err := telemetrycontext.Resolve(telemetrycontext.ResolveInput{
		SessionID:            in.SessionID,
		TurnID:               in.TurnID,
		EventID:              in.EventID,
		PipelineVersion:      in.PipelineVersion,
		NodeID:               in.NodeID,
		EdgeID:               in.EdgeID,
		AuthorityEpoch:       nonNegative(in.AuthorityEpoch),
		Lane:                 eventabi.LaneTelemetry,
		EmittedBy:            "OR-01",
		RuntimeTimestampMS:   nonNegative(in.RuntimeTimestampMS),
		WallClockTimestampMS: nonNegative(in.WallClockTimestampMS),
	})
	if err != nil {
		return InvocationResult{}, err
	}

	if in.CancelRequested {
		result.SelectedProvider = candidates[0].ProviderID()
		result.Outcome = contracts.Outcome{
			Class:     contracts.OutcomeCancelled,
			Retryable: false,
			Reason:    "cancel_requested_before_invoke",
		}
		telemetry.DefaultEmitter().EmitLog(
			"provider_invocation_cancelled",
			"info",
			"provider invocation cancelled before attempt",
			map[string]string{
				"provider_id": result.SelectedProvider,
				"modality":    string(in.Modality),
				"outcome":     string(result.Outcome.Class),
				"node_id":     in.NodeID,
				"edge_id":     in.EdgeID,
			},
			baseCorrelation,
		)
		return result, nil
	}

	actionSource := in.AllowedAdaptiveActions
	if len(plan.allowedAdaptiveActions) > 0 {
		actionSource = plan.allowedAdaptiveActions
	}
	actions, err := parseAdaptiveActions(actionSource)
	if err != nil {
		return InvocationResult{}, err
	}
	totalAttempts := 0
	totalLatencyMS := int64(0)
	for providerIndex, adapter := range candidates {
		for attempt := 1; attempt <= plan.maxAttemptsPerProvider; attempt++ {
			if plan.budget.MaxTotalAttempts > 0 && totalAttempts >= plan.budget.MaxTotalAttempts {
				return result, nil
			}
			if plan.budget.MaxTotalLatencyMS > 0 && totalLatencyMS >= plan.budget.MaxTotalLatencyMS {
				return result, nil
			}
			req := contracts.InvocationRequest{
				SessionID:              in.SessionID,
				TurnID:                 in.TurnID,
				PipelineVersion:        in.PipelineVersion,
				EventID:                in.EventID,
				ProviderInvocationID:   result.ProviderInvocationID,
				ProviderID:             adapter.ProviderID(),
				Modality:               in.Modality,
				Attempt:                attempt,
				TransportSequence:      nonNegative(in.TransportSequence),
				RuntimeSequence:        nonNegative(in.RuntimeSequence),
				AuthorityEpoch:         nonNegative(in.AuthorityEpoch),
				RuntimeTimestampMS:     nonNegative(in.RuntimeTimestampMS),
				WallClockTimestampMS:   nonNegative(in.WallClockTimestampMS),
				CancelRequested:        in.CancelRequested,
				AllowedAdaptiveActions: append([]string(nil), actions.normalized...),
				RetryBudgetRemaining:   max(0, plan.maxAttemptsPerProvider-attempt),
				CandidateProviderCount: len(candidates),
			}
			attemptStartedAt := time.Now()
			streamingUsed := streamingEnabled(in, c.cfg, adapter.Modality(), adapter)
			attemptStats := streamAttemptStats{}
			var outcome contracts.Outcome
			var invokeErr error
			if streamingUsed {
				streamAdapter := adapter.(contracts.StreamingAdapter)
				outcome, invokeErr = streamAdapter.InvokeStream(req, newInvocationStreamObserver(attemptStartedAt, &attemptStats, in.StreamHooks))
			} else {
				outcome, invokeErr = adapter.Invoke(req)
			}
			attemptLatencyMS := max(0, time.Since(attemptStartedAt).Milliseconds())
			if invokeErr != nil {
				outcome = contracts.Outcome{
					Class:         contracts.OutcomeInfrastructureFailure,
					Retryable:     true,
					Reason:        "adapter_invoke_error",
					OutputPayload: fmt.Sprintf("adapter_invoke_error=%v", invokeErr),
				}
			}
			if err := outcome.Validate(); err != nil {
				return InvocationResult{}, err
			}
			attemptStartMS := nonNegative(in.RuntimeTimestampMS) + int64(attempt-1)
			attemptEndMS := attemptStartMS + attemptLatencyMS
			telemetry.DefaultEmitter().EmitMetric(
				telemetry.MetricProviderRTTMS,
				float64(attemptLatencyMS),
				"ms",
				map[string]string{
					"provider_id": adapter.ProviderID(),
					"modality":    string(in.Modality),
					"attempt":     strconv.Itoa(attempt),
					"outcome":     string(outcome.Class),
					"node_id":     in.NodeID,
					"edge_id":     in.EdgeID,
				},
				correlationWithTimestamps(baseCorrelation, attemptStartMS, nonNegative(in.WallClockTimestampMS)),
			)
			telemetry.DefaultEmitter().EmitSpan(
				"provider_invocation_span",
				"provider_invocation_span",
				attemptStartMS,
				attemptEndMS,
				map[string]string{
					"provider_id": adapter.ProviderID(),
					"modality":    string(in.Modality),
					"attempt":     strconv.Itoa(attempt),
					"outcome":     string(outcome.Class),
					"node_id":     in.NodeID,
					"edge_id":     in.EdgeID,
				},
				correlationWithTimestamps(baseCorrelation, attemptStartMS, nonNegative(in.WallClockTimestampMS)),
			)
			logSeverity := "info"
			if outcome.Class != contracts.OutcomeSuccess {
				logSeverity = "warn"
			}
			telemetry.DefaultEmitter().EmitLog(
				"provider_invocation_attempt",
				logSeverity,
				"provider invocation attempt completed",
				map[string]string{
					"provider_id": adapter.ProviderID(),
					"modality":    string(in.Modality),
					"attempt":     strconv.Itoa(attempt),
					"outcome":     string(outcome.Class),
					"retryable":   strconv.FormatBool(outcome.Retryable),
					"node_id":     in.NodeID,
					"edge_id":     in.EdgeID,
				},
				correlationWithTimestamps(baseCorrelation, attemptEndMS, nonNegative(in.WallClockTimestampMS)),
			)

			result.Attempts = append(result.Attempts, InvocationAttempt{
				ProviderID:          adapter.ProviderID(),
				Attempt:             attempt,
				Outcome:             outcome,
				StreamingUsed:       streamingUsed,
				ChunkCount:          attemptStats.chunkCount,
				BytesOut:            attemptStats.bytesOut,
				FirstChunkLatencyMS: attemptStats.firstChunkLatencyMS,
				AttemptLatencyMS:    attemptLatencyMS,
			})
			totalAttempts++
			totalLatencyMS += attemptLatencyMS
			result.SelectedProvider = adapter.ProviderID()
			result.Outcome = outcome
			result.StreamingUsed = result.StreamingUsed || streamingUsed

			if outcome.Class == contracts.OutcomeSuccess {
				return result, nil
			}

			if err := c.appendSignal(&result, in, "provider_error", normalizeFailureReason(adapter.ProviderID(), outcome)); err != nil {
				return InvocationResult{}, err
			}
			if outcome.CircuitOpen {
				if err := c.appendSignal(&result, in, "circuit_event", fmt.Sprintf("provider=%s class=%s", adapter.ProviderID(), outcome.Class)); err != nil {
					return InvocationResult{}, err
				}
			}

			if outcome.Retryable && actions.retry && attempt < plan.maxAttemptsPerProvider {
				if plan.budget.MaxTotalAttempts > 0 && totalAttempts >= plan.budget.MaxTotalAttempts {
					break
				}
				if plan.budget.MaxTotalLatencyMS > 0 && totalLatencyMS >= plan.budget.MaxTotalLatencyMS {
					break
				}
				result.RetryDecision = "retry"
				continue
			}
			break
		}

		if providerIndex < len(candidates)-1 && (actions.providerSwitch || actions.fallback) {
			nextProvider := candidates[providerIndex+1].ProviderID()
			switchReason := fmt.Sprintf("from=%s to=%s", adapter.ProviderID(), nextProvider)
			if err := c.appendSignal(&result, in, "provider_switch", switchReason); err != nil {
				return InvocationResult{}, err
			}
			if actions.providerSwitch {
				result.RetryDecision = "provider_switch"
			} else {
				result.RetryDecision = "fallback"
			}
			continue
		}
		return result, nil
	}

	return result, nil
}

type invocationPlan struct {
	candidates             []contracts.Adapter
	maxAttemptsPerProvider int
	allowedAdaptiveActions []string
	budget                 providerpolicy.Budget
	policySnapshotRef      string
	capabilitySnapshotRef  string
	routingReason          string
	signalSource           string
}

func (c Controller) resolveInvocationPlan(in InvocationInput) (invocationPlan, error) {
	plan := invocationPlan{
		maxAttemptsPerProvider: c.cfg.MaxAttemptsPerProvider,
		allowedAdaptiveActions: append([]string(nil), in.AllowedAdaptiveActions...),
	}
	if in.ResolvedProviderPlan == nil {
		candidates, err := c.catalog.Candidates(in.Modality, in.PreferredProvider, c.cfg.MaxCandidateProviders)
		if err != nil {
			return invocationPlan{}, err
		}
		plan.candidates = candidates
		return plan, nil
	}

	resolved := in.ResolvedProviderPlan
	if resolved.MaxAttemptsPerProvider > 0 {
		plan.maxAttemptsPerProvider = resolved.MaxAttemptsPerProvider
	}
	if len(resolved.AllowedActions) > 0 {
		plan.allowedAdaptiveActions = append([]string(nil), resolved.AllowedActions...)
	}
	plan.budget = resolved.Budget
	plan.policySnapshotRef = resolved.PolicySnapshotRef
	plan.capabilitySnapshotRef = resolved.CapabilitySnapshotRef
	plan.routingReason = resolved.RoutingReason
	plan.signalSource = resolved.SignalSource

	var (
		candidates []contracts.Adapter
		err        error
	)
	if len(resolved.OrderedCandidates) == 0 {
		preferred := in.PreferredProvider
		if preferred == "" && len(resolved.OrderedCandidates) > 0 {
			preferred = resolved.OrderedCandidates[0]
		}
		candidates, err = c.catalog.Candidates(in.Modality, preferred, c.cfg.MaxCandidateProviders)
		if err != nil {
			return invocationPlan{}, err
		}
	} else {
		candidates, err = c.candidatesFromOrderedIDs(in.Modality, resolved.OrderedCandidates)
		if err != nil {
			return invocationPlan{}, err
		}
	}
	if len(candidates) == 0 {
		return invocationPlan{}, fmt.Errorf("resolved provider plan produced no candidates for modality %s", in.Modality)
	}
	plan.candidates = candidates
	return plan, nil
}

func (c Controller) candidatesFromOrderedIDs(modality contracts.Modality, orderedIDs []string) ([]contracts.Adapter, error) {
	candidates := make([]contracts.Adapter, 0, len(orderedIDs))
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, providerID := range orderedIDs {
		if providerID == "" {
			continue
		}
		if _, exists := seen[providerID]; exists {
			continue
		}
		adapter, ok := c.catalog.Adapter(modality, providerID)
		if !ok {
			continue
		}
		seen[providerID] = struct{}{}
		candidates = append(candidates, adapter)
		if len(candidates) >= c.cfg.MaxCandidateProviders {
			break
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no registered candidates match ordered provider ids")
	}
	return candidates, nil
}

func validateInput(in InvocationInput) error {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EventID == "" {
		return fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	return in.Modality.Validate()
}

type adaptiveActions struct {
	retry          bool
	providerSwitch bool
	fallback       bool
	normalized     []string
}

func parseAdaptiveActions(actions []string) (adaptiveActions, error) {
	out := adaptiveActions{}
	normalized, err := contracts.NormalizeAdaptiveActions(actions)
	if err != nil {
		return out, err
	}
	out.normalized = normalized
	for _, action := range normalized {
		switch action {
		case "retry":
			out.retry = true
		case "provider_switch":
			out.providerSwitch = true
		case "fallback":
			out.fallback = true
		}
	}
	return out, nil
}

func providerInvocationID(in InvocationInput) string {
	if in.ProviderInvocationID != "" {
		return in.ProviderInvocationID
	}
	turn := in.TurnID
	if turn == "" {
		turn = "session"
	}
	return fmt.Sprintf("pvi/%s/%s/%s/%s", in.SessionID, turn, in.EventID, in.Modality)
}

func (c Controller) appendSignal(result *InvocationResult, in InvocationInput, signalName string, reason string) error {
	offset := int64(len(result.Signals))
	eventScope := eventabi.ScopeSession
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
	}
	transportSequence := nonNegative(in.TransportSequence) + offset
	signal := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		EventID:            fmt.Sprintf("%s-%s-%d", in.EventID, signalName, offset+1),
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transportSequence,
		RuntimeSequence:    nonNegative(in.RuntimeSequence) + offset,
		AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS) + offset,
		WallClockMS:        nonNegative(in.WallClockTimestampMS) + offset,
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             signalName,
		EmittedBy:          "RK-11",
		Reason:             reason,
		Scope:              "provider_invocation",
	}
	if err := signal.Validate(); err != nil {
		return err
	}
	result.Signals = append(result.Signals, signal)
	return nil
}

func normalizeFailureReason(providerID string, outcome contracts.Outcome) string {
	reason := outcome.Reason
	if reason == "" {
		reason = "provider_failure"
	}
	return fmt.Sprintf("provider=%s class=%s reason=%s", providerID, outcome.Class, reason)
}

type streamAttemptStats struct {
	chunkCount          int
	bytesOut            int64
	firstChunkLatencyMS int64
	hasFirstChunk       bool
}

type invocationStreamObserver struct {
	start time.Time
	stats *streamAttemptStats
	hooks StreamEventHooks
	mu    sync.Mutex
}

func newInvocationStreamObserver(start time.Time, stats *streamAttemptStats, hooks StreamEventHooks) *invocationStreamObserver {
	if stats == nil {
		stats = &streamAttemptStats{}
	}
	return &invocationStreamObserver{start: start, stats: stats, hooks: hooks}
}

func (o *invocationStreamObserver) OnStart(chunk contracts.StreamChunk) error {
	if err := chunk.Validate(); err != nil {
		return err
	}
	if o.hooks.OnStart != nil {
		return o.hooks.OnStart(chunk)
	}
	return nil
}

func (o *invocationStreamObserver) OnChunk(chunk contracts.StreamChunk) error {
	if err := chunk.Validate(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stats.chunkCount++
	o.stats.bytesOut += int64(len(chunk.AudioBytes) + len(chunk.TextDelta) + len(chunk.TextFinal))
	if !o.stats.hasFirstChunk {
		o.stats.hasFirstChunk = true
		o.stats.firstChunkLatencyMS = max(0, time.Since(o.start).Milliseconds())
	}
	if o.hooks.OnChunk != nil {
		return o.hooks.OnChunk(chunk)
	}
	return nil
}

func (o *invocationStreamObserver) OnComplete(chunk contracts.StreamChunk) error {
	if err := chunk.Validate(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.stats.hasFirstChunk {
		o.stats.hasFirstChunk = true
		o.stats.firstChunkLatencyMS = max(0, time.Since(o.start).Milliseconds())
	}
	if o.hooks.OnComplete != nil {
		return o.hooks.OnComplete(chunk)
	}
	return nil
}

func (o *invocationStreamObserver) OnError(chunk contracts.StreamChunk) error {
	if err := chunk.Validate(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.stats.hasFirstChunk {
		o.stats.hasFirstChunk = true
		o.stats.firstChunkLatencyMS = max(0, time.Since(o.start).Milliseconds())
	}
	if o.hooks.OnError != nil {
		return o.hooks.OnError(chunk)
	}
	return nil
}

func streamingEnabled(in InvocationInput, cfg Config, modality contracts.Modality, adapter contracts.Adapter) bool {
	if _, ok := adapter.(contracts.StreamingAdapter); !ok {
		return false
	}
	if in.DisableProviderStreaming {
		return false
	}
	if envEnabled("RSPP_PROVIDER_STREAMING_DISABLE") {
		return false
	}
	switch modality {
	case contracts.ModalitySTT:
		if envEnabled("RSPP_PROVIDER_STREAMING_STT_DISABLE") {
			return false
		}
	case contracts.ModalityLLM:
		if envEnabled("RSPP_PROVIDER_STREAMING_LLM_DISABLE") {
			return false
		}
	case contracts.ModalityTTS:
		if envEnabled("RSPP_PROVIDER_STREAMING_TTS_DISABLE") {
			return false
		}
	}
	if envEnabled("RSPP_PROVIDER_STREAMING_ENABLE") {
		return true
	}
	if in.EnableStreaming || cfg.EnableStreaming {
		return true
	}
	// Default-on native streaming for adapters that support it.
	return true
}

func envEnabled(key string) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func correlationWithTimestamps(correlation telemetry.Correlation, runtimeTimestampMS int64, wallClockTimestampMS int64) telemetry.Correlation {
	out := correlation
	out.RuntimeTimestampMS = nonNegative(runtimeTimestampMS)
	out.WallClockTimestampMS = nonNegative(wallClockTimestampMS)
	return out
}

func max[T ~int | ~int64](a, b T) T {
	if a > b {
		return a
	}
	return b
}
