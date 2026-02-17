package executor

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
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	runtimeflowcontrol "github.com/tiger/realtime-speech-pipeline/internal/runtime/flowcontrol"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
)

const (
	handoffEnvEnable              = "RSPP_ORCH_STREAM_HANDOFF_ENABLE"
	handoffEnvEnableSTTLLM        = "RSPP_ORCH_STREAM_HANDOFF_STT_LLM_ENABLE"
	handoffEnvEnableLLMTTS        = "RSPP_ORCH_STREAM_HANDOFF_LLM_TTS_ENABLE"
	handoffEnvMinPartialChars     = "RSPP_ORCH_STREAM_HANDOFF_MIN_PARTIAL_CHARS"
	handoffEnvMaxPendingRevisions = "RSPP_ORCH_STREAM_HANDOFF_MAX_PENDING_REVISIONS"
	handoffEnvCoalesceLatest      = "RSPP_ORCH_STREAM_HANDOFF_COALESCE_LATEST_ONLY"
)

// StreamingHandoffPolicy configures orchestration overlap behavior.
type StreamingHandoffPolicy struct {
	Enabled             bool
	STTToLLMEnabled     bool
	LLMToTTSEnabled     bool
	MinPartialChars     int
	MaxPendingRevisions int
	CoalesceLatestOnly  bool
}

// DefaultStreamingHandoffPolicy returns safe defaults.
func DefaultStreamingHandoffPolicy() StreamingHandoffPolicy {
	return StreamingHandoffPolicy{
		Enabled:             false,
		STTToLLMEnabled:     false,
		LLMToTTSEnabled:     false,
		MinPartialChars:     12,
		MaxPendingRevisions: 4,
		CoalesceLatestOnly:  true,
	}
}

// StreamingHandoffPolicyFromEnv resolves policy from environment variables.
func StreamingHandoffPolicyFromEnv() StreamingHandoffPolicy {
	policy := DefaultStreamingHandoffPolicy()
	policy.Enabled = envEnabled(handoffEnvEnable)
	policy.STTToLLMEnabled = resolveEdgeEnabled(policy.Enabled, handoffEnvEnableSTTLLM)
	policy.LLMToTTSEnabled = resolveEdgeEnabled(policy.Enabled, handoffEnvEnableLLMTTS)
	policy.MinPartialChars = envPositiveInt(handoffEnvMinPartialChars, policy.MinPartialChars)
	policy.MaxPendingRevisions = envPositiveInt(handoffEnvMaxPendingRevisions, policy.MaxPendingRevisions)
	if raw := strings.TrimSpace(os.Getenv(handoffEnvCoalesceLatest)); raw != "" {
		policy.CoalesceLatestOnly = envEnabled(handoffEnvCoalesceLatest)
	}
	return policy.normalize()
}

func (p StreamingHandoffPolicy) normalize() StreamingHandoffPolicy {
	if p.MinPartialChars < 1 {
		p.MinPartialChars = 1
	}
	if p.MaxPendingRevisions < 1 {
		p.MaxPendingRevisions = 1
	}
	if !p.Enabled {
		p.STTToLLMEnabled = false
		p.LLMToTTSEnabled = false
	}
	return p
}

// StreamingHandoffEdgeResult captures one cross-stage handoff timing sample.
type StreamingHandoffEdgeResult struct {
	HandoffID             string `json:"handoff_id"`
	Edge                  string `json:"edge"`
	UpstreamRevision      int    `json:"upstream_revision"`
	Action                string `json:"action"`
	PartialAcceptedAtMS   int64  `json:"partial_accepted_at_ms"`
	DownstreamStartedAtMS int64  `json:"downstream_started_at_ms"`
	HandoffLatencyMS      int64  `json:"handoff_latency_ms"`
	QueueDepth            int    `json:"queue_depth"`
	WatermarkHigh         bool   `json:"watermark_high"`
}

// StreamingStageResult summarizes one stage in the streaming chain.
type StreamingStageResult struct {
	Stage                string            `json:"stage"`
	EventID              string            `json:"event_id"`
	ProviderInvocationID string            `json:"provider_invocation_id,omitempty"`
	InvocationCount      int               `json:"invocation_count"`
	StartedAtMS          int64             `json:"started_at_ms,omitempty"`
	FirstChunkAtMS       int64             `json:"first_chunk_at_ms,omitempty"`
	CompletedAtMS        int64             `json:"completed_at_ms,omitempty"`
	Decision             *ProviderDecision `json:"decision,omitempty"`
	Error                string            `json:"error,omitempty"`
}

// StreamingChainLatency captures live overlap timing metrics.
type StreamingChainLatency struct {
	STTFirstPartialLatencyMS      int64 `json:"stt_first_partial_latency_ms,omitempty"`
	STTPartialToLLMStartLatencyMS int64 `json:"stt_partial_to_llm_start_latency_ms,omitempty"`
	LLMFirstPartialLatencyMS      int64 `json:"llm_first_partial_latency_ms,omitempty"`
	LLMPartialToTTSStartLatencyMS int64 `json:"llm_partial_to_tts_start_latency_ms,omitempty"`
	TTSFirstAudioLatencyMS        int64 `json:"tts_first_audio_latency_ms,omitempty"`
	FirstAssistantAudioE2EMS      int64 `json:"first_assistant_audio_e2e_latency_ms,omitempty"`
	TurnCompletionE2EMS           int64 `json:"turn_completion_e2e_latency_ms,omitempty"`
}

// StreamingChainResult reports orchestration-level chain execution.
type StreamingChainResult struct {
	TurnStartedAtMS int64                        `json:"turn_started_at_ms"`
	Status          string                       `json:"status"`
	STT             StreamingStageResult         `json:"stt"`
	LLM             StreamingStageResult         `json:"llm"`
	TTS             StreamingStageResult         `json:"tts"`
	Handoffs        []StreamingHandoffEdgeResult `json:"handoffs,omitempty"`
	CoalesceCount   int                          `json:"coalesce_count,omitempty"`
	SupersedeCount  int                          `json:"supersede_count,omitempty"`
	Signals         []eventabi.ControlSignal     `json:"signals,omitempty"`
	Latency         StreamingChainLatency        `json:"latency"`
}

type handoffTrigger struct {
	acceptedAtMS int64
	revision     int
	action       string
}

// ExecuteStreamingChain runs a feature-flagged STT->LLM->TTS orchestration overlap chain.
func (s Scheduler) ExecuteStreamingChain(
	in SchedulingInput,
	stt ProviderInvocationInput,
	llm ProviderInvocationInput,
	tts ProviderInvocationInput,
	policy StreamingHandoffPolicy,
) (StreamingChainResult, error) {
	policy = policy.normalize()
	turnStartedAtMS := time.Now().UnixMilli()
	if in.WallClockTimestampMS > 0 {
		turnStartedAtMS = in.WallClockTimestampMS
	}

	result := StreamingChainResult{
		TurnStartedAtMS: turnStartedAtMS,
		Status:          "pending",
		STT:             StreamingStageResult{Stage: "stt"},
		LLM:             StreamingStageResult{Stage: "llm"},
		TTS:             StreamingStageResult{Stage: "tts"},
		Handoffs:        make([]StreamingHandoffEdgeResult, 0, 8),
		Signals:         make([]eventabi.ControlSignal, 0, 8),
	}

	// Fallback behavior keeps current deterministic stage-by-stage execution.
	if !policy.Enabled {
		return s.executeSequentialChain(in, stt, llm, tts, result)
	}

	var (
		mu               sync.Mutex
		llmForwardSent   bool
		sttSupersedeSent bool
		sttRevision      int
		llmRevision      int
		sttText          strings.Builder
		llmText          strings.Builder
		sttDecision      *ProviderDecision
		llmDecision      *ProviderDecision
		ttsDecision      *ProviderDecision
		firstErr         error
	)

	baseEventID := in.EventID
	if baseEventID == "" {
		baseEventID = "evt-streaming-chain"
	}
	llmTriggers := make(chan handoffTrigger, policy.MaxPendingRevisions)
	ttsTriggers := make(chan handoffTrigger, policy.MaxPendingRevisions)
	llmDone := make(chan struct{})
	ttsDone := make(chan struct{})
	edgeSaturated := map[string]bool{
		"stt_to_llm": false,
		"llm_to_tts": false,
	}

	recordHandoff := func(edge string, trigger handoffTrigger, downstreamStartedAtMS int64, queueDepth int, watermarkHigh bool) {
		handoff := StreamingHandoffEdgeResult{
			HandoffID:             fmt.Sprintf("%s-%s-r%d", baseEventID, edge, trigger.revision),
			Edge:                  edge,
			UpstreamRevision:      trigger.revision,
			Action:                trigger.action,
			PartialAcceptedAtMS:   trigger.acceptedAtMS,
			DownstreamStartedAtMS: downstreamStartedAtMS,
			HandoffLatencyMS:      maxInt64(0, downstreamStartedAtMS-trigger.acceptedAtMS),
			QueueDepth:            queueDepth,
			WatermarkHigh:         watermarkHigh,
		}
		result.Handoffs = append(result.Handoffs, handoff)
	}

	enqueueTrigger := func(edge string, ch chan handoffTrigger, trigger handoffTrigger) error {
		queueSaturated := false
		select {
		case ch <- trigger:
			queueSaturated = false
		default:
			queueSaturated = true
			if !policy.CoalesceLatestOnly {
				break
			}
			select {
			case <-ch:
				result.CoalesceCount++
			default:
			}
			select {
			case ch <- trigger:
			default:
			}
		}
		signals, nextSaturated, err := handoffBackpressureSignals(in, baseEventID, edge, queueSaturated, edgeSaturated[edge], len(result.Signals))
		if err != nil {
			return err
		}
		edgeSaturated[edge] = nextSaturated
		result.Signals = append(result.Signals, signals...)
		return nil
	}

	runLLM := func() {
		defer close(llmDone)
		defer close(ttsTriggers)

		invocationIndex := 0
		for trigger := range llmTriggers {
			if firstErr != nil {
				return
			}
			invocationIndex++
			startedAtMS := time.Now().UnixMilli()
			if result.LLM.StartedAtMS == 0 {
				result.LLM.StartedAtMS = startedAtMS
			}

			trackFirstChunk := true
			llmText.Reset()
			llmRevision = 0
			ttsForwardSentLocal := false
			stageEventID := fmt.Sprintf("%s-llm-r%d", baseEventID, trigger.revision)
			result.LLM.EventID = stageEventID
			providerInvocationID := llm.ProviderInvocationID
			if providerInvocationID == "" {
				providerInvocationID = fmt.Sprintf("pvi/%s/%s/llm/r%d", in.SessionID, in.TurnID, trigger.revision)
			} else if invocationIndex > 1 {
				providerInvocationID = fmt.Sprintf("%s-r%d", llm.ProviderInvocationID, trigger.revision)
			}

			mu.Lock()
			recordHandoff("stt_to_llm", trigger, startedAtMS, len(llmTriggers), len(llmTriggers) >= policy.MaxPendingRevisions)
			mu.Unlock()

			llmHooks := invocation.StreamEventHooks{
				OnChunk: func(chunk contracts.StreamChunk) error {
					now := time.Now().UnixMilli()
					mu.Lock()
					defer mu.Unlock()

					if trackFirstChunk && result.LLM.FirstChunkAtMS == 0 {
						result.LLM.FirstChunkAtMS = now
						trackFirstChunk = false
					}
					text := strings.TrimSpace(chunk.TextDelta)
					if text == "" {
						text = strings.TrimSpace(chunk.TextFinal)
					}
					if text != "" {
						llmRevision++
						llmText.WriteString(text)
					}
					if policy.LLMToTTSEnabled && !ttsForwardSentLocal && shouldTriggerPartial(llmText.String(), policy.MinPartialChars) {
						if err := enqueueTrigger("llm_to_tts", ttsTriggers, handoffTrigger{
							acceptedAtMS: now,
							revision:     maxInt(1, llmRevision),
							action:       "forward",
						}); err != nil {
							return err
						}
						ttsForwardSentLocal = true
					}
					return nil
				},
				OnComplete: func(chunk contracts.StreamChunk) error {
					now := time.Now().UnixMilli()
					mu.Lock()
					defer mu.Unlock()
					result.LLM.CompletedAtMS = now
					if policy.LLMToTTSEnabled && !ttsForwardSentLocal {
						if err := enqueueTrigger("llm_to_tts", ttsTriggers, handoffTrigger{
							acceptedAtMS: now,
							revision:     maxInt(1, llmRevision+1),
							action:       "final_fallback",
						}); err != nil {
							return err
						}
						ttsForwardSentLocal = true
					}
					return nil
				},
			}

			decision, err := s.NodeDispatch(withStageSchedulingInput(in, "llm", 2, stageEventID, ProviderInvocationInput{
				Modality:               llm.Modality,
				PreferredProvider:      llm.PreferredProvider,
				AllowedAdaptiveActions: append([]string(nil), llm.AllowedAdaptiveActions...),
				ProviderInvocationID:   providerInvocationID,
				CancelRequested:        llm.CancelRequested,
				EnableStreaming:        true,
				StreamHooks:            llmHooks,
			}))
			if err != nil {
				mu.Lock()
				result.LLM.Error = err.Error()
				firstErr = err
				mu.Unlock()
				return
			}
			mu.Lock()
			result.LLM.InvocationCount++
			if decision.Provider != nil {
				result.LLM.ProviderInvocationID = decision.Provider.ProviderInvocationID
				llmDecision = cloneProviderDecision(decision.Provider)
				result.Signals = append(result.Signals, decision.Provider.Signals...)
			}
			mu.Unlock()
			if decision.Provider == nil {
				mu.Lock()
				firstErr = fmt.Errorf("llm provider decision missing")
				result.LLM.Error = firstErr.Error()
				mu.Unlock()
				return
			}
			if decision.Provider.OutcomeClass != contracts.OutcomeSuccess {
				mu.Lock()
				firstErr = fmt.Errorf("llm invocation failed outcome_class=%s reason=%s", decision.Provider.OutcomeClass, outcomeReason(decision.Provider))
				result.LLM.Error = firstErr.Error()
				mu.Unlock()
				return
			}
		}
	}

	runTTS := func() {
		defer close(ttsDone)

		invocationIndex := 0
		for trigger := range ttsTriggers {
			if firstErr != nil {
				return
			}
			invocationIndex++
			startedAtMS := time.Now().UnixMilli()
			if result.TTS.StartedAtMS == 0 {
				result.TTS.StartedAtMS = startedAtMS
			}
			stageEventID := fmt.Sprintf("%s-tts-r%d", baseEventID, trigger.revision)
			result.TTS.EventID = stageEventID
			providerInvocationID := tts.ProviderInvocationID
			if providerInvocationID == "" {
				providerInvocationID = fmt.Sprintf("pvi/%s/%s/tts/r%d", in.SessionID, in.TurnID, trigger.revision)
			} else if invocationIndex > 1 {
				providerInvocationID = fmt.Sprintf("%s-r%d", tts.ProviderInvocationID, trigger.revision)
			}

			mu.Lock()
			recordHandoff("llm_to_tts", trigger, startedAtMS, len(ttsTriggers), len(ttsTriggers) >= policy.MaxPendingRevisions)
			mu.Unlock()

			ttsHooks := invocation.StreamEventHooks{
				OnChunk: func(chunk contracts.StreamChunk) error {
					now := time.Now().UnixMilli()
					mu.Lock()
					defer mu.Unlock()
					if len(chunk.AudioBytes) > 0 && result.TTS.FirstChunkAtMS == 0 {
						result.TTS.FirstChunkAtMS = now
					}
					return nil
				},
				OnComplete: func(chunk contracts.StreamChunk) error {
					now := time.Now().UnixMilli()
					mu.Lock()
					defer mu.Unlock()
					result.TTS.CompletedAtMS = now
					return nil
				},
			}

			decision, err := s.NodeDispatch(withStageSchedulingInput(in, "tts", 3, stageEventID, ProviderInvocationInput{
				Modality:               tts.Modality,
				PreferredProvider:      tts.PreferredProvider,
				AllowedAdaptiveActions: append([]string(nil), tts.AllowedAdaptiveActions...),
				ProviderInvocationID:   providerInvocationID,
				CancelRequested:        tts.CancelRequested,
				EnableStreaming:        true,
				StreamHooks:            ttsHooks,
			}))
			if err != nil {
				mu.Lock()
				result.TTS.Error = err.Error()
				firstErr = err
				mu.Unlock()
				return
			}
			mu.Lock()
			result.TTS.InvocationCount++
			if decision.Provider != nil {
				result.TTS.ProviderInvocationID = decision.Provider.ProviderInvocationID
				ttsDecision = cloneProviderDecision(decision.Provider)
				result.Signals = append(result.Signals, decision.Provider.Signals...)
			}
			mu.Unlock()
			if decision.Provider == nil {
				mu.Lock()
				firstErr = fmt.Errorf("tts provider decision missing")
				result.TTS.Error = firstErr.Error()
				mu.Unlock()
				return
			}
			if decision.Provider.OutcomeClass != contracts.OutcomeSuccess {
				mu.Lock()
				firstErr = fmt.Errorf("tts invocation failed outcome_class=%s reason=%s", decision.Provider.OutcomeClass, outcomeReason(decision.Provider))
				result.TTS.Error = firstErr.Error()
				mu.Unlock()
				return
			}
		}
	}

	go runLLM()
	go runTTS()

	sttStartedAtMS := time.Now().UnixMilli()
	result.STT.StartedAtMS = sttStartedAtMS
	sttHooks := invocation.StreamEventHooks{
		OnChunk: func(chunk contracts.StreamChunk) error {
			now := time.Now().UnixMilli()
			mu.Lock()
			defer mu.Unlock()

			if result.STT.FirstChunkAtMS == 0 {
				result.STT.FirstChunkAtMS = now
			}
			text := strings.TrimSpace(chunk.TextDelta)
			if text == "" {
				text = strings.TrimSpace(chunk.TextFinal)
			}
			if text != "" {
				sttRevision++
				sttText.WriteString(text)
			}
			if policy.STTToLLMEnabled && !llmForwardSent && shouldTriggerPartial(sttText.String(), policy.MinPartialChars) {
				if err := enqueueTrigger("stt_to_llm", llmTriggers, handoffTrigger{
					acceptedAtMS: now,
					revision:     maxInt(1, sttRevision),
					action:       "forward",
				}); err != nil {
					return err
				}
				llmForwardSent = true
			}
			return nil
		},
		OnComplete: func(chunk contracts.StreamChunk) error {
			now := time.Now().UnixMilli()
			mu.Lock()
			defer mu.Unlock()
			result.STT.CompletedAtMS = now
			finalText := strings.TrimSpace(chunk.TextFinal)
			currentPartial := strings.TrimSpace(sttText.String())
			if policy.STTToLLMEnabled && !llmForwardSent && finalText != "" {
				if err := enqueueTrigger("stt_to_llm", llmTriggers, handoffTrigger{
					acceptedAtMS: now,
					revision:     maxInt(1, sttRevision+1),
					action:       "final_fallback",
				}); err != nil {
					return err
				}
				llmForwardSent = true
			}
			if policy.STTToLLMEnabled && llmForwardSent && !sttSupersedeSent && finalText != "" && finalText != currentPartial {
				sttRevision++
				if err := enqueueTrigger("stt_to_llm", llmTriggers, handoffTrigger{
					acceptedAtMS: now,
					revision:     maxInt(1, sttRevision),
					action:       "supersede",
				}); err != nil {
					return err
				}
				sttSupersedeSent = true
				result.SupersedeCount++
			}
			return nil
		},
	}

	sttEventID := fmt.Sprintf("%s-stt", baseEventID)
	result.STT.EventID = sttEventID
	sttDecisionResult, sttErr := s.NodeDispatch(withStageSchedulingInput(in, "stt", 1, sttEventID, ProviderInvocationInput{
		Modality:               stt.Modality,
		PreferredProvider:      stt.PreferredProvider,
		AllowedAdaptiveActions: append([]string(nil), stt.AllowedAdaptiveActions...),
		ProviderInvocationID:   stt.ProviderInvocationID,
		CancelRequested:        stt.CancelRequested,
		EnableStreaming:        true,
		StreamHooks:            sttHooks,
	}))
	close(llmTriggers)
	<-llmDone
	<-ttsDone

	if sttErr != nil {
		result.STT.Error = sttErr.Error()
		firstErr = sttErr
	} else {
		result.STT.InvocationCount = 1
		if sttDecisionResult.Provider != nil {
			result.STT.ProviderInvocationID = sttDecisionResult.Provider.ProviderInvocationID
			sttDecision = cloneProviderDecision(sttDecisionResult.Provider)
			result.Signals = append(result.Signals, sttDecisionResult.Provider.Signals...)
		}
	}

	result.STT.Decision = sttDecision
	result.LLM.Decision = llmDecision
	result.TTS.Decision = ttsDecision

	populateStreamingChainLatency(&result)
	appendHandoffEvidenceIfPossible(s.handoffAppender, in, result.Handoffs)

	if firstErr != nil {
		result.Status = "fail"
		return result, firstErr
	}
	if sttDecision == nil || sttDecision.OutcomeClass != contracts.OutcomeSuccess {
		result.Status = "fail"
		return result, fmt.Errorf("stt stage did not complete with success")
	}
	if policy.STTToLLMEnabled && (llmDecision == nil || llmDecision.OutcomeClass != contracts.OutcomeSuccess) {
		result.Status = "fail"
		return result, fmt.Errorf("llm stage did not complete with success")
	}
	if policy.LLMToTTSEnabled && (ttsDecision == nil || ttsDecision.OutcomeClass != contracts.OutcomeSuccess) {
		result.Status = "fail"
		return result, fmt.Errorf("tts stage did not complete with success")
	}

	result.Status = "pass"
	return result, nil
}

func (s Scheduler) executeSequentialChain(
	in SchedulingInput,
	stt ProviderInvocationInput,
	llm ProviderInvocationInput,
	tts ProviderInvocationInput,
	result StreamingChainResult,
) (StreamingChainResult, error) {
	baseEventID := in.EventID
	if baseEventID == "" {
		baseEventID = "evt-streaming-chain"
	}

	run := func(stage string, order int, provider ProviderInvocationInput) (*ProviderDecision, int64, int64, error) {
		startedAtMS := time.Now().UnixMilli()
		decision, err := s.NodeDispatch(withStageSchedulingInput(in, stage, order, fmt.Sprintf("%s-%s", baseEventID, stage), provider))
		completedAtMS := time.Now().UnixMilli()
		if err != nil {
			return nil, startedAtMS, completedAtMS, err
		}
		if decision.Provider == nil {
			return nil, startedAtMS, completedAtMS, fmt.Errorf("%s provider decision missing", stage)
		}
		return cloneProviderDecision(decision.Provider), startedAtMS, completedAtMS, nil
	}

	sttDecision, sttStartedAt, sttCompletedAt, err := run("stt", 1, stt)
	result.STT.StartedAtMS = sttStartedAt
	result.STT.CompletedAtMS = sttCompletedAt
	result.STT.InvocationCount = 1
	result.STT.EventID = fmt.Sprintf("%s-stt", baseEventID)
	result.STT.Decision = sttDecision
	if err != nil {
		result.STT.Error = err.Error()
		result.Status = "fail"
		return result, err
	}
	result.STT.ProviderInvocationID = sttDecision.ProviderInvocationID
	result.Signals = append(result.Signals, sttDecision.Signals...)
	if sttDecision.OutcomeClass != contracts.OutcomeSuccess {
		result.Status = "fail"
		return result, fmt.Errorf("stt stage failed outcome_class=%s", sttDecision.OutcomeClass)
	}

	llmDecision, llmStartedAt, llmCompletedAt, err := run("llm", 2, llm)
	result.LLM.StartedAtMS = llmStartedAt
	result.LLM.CompletedAtMS = llmCompletedAt
	result.LLM.InvocationCount = 1
	result.LLM.EventID = fmt.Sprintf("%s-llm", baseEventID)
	result.LLM.Decision = llmDecision
	if err != nil {
		result.LLM.Error = err.Error()
		result.Status = "fail"
		return result, err
	}
	result.LLM.ProviderInvocationID = llmDecision.ProviderInvocationID
	result.Signals = append(result.Signals, llmDecision.Signals...)
	if llmDecision.OutcomeClass != contracts.OutcomeSuccess {
		result.Status = "fail"
		return result, fmt.Errorf("llm stage failed outcome_class=%s", llmDecision.OutcomeClass)
	}

	ttsDecision, ttsStartedAt, ttsCompletedAt, err := run("tts", 3, tts)
	result.TTS.StartedAtMS = ttsStartedAt
	result.TTS.CompletedAtMS = ttsCompletedAt
	result.TTS.InvocationCount = 1
	result.TTS.EventID = fmt.Sprintf("%s-tts", baseEventID)
	result.TTS.Decision = ttsDecision
	if err != nil {
		result.TTS.Error = err.Error()
		result.Status = "fail"
		return result, err
	}
	result.TTS.ProviderInvocationID = ttsDecision.ProviderInvocationID
	result.Signals = append(result.Signals, ttsDecision.Signals...)
	if ttsDecision.OutcomeClass != contracts.OutcomeSuccess {
		result.Status = "fail"
		return result, fmt.Errorf("tts stage failed outcome_class=%s", ttsDecision.OutcomeClass)
	}

	populateStreamingChainLatency(&result)
	result.Status = "pass"
	return result, nil
}

func withStageSchedulingInput(base SchedulingInput, stage string, order int, eventID string, provider ProviderInvocationInput) SchedulingInput {
	offset := int64(order - 1)
	out := base
	out.EventID = eventID
	out.TransportSequence = nonNegative(base.TransportSequence) + offset
	out.RuntimeSequence = nonNegative(base.RuntimeSequence) + offset
	out.RuntimeTimestampMS = nonNegative(base.RuntimeTimestampMS) + offset
	out.WallClockTimestampMS = nonNegative(base.WallClockTimestampMS) + offset
	out.ProviderInvocation = &provider
	return out
}

func shouldTriggerPartial(text string, minChars int) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if len(trimmed) >= minChars {
		return true
	}
	return strings.HasSuffix(trimmed, ".") ||
		strings.HasSuffix(trimmed, "!") ||
		strings.HasSuffix(trimmed, "?")
}

func appendHandoffEvidenceIfPossible(appender HandoffEdgeAppender, in SchedulingInput, handoffs []StreamingHandoffEdgeResult) {
	if len(handoffs) == 0 {
		return
	}
	eventID := in.EventID
	if eventID == "" {
		eventID = "evt-streaming-chain"
	}
	pipelineVersion := defaultPipelineVersion(in.PipelineVersion)
	runtimeTS := nonNegative(in.RuntimeTimestampMS)
	wallClockTS := nonNegative(in.WallClockTimestampMS)

	entries := make([]timeline.HandoffEdgeEvidence, 0, len(handoffs))
	for _, handoff := range handoffs {
		correlation, err := telemetrycontext.Resolve(telemetrycontext.ResolveInput{
			SessionID:            in.SessionID,
			TurnID:               in.TurnID,
			EventID:              eventID,
			PipelineVersion:      pipelineVersion,
			NodeID:               "streaming_handoff",
			EdgeID:               handoff.Edge,
			AuthorityEpoch:       nonNegative(in.AuthorityEpoch),
			Lane:                 eventabi.LaneTelemetry,
			EmittedBy:            "OR-01",
			RuntimeTimestampMS:   nonNegative(handoff.DownstreamStartedAtMS),
			WallClockTimestampMS: wallClockTS,
		})
		if err == nil {
			telemetry.DefaultEmitter().EmitMetric(
				telemetry.MetricEdgeLatencyMS,
				float64(nonNegative(handoff.HandoffLatencyMS)),
				"ms",
				map[string]string{
					"edge_id":        handoff.Edge,
					"action":         handoff.Action,
					"watermark_high": strconv.FormatBool(handoff.WatermarkHigh),
				},
				correlation,
			)
		}
		if appender == nil {
			continue
		}
		entries = append(entries, timeline.HandoffEdgeEvidence{
			SessionID:             in.SessionID,
			TurnID:                in.TurnID,
			PipelineVersion:       pipelineVersion,
			EventID:               eventID,
			HandoffID:             handoff.HandoffID,
			Edge:                  handoff.Edge,
			UpstreamRevision:      handoff.UpstreamRevision,
			Action:                handoff.Action,
			PartialAcceptedAtMS:   handoff.PartialAcceptedAtMS,
			DownstreamStartedAtMS: handoff.DownstreamStartedAtMS,
			HandoffLatencyMS:      handoff.HandoffLatencyMS,
			QueueDepth:            handoff.QueueDepth,
			WatermarkHigh:         handoff.WatermarkHigh,
			RuntimeTimestampMS:    runtimeTS,
			WallClockTimestampMS:  wallClockTS,
		})
	}
	if appender == nil || len(entries) == 0 {
		return
	}
	_ = appender.AppendHandoffEdges(entries)
}

func populateStreamingChainLatency(result *StreamingChainResult) {
	if result == nil {
		return
	}
	if result.STT.FirstChunkAtMS > 0 && result.TurnStartedAtMS > 0 {
		result.Latency.STTFirstPartialLatencyMS = maxInt64(0, result.STT.FirstChunkAtMS-result.TurnStartedAtMS)
	}
	if result.LLM.StartedAtMS > 0 && result.LLM.FirstChunkAtMS > 0 {
		result.Latency.LLMFirstPartialLatencyMS = maxInt64(0, result.LLM.FirstChunkAtMS-result.LLM.StartedAtMS)
	}
	if result.TTS.StartedAtMS > 0 && result.TTS.FirstChunkAtMS > 0 {
		result.Latency.TTSFirstAudioLatencyMS = maxInt64(0, result.TTS.FirstChunkAtMS-result.TTS.StartedAtMS)
	}
	if result.TurnStartedAtMS > 0 && result.TTS.FirstChunkAtMS > 0 {
		result.Latency.FirstAssistantAudioE2EMS = maxInt64(0, result.TTS.FirstChunkAtMS-result.TurnStartedAtMS)
	}
	if result.TurnStartedAtMS > 0 && result.TTS.CompletedAtMS > 0 {
		result.Latency.TurnCompletionE2EMS = maxInt64(0, result.TTS.CompletedAtMS-result.TurnStartedAtMS)
	}

	for _, handoff := range result.Handoffs {
		switch handoff.Edge {
		case "stt_to_llm":
			if handoff.HandoffLatencyMS > 0 && (result.Latency.STTPartialToLLMStartLatencyMS == 0 || handoff.HandoffLatencyMS < result.Latency.STTPartialToLLMStartLatencyMS) {
				result.Latency.STTPartialToLLMStartLatencyMS = handoff.HandoffLatencyMS
			}
		case "llm_to_tts":
			if handoff.HandoffLatencyMS > 0 && (result.Latency.LLMPartialToTTSStartLatencyMS == 0 || handoff.HandoffLatencyMS < result.Latency.LLMPartialToTTSStartLatencyMS) {
				result.Latency.LLMPartialToTTSStartLatencyMS = handoff.HandoffLatencyMS
			}
		}
	}
}

func cloneProviderDecision(in *ProviderDecision) *ProviderDecision {
	if in == nil {
		return nil
	}
	out := *in
	if len(in.Signals) > 0 {
		out.Signals = append([]eventabi.ControlSignal(nil), in.Signals...)
	}
	return &out
}

func outcomeReason(decision *ProviderDecision) string {
	if decision == nil {
		return ""
	}
	for i := range decision.Signals {
		if decision.Signals[i].Signal == "provider_error" && decision.Signals[i].Reason != "" {
			return decision.Signals[i].Reason
		}
	}
	return ""
}

func resolveEdgeEnabled(globalEnabled bool, edgeEnvKey string) bool {
	raw := strings.TrimSpace(os.Getenv(edgeEnvKey))
	if raw == "" {
		return globalEnabled
	}
	return envEnabled(edgeEnvKey)
}

func envPositiveInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
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

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func handoffBackpressureSignals(
	in SchedulingInput,
	baseEventID string,
	edge string,
	queueSaturated bool,
	edgeSaturated bool,
	signalOffset int,
) ([]eventabi.ControlSignal, bool, error) {
	if edge != "stt_to_llm" && edge != "llm_to_tts" {
		return nil, edgeSaturated, fmt.Errorf("invalid handoff edge: %s", edge)
	}
	controller := runtimeflowcontrol.NewController()
	offset := int64(signalOffset)
	common := runtimeflowcontrol.Input{
		SessionID:            in.SessionID,
		TurnID:               in.TurnID,
		PipelineVersion:      defaultPipelineVersion(in.PipelineVersion),
		EdgeID:               edge,
		TargetLane:           eventabi.LaneData,
		TransportSequence:    nonNegative(in.TransportSequence) + offset,
		RuntimeSequence:      nonNegative(in.RuntimeSequence) + offset,
		AuthorityEpoch:       nonNegative(in.AuthorityEpoch),
		RuntimeTimestampMS:   nonNegative(in.RuntimeTimestampMS) + offset,
		WallClockTimestampMS: nonNegative(in.WallClockTimestampMS) + offset,
		Mode:                 runtimeflowcontrol.ModeSignal,
	}
	if queueSaturated {
		if edgeSaturated {
			return nil, true, nil
		}
		common.EventID = handoffFlowEventID(baseEventID, edge, "xoff", signalOffset)
		common.HighWatermark = true
		signals, err := controller.Evaluate(common)
		if err != nil {
			return nil, edgeSaturated, err
		}
		return signals, true, nil
	}
	if !edgeSaturated {
		return nil, false, nil
	}
	common.EventID = handoffFlowEventID(baseEventID, edge, "xon", signalOffset)
	common.EmitRecovery = true
	signals, err := controller.Evaluate(common)
	if err != nil {
		return nil, edgeSaturated, err
	}
	return signals, false, nil
}

func handoffFlowEventID(baseEventID string, edge string, phase string, signalOffset int) string {
	if baseEventID == "" {
		baseEventID = "evt-streaming-chain"
	}
	return fmt.Sprintf("%s-%s-flow-%s-%d", baseEventID, edge, phase, signalOffset+1)
}
