package invocation

import (
	"fmt"
	"strconv"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
)

// Config controls deterministic RK-11 invocation behavior.
type Config struct {
	MaxAttemptsPerProvider int
	MaxCandidateProviders  int
}

// Controller executes deterministic provider invocation attempts.
type Controller struct {
	catalog registry.Catalog
	cfg     Config
}

// InvocationInput carries scheduler-side context into RK-11.
type InvocationInput struct {
	SessionID              string
	TurnID                 string
	PipelineVersion        string
	EventID                string
	Modality               contracts.Modality
	PreferredProvider      string
	AllowedAdaptiveActions []string
	ProviderInvocationID   string
	TransportSequence      int64
	RuntimeSequence        int64
	AuthorityEpoch         int64
	RuntimeTimestampMS     int64
	WallClockTimestampMS   int64
	CancelRequested        bool
}

// InvocationAttempt records one provider attempt with normalized outcome.
type InvocationAttempt struct {
	ProviderID string
	Attempt    int
	Outcome    contracts.Outcome
}

// InvocationResult summarizes deterministic invocation behavior.
type InvocationResult struct {
	ProviderInvocationID string
	SelectedProvider     string
	Outcome              contracts.Outcome
	RetryDecision        string
	Attempts             []InvocationAttempt
	Signals              []eventabi.ControlSignal
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

	candidates, err := c.catalog.Candidates(in.Modality, in.PreferredProvider, c.cfg.MaxCandidateProviders)
	if err != nil {
		return InvocationResult{}, err
	}

	result := InvocationResult{
		ProviderInvocationID: providerInvocationID(in),
		RetryDecision:        "none",
		Attempts:             make([]InvocationAttempt, 0, c.cfg.MaxAttemptsPerProvider*len(candidates)),
		Signals:              make([]eventabi.ControlSignal, 0),
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
			},
			telemetry.Correlation{
				SessionID:          in.SessionID,
				TurnID:             in.TurnID,
				EventID:            in.EventID,
				PipelineVersion:    in.PipelineVersion,
				AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
				Lane:               string(eventabi.LaneTelemetry),
				EmittedBy:          "OR-01",
				RuntimeTimestampMS: nonNegative(in.RuntimeTimestampMS),
			},
		)
		return result, nil
	}

	actions, err := parseAdaptiveActions(in.AllowedAdaptiveActions)
	if err != nil {
		return InvocationResult{}, err
	}
	for providerIndex, adapter := range candidates {
		for attempt := 1; attempt <= c.cfg.MaxAttemptsPerProvider; attempt++ {
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
				RetryBudgetRemaining:   max(0, c.cfg.MaxAttemptsPerProvider-attempt),
				CandidateProviderCount: len(candidates),
			}
			outcome, invokeErr := adapter.Invoke(req)
			if invokeErr != nil {
				outcome = contracts.Outcome{
					Class:     contracts.OutcomeInfrastructureFailure,
					Retryable: true,
					Reason:    "adapter_invoke_error",
				}
			}
			if err := outcome.Validate(); err != nil {
				return InvocationResult{}, err
			}
			attemptStartMS := nonNegative(in.RuntimeTimestampMS) + int64(attempt-1)
			attemptLatencyMS := nonNegative(outcome.BackoffMS)
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
				},
				telemetry.Correlation{
					SessionID:          in.SessionID,
					TurnID:             in.TurnID,
					EventID:            in.EventID,
					PipelineVersion:    in.PipelineVersion,
					AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
					Lane:               string(eventabi.LaneTelemetry),
					EmittedBy:          "OR-01",
					RuntimeTimestampMS: attemptStartMS,
				},
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
				},
				telemetry.Correlation{
					SessionID:          in.SessionID,
					TurnID:             in.TurnID,
					EventID:            in.EventID,
					PipelineVersion:    in.PipelineVersion,
					AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
					Lane:               string(eventabi.LaneTelemetry),
					EmittedBy:          "OR-01",
					RuntimeTimestampMS: attemptStartMS,
				},
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
				},
				telemetry.Correlation{
					SessionID:          in.SessionID,
					TurnID:             in.TurnID,
					EventID:            in.EventID,
					PipelineVersion:    in.PipelineVersion,
					AuthorityEpoch:     nonNegative(in.AuthorityEpoch),
					Lane:               string(eventabi.LaneTelemetry),
					EmittedBy:          "OR-01",
					RuntimeTimestampMS: attemptEndMS,
				},
			)

			result.Attempts = append(result.Attempts, InvocationAttempt{
				ProviderID: adapter.ProviderID(),
				Attempt:    attempt,
				Outcome:    outcome,
			})
			result.SelectedProvider = adapter.ProviderID()
			result.Outcome = outcome

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

			if outcome.Retryable && actions.retry && attempt < c.cfg.MaxAttemptsPerProvider {
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

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
