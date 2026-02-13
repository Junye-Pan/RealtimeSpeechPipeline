//go:build liveproviders

package integration_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/executor"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/bootstrap"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
	llmanthropicprovider "github.com/tiger/realtime-speech-pipeline/providers/llm/anthropic"
	llmcohereprovider "github.com/tiger/realtime-speech-pipeline/providers/llm/cohere"
	llmgeminiprovider "github.com/tiger/realtime-speech-pipeline/providers/llm/gemini"
	sttassemblyaiprovider "github.com/tiger/realtime-speech-pipeline/providers/stt/assemblyai"
	sttdeepgramprovider "github.com/tiger/realtime-speech-pipeline/providers/stt/deepgram"
	sttgoogleprovider "github.com/tiger/realtime-speech-pipeline/providers/stt/google"
	ttselevenlabsprovider "github.com/tiger/realtime-speech-pipeline/providers/tts/elevenlabs"
	ttsgoogleprovider "github.com/tiger/realtime-speech-pipeline/providers/tts/google"
	ttspollyprovider "github.com/tiger/realtime-speech-pipeline/providers/tts/polly"
)

const (
	defaultLiveProviderChainMaxCombos    = 12
	defaultLiveProviderChainReportPath   = ".codex/providers/live-provider-chain-report.json"
	defaultLiveProviderChainReportMDPath = ".codex/providers/live-provider-chain-report.md"

	defaultProviderIOCaptureMode     = "redacted"
	defaultProviderIOCaptureMaxBytes = 8192
	minProviderIOCaptureMaxBytes     = 256
)

type liveProviderCase struct {
	providerID string
	modality   contracts.Modality
	enableEnv  string
	required   []string
}

type chainEnabledProviders struct {
	STT []string `json:"stt"`
	LLM []string `json:"llm"`
	TTS []string `json:"tts"`
}

type liveProviderChainSelectedCombination struct {
	ComboIndex    int    `json:"combo_index"`
	STTProviderID string `json:"stt_provider_id"`
	LLMProviderID string `json:"llm_provider_id"`
	TTSProviderID string `json:"tts_provider_id"`
}

type liveProviderChainAggregate struct {
	ComboPassCount   int `json:"combo_pass_count"`
	ComboFailCount   int `json:"combo_fail_count"`
	ComboSkipCount   int `json:"combo_skip_count"`
	StepSuccessCount int `json:"step_success_count"`
	StepFailureCount int `json:"step_failure_count"`
}

type liveProviderChainStepAttempt struct {
	ProviderID       string `json:"provider_id"`
	Attempt          int    `json:"attempt"`
	OutcomeClass     string `json:"outcome_class"`
	Reason           string `json:"reason,omitempty"`
	Retryable        bool   `json:"retryable"`
	CircuitOpen      bool   `json:"circuit_open"`
	BackoffMS        int64  `json:"backoff_ms"`
	AttemptLatencyMS int64  `json:"attempt_latency_ms,omitempty"`
	FirstChunkMS     int64  `json:"first_chunk_latency_ms,omitempty"`
	ChunkCount       int    `json:"chunk_count,omitempty"`
	BytesOut         int64  `json:"bytes_out,omitempty"`
	StreamingUsed    bool   `json:"streaming_used,omitempty"`
	InputPayload     string `json:"input_payload,omitempty"`
	OutputPayload    string `json:"output_payload,omitempty"`
	OutputStatusCode int    `json:"output_status_code,omitempty"`
	PayloadTruncated bool   `json:"payload_truncated,omitempty"`
}

type liveProviderChainStepSignal struct {
	Signal    string `json:"signal"`
	Reason    string `json:"reason,omitempty"`
	EmittedBy string `json:"emitted_by,omitempty"`
}

type liveProviderChainStepInput struct {
	SessionID          string         `json:"session_id"`
	TurnID             string         `json:"turn_id"`
	PipelineVersion    string         `json:"pipeline_version"`
	EventID            string         `json:"event_id"`
	TransportSequence  int64          `json:"transport_sequence"`
	RuntimeSequence    int64          `json:"runtime_sequence"`
	AuthorityEpoch     int64          `json:"authority_epoch"`
	RuntimeTimestampMS int64          `json:"runtime_timestamp_ms"`
	WallClockMS        int64          `json:"wall_clock_timestamp_ms"`
	InputSnapshot      map[string]any `json:"input_snapshot"`
}

type liveProviderChainStepOutput struct {
	SelectedProviderID  string                         `json:"selected_provider_id"`
	OutcomeClass        string                         `json:"outcome_class"`
	OutcomeReason       string                         `json:"outcome_reason,omitempty"`
	Retryable           bool                           `json:"retryable"`
	CircuitOpen         bool                           `json:"circuit_open"`
	BackoffMS           int64                          `json:"backoff_ms"`
	RetryDecision       string                         `json:"retry_decision"`
	AttemptCount        int                            `json:"attempt_count"`
	Attempts            []liveProviderChainStepAttempt `json:"attempts"`
	Signals             []liveProviderChainStepSignal  `json:"signals,omitempty"`
	RawInputPayload     string                         `json:"raw_input_payload,omitempty"`
	RawOutputPayload    string                         `json:"raw_output_payload,omitempty"`
	RawOutputStatusCode int                            `json:"raw_output_status_code,omitempty"`
	RawPayloadTruncated bool                           `json:"raw_payload_truncated,omitempty"`
}

type liveProviderChainStepReport struct {
	Step                string                      `json:"step"`
	Modality            string                      `json:"modality"`
	PreferredProviderID string                      `json:"preferred_provider_id"`
	Input               liveProviderChainStepInput  `json:"input"`
	Output              liveProviderChainStepOutput `json:"output"`
	Error               string                      `json:"error,omitempty"`
}

type liveProviderChainCombinationReport struct {
	ComboIndex     int                           `json:"combo_index"`
	ComboID        string                        `json:"combo_id"`
	STTProviderID  string                        `json:"stt_provider_id"`
	LLMProviderID  string                        `json:"llm_provider_id"`
	TTSProviderID  string                        `json:"tts_provider_id"`
	Status         string                        `json:"status"`
	FailureStep    string                        `json:"failure_step,omitempty"`
	FailureReason  string                        `json:"failure_reason,omitempty"`
	Latency        liveProviderChainLatency      `json:"latency,omitempty"`
	Handoffs       []liveProviderChainHandoff    `json:"handoffs,omitempty"`
	CoalesceCount  int                           `json:"coalesce_count,omitempty"`
	SupersedeCount int                           `json:"supersede_count,omitempty"`
	Steps          []liveProviderChainStepReport `json:"steps"`
}

type liveProviderChainHandoff struct {
	HandoffID             string `json:"handoff_id"`
	Edge                  string `json:"edge"`
	UpstreamRevision      int    `json:"upstream_revision"`
	Action                string `json:"action"`
	PartialAcceptedAtMS   int64  `json:"partial_accepted_at_ms"`
	DownstreamStartedAtMS int64  `json:"downstream_started_at_ms"`
	HandoffLatencyMS      int64  `json:"handoff_latency_ms"`
	QueueDepth            int    `json:"queue_depth,omitempty"`
	WatermarkHigh         bool   `json:"watermark_high,omitempty"`
}

type liveProviderChainLatency struct {
	STTFirstPartialLatencyMS      int64 `json:"stt_first_partial_latency_ms,omitempty"`
	STTPartialToLLMStartLatencyMS int64 `json:"stt_partial_to_llm_start_latency_ms,omitempty"`
	LLMFirstPartialLatencyMS      int64 `json:"llm_first_partial_latency_ms,omitempty"`
	LLMPartialToTTSStartLatencyMS int64 `json:"llm_partial_to_tts_start_latency_ms,omitempty"`
	TTSFirstAudioLatencyMS        int64 `json:"tts_first_audio_latency_ms,omitempty"`
	FirstAssistantAudioE2EMS      int64 `json:"first_assistant_audio_e2e_latency_ms,omitempty"`
	TurnCompletionE2EMS           int64 `json:"turn_completion_e2e_latency_ms,omitempty"`
}

type liveProviderChainReport struct {
	GeneratedAtUTC            string                                 `json:"generated_at_utc"`
	Status                    string                                 `json:"status"`
	ComboCap                  int                                    `json:"combo_cap"`
	ComboCapSource            string                                 `json:"combo_cap_source"`
	ComboCapParseWarning      string                                 `json:"combo_cap_parse_warning,omitempty"`
	ProviderIOCaptureMode     string                                 `json:"provider_io_capture_mode"`
	ProviderIOCaptureMaxBytes int                                    `json:"provider_io_capture_max_bytes"`
	ReportPath                string                                 `json:"report_path"`
	ReportMarkdownPath        string                                 `json:"report_markdown_path"`
	EnabledProviders          chainEnabledProviders                  `json:"enabled_providers"`
	MissingModalities         []string                               `json:"missing_modalities,omitempty"`
	SkipReason                string                                 `json:"skip_reason,omitempty"`
	TotalCombinationCount     int                                    `json:"total_combination_count"`
	ExecutedCombinationCount  int                                    `json:"executed_combination_count"`
	SelectedCombinations      []liveProviderChainSelectedCombination `json:"selected_combinations,omitempty"`
	Aggregate                 liveProviderChainAggregate             `json:"aggregate"`
	Combinations              []liveProviderChainCombinationReport   `json:"combinations,omitempty"`
}

type liveProviderChainCombination struct {
	STTProviderID string
	LLMProviderID string
	TTSProviderID string
}

var liveProviderCases = []liveProviderCase{
	{providerID: "stt-deepgram", modality: contracts.ModalitySTT, enableEnv: "RSPP_STT_DEEPGRAM_ENABLE", required: []string{"RSPP_STT_DEEPGRAM_API_KEY"}},
	{providerID: "stt-google", modality: contracts.ModalitySTT, enableEnv: "RSPP_STT_GOOGLE_ENABLE", required: []string{"RSPP_STT_GOOGLE_API_KEY"}},
	{providerID: "stt-assemblyai", modality: contracts.ModalitySTT, enableEnv: "RSPP_STT_ASSEMBLYAI_ENABLE", required: []string{"RSPP_STT_ASSEMBLYAI_API_KEY"}},
	{providerID: "llm-anthropic", modality: contracts.ModalityLLM, enableEnv: "RSPP_LLM_ANTHROPIC_ENABLE", required: []string{"RSPP_LLM_ANTHROPIC_API_KEY"}},
	{providerID: "llm-gemini", modality: contracts.ModalityLLM, enableEnv: "RSPP_LLM_GEMINI_ENABLE", required: []string{"RSPP_LLM_GEMINI_API_KEY"}},
	{providerID: "llm-cohere", modality: contracts.ModalityLLM, enableEnv: "RSPP_LLM_COHERE_ENABLE", required: []string{"RSPP_LLM_COHERE_API_KEY"}},
	{providerID: "tts-elevenlabs", modality: contracts.ModalityTTS, enableEnv: "RSPP_TTS_ELEVENLABS_ENABLE", required: []string{"RSPP_TTS_ELEVENLABS_API_KEY"}},
	{providerID: "tts-google", modality: contracts.ModalityTTS, enableEnv: "RSPP_TTS_GOOGLE_ENABLE", required: []string{"RSPP_TTS_GOOGLE_API_KEY"}},
	{providerID: "tts-amazon-polly", modality: contracts.ModalityTTS, enableEnv: "RSPP_TTS_POLLY_ENABLE", required: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}},
}

func TestLiveProviderSmoke(t *testing.T) {
	t.Parallel()

	if os.Getenv("RSPP_LIVE_PROVIDER_SMOKE") != "1" {
		t.Skip("live provider smoke disabled (set RSPP_LIVE_PROVIDER_SMOKE=1)")
	}

	runtimeProviders, err := bootstrap.BuildMVPProviders()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	for _, tc := range liveProviderCases {
		tc := tc
		t.Run(tc.providerID, func(t *testing.T) {
			t.Parallel()

			enabled, reason := providerEnabled(tc)
			if !enabled {
				t.Skip(reason)
			}

			now := time.Now().UnixMilli()
			result, invokeErr := runtimeProviders.Controller.Invoke(invocation.InvocationInput{
				SessionID:              "sess-live-provider",
				TurnID:                 "turn-live-provider",
				PipelineVersion:        "pipeline-v1",
				EventID:                fmt.Sprintf("evt-live-%s", tc.providerID),
				Modality:               tc.modality,
				PreferredProvider:      tc.providerID,
				AllowedAdaptiveActions: []string{"retry", "provider_switch", "fallback"},
				TransportSequence:      1,
				RuntimeSequence:        1,
				AuthorityEpoch:         1,
				RuntimeTimestampMS:     now,
				WallClockTimestampMS:   now,
			})
			if invokeErr != nil {
				t.Fatalf("provider %s invocation error: %v", tc.providerID, invokeErr)
			}
			if result.Outcome.Class != contracts.OutcomeSuccess {
				t.Fatalf("provider %s expected success, got class=%s reason=%s retry=%s signals=%+v", tc.providerID, result.Outcome.Class, result.Outcome.Reason, result.RetryDecision, result.Signals)
			}
		})
	}
}

func TestLiveProviderSmokeSwitchAndFallbackRouting(t *testing.T) {
	t.Parallel()

	if os.Getenv("RSPP_LIVE_PROVIDER_SMOKE") != "1" {
		t.Skip("live provider smoke disabled (set RSPP_LIVE_PROVIDER_SMOKE=1)")
	}

	runtimeProviders, err := bootstrap.BuildMVPProviders()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	var chosen *liveProviderCase
	for i := range liveProviderCases {
		tc := &liveProviderCases[i]
		enabled, _ := providerEnabled(*tc)
		if !enabled {
			continue
		}
		chosen = tc
		break
	}
	if chosen == nil {
		t.Skip("no live provider with enabled credentials available for switch/fallback routing test")
	}

	realAdapter, ok := runtimeProviders.Catalog.Adapter(chosen.modality, chosen.providerID)
	if !ok {
		t.Fatalf("expected provider adapter %s for modality %s to exist in catalog", chosen.providerID, chosen.modality)
	}

	failingProviderID := "synthetic-failover-" + chosen.providerID
	failingAdapter := contracts.StaticAdapter{
		ID:   failingProviderID,
		Mode: chosen.modality,
		InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
			return contracts.Outcome{
				Class:       contracts.OutcomeOverload,
				Retryable:   false,
				CircuitOpen: true,
				Reason:      "synthetic_overload_for_switch_path",
			}, nil
		},
	}

	catalog, err := registry.NewCatalog([]contracts.Adapter{failingAdapter, realAdapter})
	if err != nil {
		t.Fatalf("build synthetic live routing catalog: %v", err)
	}
	controller := invocation.NewControllerWithConfig(catalog, invocation.Config{
		MaxAttemptsPerProvider: 1,
		MaxCandidateProviders:  2,
	})

	runRouting := func(t *testing.T, action string, expectedDecision string) {
		t.Helper()

		now := time.Now().UnixMilli()
		result, invokeErr := controller.Invoke(invocation.InvocationInput{
			SessionID:              "sess-live-provider-routing",
			TurnID:                 "turn-live-provider-routing",
			PipelineVersion:        "pipeline-v1",
			EventID:                fmt.Sprintf("evt-live-routing-%s-%s", chosen.providerID, action),
			Modality:               chosen.modality,
			PreferredProvider:      failingProviderID,
			AllowedAdaptiveActions: []string{action},
			TransportSequence:      1,
			RuntimeSequence:        1,
			AuthorityEpoch:         1,
			RuntimeTimestampMS:     now,
			WallClockTimestampMS:   now,
		})
		if invokeErr != nil {
			t.Fatalf("unexpected routing invocation error: %v", invokeErr)
		}
		if result.Outcome.Class != contracts.OutcomeSuccess {
			t.Fatalf("expected routed invocation success, got class=%s reason=%s", result.Outcome.Class, result.Outcome.Reason)
		}
		if result.SelectedProvider != chosen.providerID {
			t.Fatalf("expected routed provider %s, got %s", chosen.providerID, result.SelectedProvider)
		}
		if result.RetryDecision != expectedDecision {
			t.Fatalf("expected retry decision %s, got %s", expectedDecision, result.RetryDecision)
		}
		if len(result.Attempts) < 2 {
			t.Fatalf("expected at least two attempts across failover routing, got %+v", result.Attempts)
		}
		hasSwitchSignal := false
		for _, signal := range result.Signals {
			if signal.Signal == "provider_switch" {
				hasSwitchSignal = true
				break
			}
		}
		if !hasSwitchSignal {
			t.Fatalf("expected provider_switch signal in routed invocation, got %+v", result.Signals)
		}
	}

	t.Run("provider_switch", func(t *testing.T) {
		runRouting(t, "provider_switch", "provider_switch")
	})
	t.Run("fallback", func(t *testing.T) {
		runRouting(t, "fallback", "fallback")
	})
}

func TestLiveProviderSmokeChainedWorkflow(t *testing.T) {
	if os.Getenv("RSPP_LIVE_PROVIDER_SMOKE") != "1" {
		t.Skip("live provider smoke disabled (set RSPP_LIVE_PROVIDER_SMOKE=1)")
	}

	runtimeProviders, err := bootstrap.BuildMVPProviders()
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	maxCombos, maxComboSource, maxComboParseWarning := liveProviderChainComboLimit()
	reportPath, reportMDPath := liveProviderChainReportPaths()
	captureMode, captureMaxBytes := liveProviderIOCaptureConfig()
	enabled := enabledProvidersByModality()
	missing := missingModalities(enabled)
	allCombos := buildLiveProviderChainCombinations(enabled)
	selectedCombos := limitLiveProviderChainCombinations(allCombos, maxCombos)

	report := liveProviderChainReport{
		GeneratedAtUTC:            time.Now().UTC().Format(time.RFC3339),
		Status:                    "pending",
		ComboCap:                  maxCombos,
		ComboCapSource:            maxComboSource,
		ComboCapParseWarning:      maxComboParseWarning,
		ProviderIOCaptureMode:     captureMode,
		ProviderIOCaptureMaxBytes: captureMaxBytes,
		ReportPath:                reportPath,
		ReportMarkdownPath:        reportMDPath,
		EnabledProviders:          enabled,
		MissingModalities:         append([]string(nil), missing...),
		TotalCombinationCount:     len(allCombos),
		ExecutedCombinationCount:  len(selectedCombos),
		SelectedCombinations:      make([]liveProviderChainSelectedCombination, 0, len(selectedCombos)),
		Combinations:              make([]liveProviderChainCombinationReport, 0, len(selectedCombos)),
	}

	for i, combo := range selectedCombos {
		report.SelectedCombinations = append(report.SelectedCombinations, liveProviderChainSelectedCombination{
			ComboIndex:    i + 1,
			STTProviderID: combo.STTProviderID,
			LLMProviderID: combo.LLMProviderID,
			TTSProviderID: combo.TTSProviderID,
		})
	}

	if len(missing) > 0 {
		report.Status = "skip"
		report.SkipReason = fmt.Sprintf(
			"missing enabled providers by modality: %s (stt=%s llm=%s tts=%s)",
			strings.Join(missing, ","),
			joinOrNone(enabled.STT),
			joinOrNone(enabled.LLM),
			joinOrNone(enabled.TTS),
		)
		report.Aggregate.ComboSkipCount = 1
		if err := writeLiveProviderChainReport(report); err != nil {
			t.Fatalf("write live-provider chain report: %v", err)
		}
		t.Skip(report.SkipReason)
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{
		AttemptCapacity: maxInt(32, len(selectedCombos)*8),
	})
	scheduler := executor.NewSchedulerWithProviderInvokerAndAttemptAppender(
		localadmission.Evaluator{},
		runtimeProviders.Controller,
		&recorder,
	)
	handoffPolicy := executor.StreamingHandoffPolicyFromEnv()

	failureMessage := ""
	for i, combo := range selectedCombos {
		comboReport := liveProviderChainCombinationReport{
			ComboIndex:    i + 1,
			ComboID:       fmt.Sprintf("combo-%02d", i+1),
			STTProviderID: combo.STTProviderID,
			LLMProviderID: combo.LLMProviderID,
			TTSProviderID: combo.TTSProviderID,
			Status:        "pass",
			Steps:         make([]liveProviderChainStepReport, 0, 3),
		}

		comboBaseTimestamp := time.Now().UnixMilli() + int64(i*100)
		sessionID := "sess-live-provider-chain"
		turnID := fmt.Sprintf("turn-live-provider-chain-%02d", i+1)
		pipelineVersion := "pipeline-v1"
		baseEventID := fmt.Sprintf("evt-live-provider-chain-%02d", i+1)
		chainResult, chainErr := scheduler.ExecuteStreamingChain(
			executor.SchedulingInput{
				SessionID:            sessionID,
				TurnID:               turnID,
				PipelineVersion:      pipelineVersion,
				EventID:              baseEventID,
				TransportSequence:    1,
				RuntimeSequence:      1,
				AuthorityEpoch:       1,
				RuntimeTimestampMS:   comboBaseTimestamp,
				WallClockTimestampMS: comboBaseTimestamp,
			},
			executor.ProviderInvocationInput{
				Modality:               contracts.ModalitySTT,
				PreferredProvider:      combo.STTProviderID,
				AllowedAdaptiveActions: []string{"retry", "provider_switch", "fallback"},
				ProviderInvocationID:   fmt.Sprintf("pvi-live-provider-chain-%02d-stt", i+1),
				EnableStreaming:        true,
			},
			executor.ProviderInvocationInput{
				Modality:               contracts.ModalityLLM,
				PreferredProvider:      combo.LLMProviderID,
				AllowedAdaptiveActions: []string{"retry", "provider_switch", "fallback"},
				ProviderInvocationID:   fmt.Sprintf("pvi-live-provider-chain-%02d-llm", i+1),
				EnableStreaming:        true,
			},
			executor.ProviderInvocationInput{
				Modality:               contracts.ModalityTTS,
				PreferredProvider:      combo.TTSProviderID,
				AllowedAdaptiveActions: []string{"retry", "provider_switch", "fallback"},
				ProviderInvocationID:   fmt.Sprintf("pvi-live-provider-chain-%02d-tts", i+1),
				EnableStreaming:        true,
			},
			handoffPolicy,
		)

		comboReport.Latency = chainLatencyFromExecutor(chainResult.Latency)
		comboReport.Handoffs = chainHandoffsFromExecutor(chainResult.Handoffs)
		comboReport.CoalesceCount = chainResult.CoalesceCount
		comboReport.SupersedeCount = chainResult.SupersedeCount

		stages := []struct {
			stepName          string
			modality          contracts.Modality
			preferredProvider string
			stageResult       executor.StreamingStageResult
			stageIndex        int
		}{
			{stepName: "stt", modality: contracts.ModalitySTT, preferredProvider: combo.STTProviderID, stageResult: chainResult.STT, stageIndex: 0},
			{stepName: "llm", modality: contracts.ModalityLLM, preferredProvider: combo.LLMProviderID, stageResult: chainResult.LLM, stageIndex: 1},
			{stepName: "tts", modality: contracts.ModalityTTS, preferredProvider: combo.TTSProviderID, stageResult: chainResult.TTS, stageIndex: 2},
		}

		for _, stage := range stages {
			transportSequence := int64(stage.stageIndex + 1)
			runtimeSequence := int64(stage.stageIndex + 1)
			runtimeTS := comboBaseTimestamp + int64(stage.stageIndex)
			eventID := stage.stageResult.EventID
			if eventID == "" {
				eventID = fmt.Sprintf("%s-%s", baseEventID, stage.stepName)
			}

			stepReport := liveProviderChainStepReport{
				Step:                stage.stepName,
				Modality:            string(stage.modality),
				PreferredProviderID: stage.preferredProvider,
				Input: liveProviderChainStepInput{
					SessionID:          sessionID,
					TurnID:             turnID,
					PipelineVersion:    pipelineVersion,
					EventID:            eventID,
					TransportSequence:  transportSequence,
					RuntimeSequence:    runtimeSequence,
					AuthorityEpoch:     1,
					RuntimeTimestampMS: runtimeTS,
					WallClockMS:        runtimeTS,
					InputSnapshot:      chainStepInputSnapshot(stage.preferredProvider),
				},
				Output: liveProviderChainStepOutput{
					RetryDecision: "none",
					Attempts:      []liveProviderChainStepAttempt{},
					Signals:       []liveProviderChainStepSignal{},
				},
			}

			if stage.stageResult.Decision == nil {
				stepReport.Error = defaultString(stage.stageResult.Error, "provider_decision_missing")
				comboReport.Status = "fail"
				comboReport.FailureStep = stage.stepName
				comboReport.FailureReason = stepReport.Error
				report.Aggregate.StepFailureCount++
				comboReport.Steps = append(comboReport.Steps, stepReport)
				break
			}

			attempts := providerAttemptEntriesForInvocation(
				recorder.ProviderAttemptEntriesForTurn(sessionID, turnID),
				stage.stageResult.Decision.ProviderInvocationID,
			)
			stepReport.Output = chainStepOutputFromEvidence(*stage.stageResult.Decision, attempts)
			comboReport.Steps = append(comboReport.Steps, stepReport)

			if stage.stageResult.Decision.OutcomeClass != contracts.OutcomeSuccess {
				comboReport.Status = "fail"
				comboReport.FailureStep = stage.stepName
				comboReport.FailureReason = fmt.Sprintf("outcome_class=%s reason=%s", stepReport.Output.OutcomeClass, stepReport.Output.OutcomeReason)
				report.Aggregate.StepFailureCount++
				break
			}
			report.Aggregate.StepSuccessCount++
		}

		if chainErr != nil && comboReport.Status == "pass" {
			comboReport.Status = "fail"
			if comboReport.FailureStep == "" {
				comboReport.FailureStep = "chain"
			}
			comboReport.FailureReason = chainErr.Error()
			report.Aggregate.StepFailureCount++
		}

		if comboReport.Status == "fail" {
			failureMessage = fmt.Sprintf(
				"chained workflow failed for combo[%d] stt=%s llm=%s tts=%s: step=%s reason=%s",
				i+1,
				combo.STTProviderID,
				combo.LLMProviderID,
				combo.TTSProviderID,
				comboReport.FailureStep,
				comboReport.FailureReason,
			)
		}

		report.Combinations = append(report.Combinations, comboReport)
		if comboReport.Status == "pass" {
			report.Aggregate.ComboPassCount++
		} else if comboReport.Status == "fail" {
			report.Aggregate.ComboFailCount++
			break
		}
	}

	if failureMessage != "" {
		report.Status = "fail"
	} else {
		report.Status = "pass"
	}

	if err := writeLiveProviderChainReport(report); err != nil {
		t.Fatalf("write live-provider chain report: %v", err)
	}

	if failureMessage != "" {
		t.Fatalf("%s", failureMessage)
	}
}

func providerEnabled(tc liveProviderCase) (bool, string) {
	if os.Getenv(tc.enableEnv) != "1" {
		return false, fmt.Sprintf("provider %s disabled (%s != 1)", tc.providerID, tc.enableEnv)
	}
	for _, key := range tc.required {
		if os.Getenv(key) == "" {
			return false, fmt.Sprintf("provider %s missing required env %s", tc.providerID, key)
		}
	}
	return true, ""
}

func enabledProvidersByModality() chainEnabledProviders {
	enabled := chainEnabledProviders{
		STT: make([]string, 0, 3),
		LLM: make([]string, 0, 3),
		TTS: make([]string, 0, 3),
	}
	for _, tc := range liveProviderCases {
		isEnabled, _ := providerEnabled(tc)
		if !isEnabled {
			continue
		}
		switch tc.modality {
		case contracts.ModalitySTT:
			enabled.STT = append(enabled.STT, tc.providerID)
		case contracts.ModalityLLM:
			enabled.LLM = append(enabled.LLM, tc.providerID)
		case contracts.ModalityTTS:
			enabled.TTS = append(enabled.TTS, tc.providerID)
		}
	}
	sort.Strings(enabled.STT)
	sort.Strings(enabled.LLM)
	sort.Strings(enabled.TTS)
	return enabled
}

func missingModalities(enabled chainEnabledProviders) []string {
	missing := make([]string, 0, 3)
	if len(enabled.STT) == 0 {
		missing = append(missing, "stt")
	}
	if len(enabled.LLM) == 0 {
		missing = append(missing, "llm")
	}
	if len(enabled.TTS) == 0 {
		missing = append(missing, "tts")
	}
	return missing
}

func buildLiveProviderChainCombinations(enabled chainEnabledProviders) []liveProviderChainCombination {
	combos := make([]liveProviderChainCombination, 0, len(enabled.STT)*len(enabled.LLM)*len(enabled.TTS))
	for _, sttID := range enabled.STT {
		for _, llmID := range enabled.LLM {
			for _, ttsID := range enabled.TTS {
				combos = append(combos, liveProviderChainCombination{
					STTProviderID: sttID,
					LLMProviderID: llmID,
					TTSProviderID: ttsID,
				})
			}
		}
	}
	return combos
}

func limitLiveProviderChainCombinations(combos []liveProviderChainCombination, limit int) []liveProviderChainCombination {
	if limit < 1 {
		limit = 1
	}
	if len(combos) <= limit {
		return append([]liveProviderChainCombination(nil), combos...)
	}
	return append([]liveProviderChainCombination(nil), combos[:limit]...)
}

func liveProviderChainComboLimit() (int, string, string) {
	raw := strings.TrimSpace(os.Getenv("RSPP_LIVE_PROVIDER_CHAIN_MAX_COMBOS"))
	if raw == "" {
		return defaultLiveProviderChainMaxCombos, "default", ""
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return defaultLiveProviderChainMaxCombos, "fallback_default_invalid_env", fmt.Sprintf(
			"invalid RSPP_LIVE_PROVIDER_CHAIN_MAX_COMBOS=%q; using default %d",
			raw,
			defaultLiveProviderChainMaxCombos,
		)
	}
	return value, "env", ""
}

func liveProviderChainReportPaths() (string, string) {
	reportPath := strings.TrimSpace(os.Getenv("RSPP_LIVE_PROVIDER_CHAIN_REPORT_PATH"))
	if reportPath == "" {
		reportPath = defaultLiveProviderChainReportPath
	}
	reportMDPath := strings.TrimSpace(os.Getenv("RSPP_LIVE_PROVIDER_CHAIN_REPORT_MD_PATH"))
	if reportMDPath == "" {
		reportMDPath = defaultLiveProviderChainReportMDPath
	}
	return reportPath, reportMDPath
}

func liveProviderIOCaptureConfig() (string, int) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("RSPP_PROVIDER_IO_CAPTURE_MODE")))
	switch mode {
	case "full", "hash", "redacted":
	default:
		mode = defaultProviderIOCaptureMode
	}

	maxBytes := defaultProviderIOCaptureMaxBytes
	rawMaxBytes := strings.TrimSpace(os.Getenv("RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES"))
	if rawMaxBytes != "" {
		if value, err := strconv.Atoi(rawMaxBytes); err == nil && value >= minProviderIOCaptureMaxBytes {
			maxBytes = value
		}
	}
	return mode, maxBytes
}

func chainStepOutputFromEvidence(provider executor.ProviderDecision, attempts []timeline.ProviderAttemptEvidence) liveProviderChainStepOutput {
	retryDecision := provider.RetryDecision
	if retryDecision == "" {
		retryDecision = "none"
	}
	out := liveProviderChainStepOutput{
		SelectedProviderID: provider.SelectedProvider,
		OutcomeClass:       string(provider.OutcomeClass),
		Retryable:          provider.Retryable,
		RetryDecision:      retryDecision,
		AttemptCount:       len(attempts),
		Attempts:           make([]liveProviderChainStepAttempt, 0, len(attempts)),
		Signals:            make([]liveProviderChainStepSignal, 0, len(provider.Signals)),
	}
	for _, attempt := range attempts {
		out.Attempts = append(out.Attempts, liveProviderChainStepAttempt{
			ProviderID:       attempt.ProviderID,
			Attempt:          attempt.Attempt,
			OutcomeClass:     attempt.OutcomeClass,
			Reason:           attempt.OutcomeReason,
			Retryable:        attempt.Retryable,
			CircuitOpen:      attempt.CircuitOpen,
			BackoffMS:        attempt.BackoffMS,
			AttemptLatencyMS: attempt.AttemptLatencyMS,
			FirstChunkMS:     attempt.FirstChunkLatencyMS,
			ChunkCount:       attempt.ChunkCount,
			BytesOut:         attempt.BytesOut,
			StreamingUsed:    attempt.StreamingUsed,
			InputPayload:     attempt.InputPayload,
			OutputPayload:    attempt.OutputPayload,
			OutputStatusCode: attempt.OutputStatusCode,
			PayloadTruncated: attempt.PayloadTruncated,
		})
	}
	if len(attempts) > 0 {
		final := attempts[len(attempts)-1]
		out.OutcomeReason = final.OutcomeReason
		out.CircuitOpen = final.CircuitOpen
		out.BackoffMS = final.BackoffMS
		out.RawInputPayload = final.InputPayload
		out.RawOutputPayload = final.OutputPayload
		out.RawOutputStatusCode = final.OutputStatusCode
		out.RawPayloadTruncated = final.PayloadTruncated
	}
	if out.AttemptCount == 0 && provider.Attempts > 0 {
		out.AttemptCount = provider.Attempts
	}
	for _, signal := range provider.Signals {
		out.Signals = append(out.Signals, liveProviderChainStepSignal{
			Signal:    signal.Signal,
			Reason:    signal.Reason,
			EmittedBy: signal.EmittedBy,
		})
	}
	return out
}

func chainLatencyFromExecutor(in executor.StreamingChainLatency) liveProviderChainLatency {
	return liveProviderChainLatency{
		STTFirstPartialLatencyMS:      in.STTFirstPartialLatencyMS,
		STTPartialToLLMStartLatencyMS: in.STTPartialToLLMStartLatencyMS,
		LLMFirstPartialLatencyMS:      in.LLMFirstPartialLatencyMS,
		LLMPartialToTTSStartLatencyMS: in.LLMPartialToTTSStartLatencyMS,
		TTSFirstAudioLatencyMS:        in.TTSFirstAudioLatencyMS,
		FirstAssistantAudioE2EMS:      in.FirstAssistantAudioE2EMS,
		TurnCompletionE2EMS:           in.TurnCompletionE2EMS,
	}
}

func chainHandoffsFromExecutor(in []executor.StreamingHandoffEdgeResult) []liveProviderChainHandoff {
	if len(in) == 0 {
		return nil
	}
	out := make([]liveProviderChainHandoff, 0, len(in))
	for _, handoff := range in {
		out = append(out, liveProviderChainHandoff{
			HandoffID:             handoff.HandoffID,
			Edge:                  handoff.Edge,
			UpstreamRevision:      handoff.UpstreamRevision,
			Action:                handoff.Action,
			PartialAcceptedAtMS:   handoff.PartialAcceptedAtMS,
			DownstreamStartedAtMS: handoff.DownstreamStartedAtMS,
			HandoffLatencyMS:      handoff.HandoffLatencyMS,
			QueueDepth:            handoff.QueueDepth,
			WatermarkHigh:         handoff.WatermarkHigh,
		})
	}
	return out
}

func providerAttemptEntriesForInvocation(entries []timeline.ProviderAttemptEvidence, providerInvocationID string) []timeline.ProviderAttemptEvidence {
	filtered := make([]timeline.ProviderAttemptEvidence, 0, len(entries))
	for _, entry := range entries {
		if entry.ProviderInvocationID != providerInvocationID {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].RuntimeTimestampMS != filtered[j].RuntimeTimestampMS {
			return filtered[i].RuntimeTimestampMS < filtered[j].RuntimeTimestampMS
		}
		if filtered[i].WallClockTimestampMS != filtered[j].WallClockTimestampMS {
			return filtered[i].WallClockTimestampMS < filtered[j].WallClockTimestampMS
		}
		if filtered[i].RuntimeSequence != filtered[j].RuntimeSequence {
			return filtered[i].RuntimeSequence < filtered[j].RuntimeSequence
		}
		if filtered[i].TransportSequence != filtered[j].TransportSequence {
			return filtered[i].TransportSequence < filtered[j].TransportSequence
		}
		if filtered[i].Attempt != filtered[j].Attempt {
			return filtered[i].Attempt < filtered[j].Attempt
		}
		if filtered[i].ProviderID != filtered[j].ProviderID {
			return filtered[i].ProviderID < filtered[j].ProviderID
		}
		return filtered[i].EventID < filtered[j].EventID
	})
	return filtered
}

func chainStepInputSnapshot(providerID string) map[string]any {
	switch providerID {
	case sttdeepgramprovider.ProviderID:
		cfg := sttdeepgramprovider.ConfigFromEnv()
		return map[string]any{
			"audio_url": cfg.AudioURL,
			"model":     cfg.Model,
		}
	case sttassemblyaiprovider.ProviderID:
		cfg := sttassemblyaiprovider.ConfigFromEnv()
		return map[string]any{
			"audio_url":     cfg.AudioURL,
			"speech_models": cfg.SpeechModels,
		}
	case sttgoogleprovider.ProviderID:
		cfg := sttgoogleprovider.ConfigFromEnv()
		return map[string]any{
			"audio_uri":           cfg.AudioURI,
			"language":            cfg.Language,
			"model":               cfg.Model,
			"sample_rate_hz":      cfg.SampleRate,
			"audio_channel_count": cfg.ChannelMode,
		}
	case llmanthropicprovider.ProviderID:
		cfg := llmanthropicprovider.ConfigFromEnv()
		return map[string]any{
			"prompt":     cfg.Prompt,
			"model":      cfg.Model,
			"max_tokens": cfg.MaxTokens,
		}
	case llmgeminiprovider.ProviderID:
		cfg := llmgeminiprovider.ConfigFromEnv()
		return map[string]any{
			"prompt": cfg.Prompt,
		}
	case llmcohereprovider.ProviderID:
		cfg := llmcohereprovider.ConfigFromEnv()
		return map[string]any{
			"prompt":     cfg.Prompt,
			"model":      cfg.Model,
			"openrouter": cfg.OpenRouter,
			"max_tokens": cfg.MaxTokens,
		}
	case ttselevenlabsprovider.ProviderID:
		cfg := ttselevenlabsprovider.ConfigFromEnv()
		return map[string]any{
			"text":     cfg.Text,
			"voice_id": cfg.VoiceID,
			"model_id": cfg.ModelID,
		}
	case ttsgoogleprovider.ProviderID:
		cfg := ttsgoogleprovider.ConfigFromEnv()
		return map[string]any{
			"text":           cfg.SampleText,
			"voice_name":     cfg.VoiceName,
			"language":       cfg.Language,
			"audio_encoding": cfg.AudioFormat,
		}
	case ttspollyprovider.ProviderID:
		cfg := ttspollyprovider.ConfigFromEnv()
		return map[string]any{
			"text":     cfg.SampleText,
			"voice_id": cfg.VoiceID,
			"engine":   cfg.Engine,
			"region":   cfg.Region,
		}
	default:
		return map[string]any{}
	}
}

func writeLiveProviderChainReport(report liveProviderChainReport) error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(root, filepath.FromSlash(report.ReportPath))
	mdPath := filepath.Join(root, filepath.FromSlash(report.ReportMarkdownPath))

	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, payload, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(mdPath, []byte(renderLiveProviderChainMarkdown(report)), 0o644); err != nil {
		return err
	}
	return nil
}

func renderLiveProviderChainMarkdown(report liveProviderChainReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Live Provider Chained Workflow Report\n\n")
	fmt.Fprintf(&b, "- Generated at (UTC): `%s`\n", report.GeneratedAtUTC)
	fmt.Fprintf(&b, "- Status: `%s`\n", report.Status)
	fmt.Fprintf(&b, "- Combo cap: `%d` (`%s`)\n", report.ComboCap, report.ComboCapSource)
	if report.ComboCapParseWarning != "" {
		fmt.Fprintf(&b, "- Combo cap parse warning: %s\n", report.ComboCapParseWarning)
	}
	fmt.Fprintf(&b, "- Provider I/O capture mode: `%s`\n", report.ProviderIOCaptureMode)
	fmt.Fprintf(&b, "- Provider I/O capture max bytes: `%d`\n", report.ProviderIOCaptureMaxBytes)
	fmt.Fprintf(&b, "- Enabled STT providers: `%s`\n", joinOrNone(report.EnabledProviders.STT))
	fmt.Fprintf(&b, "- Enabled LLM providers: `%s`\n", joinOrNone(report.EnabledProviders.LLM))
	fmt.Fprintf(&b, "- Enabled TTS providers: `%s`\n", joinOrNone(report.EnabledProviders.TTS))
	fmt.Fprintf(&b, "- Combinations: total=%d executed=%d pass=%d fail=%d skip=%d\n\n",
		report.TotalCombinationCount,
		report.ExecutedCombinationCount,
		report.Aggregate.ComboPassCount,
		report.Aggregate.ComboFailCount,
		report.Aggregate.ComboSkipCount,
	)

	if len(report.MissingModalities) > 0 {
		fmt.Fprintf(&b, "## Missing Modalities\n\n")
		fmt.Fprintf(&b, "- Missing: `%s`\n", strings.Join(report.MissingModalities, ", "))
		if report.SkipReason != "" {
			fmt.Fprintf(&b, "- Skip reason: %s\n", report.SkipReason)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Selected Combinations\n\n")
	if len(report.SelectedCombinations) == 0 {
		fmt.Fprintf(&b, "- _none_\n\n")
	} else {
		fmt.Fprintf(&b, "| Combo | STT | LLM | TTS |\n")
		fmt.Fprintf(&b, "| --- | --- | --- | --- |\n")
		for _, combo := range report.SelectedCombinations {
			fmt.Fprintf(&b, "| `%02d` | `%s` | `%s` | `%s` |\n", combo.ComboIndex, combo.STTProviderID, combo.LLMProviderID, combo.TTSProviderID)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Streaming Handoff Latency\n\n")
	fmt.Fprintf(&b, "| Combo | STT->LLM (ms) | LLM->TTS (ms) | STT First Partial (ms) | LLM First Partial (ms) | TTS First Audio (ms) | First Audio E2E (ms) | Completion E2E (ms) | Coalesce | Supersede |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	if len(report.Combinations) == 0 {
		fmt.Fprintf(&b, "| _none_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ |\n\n")
	} else {
		for _, combo := range report.Combinations {
			fmt.Fprintf(
				&b,
				"| `%02d` | `%d` | `%d` | `%d` | `%d` | `%d` | `%d` | `%d` | `%d` | `%d` |\n",
				combo.ComboIndex,
				combo.Latency.STTPartialToLLMStartLatencyMS,
				combo.Latency.LLMPartialToTTSStartLatencyMS,
				combo.Latency.STTFirstPartialLatencyMS,
				combo.Latency.LLMFirstPartialLatencyMS,
				combo.Latency.TTSFirstAudioLatencyMS,
				combo.Latency.FirstAssistantAudioE2EMS,
				combo.Latency.TurnCompletionE2EMS,
				combo.CoalesceCount,
				combo.SupersedeCount,
			)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Per-Combo Step Outcomes\n\n")
	fmt.Fprintf(&b, "| Combo | Step | Preferred | Selected | Input | Outcome | Retry | Attempts | Latency (ms) | FirstChunk (ms) | Chunks | BytesOut | Streaming | Reason | Raw Status | Raw Input | Raw Output | Error |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	if len(report.Combinations) == 0 {
		fmt.Fprintf(&b, "| _none_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | `0` | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ |\n")
	} else {
		for _, combo := range report.Combinations {
			for _, step := range combo.Steps {
				latencyMS, firstChunkMS, chunks, bytesOut, streamingUsed := summarizeStepAttempts(step.Output.Attempts)
				fmt.Fprintf(
					&b,
					"| `%02d` | `%s` | `%s` | `%s` | %s | `%s` | `%s` | `%d` | `%d` | `%d` | `%d` | `%d` | `%t` | %s | `%d` | %s | %s | %s |\n",
					combo.ComboIndex,
					step.Step,
					step.PreferredProviderID,
					defaultString(step.Output.SelectedProviderID, "-"),
					escapeChainMarkdownCell(formatInputSnapshot(step.Input.InputSnapshot)),
					defaultString(step.Output.OutcomeClass, "-"),
					defaultString(step.Output.RetryDecision, "none"),
					step.Output.AttemptCount,
					latencyMS,
					firstChunkMS,
					chunks,
					bytesOut,
					streamingUsed,
					escapeChainMarkdownCell(defaultString(step.Output.OutcomeReason, "-")),
					step.Output.RawOutputStatusCode,
					escapeChainMarkdownCell(summarizeRawPayload(defaultString(step.Output.RawInputPayload, "-"))),
					escapeChainMarkdownCell(summarizeRawPayload(defaultString(step.Output.RawOutputPayload, "-"))),
					escapeChainMarkdownCell(defaultString(step.Error, "-")),
				)
			}
			if combo.Status == "fail" {
				fmt.Fprintf(
					&b,
					"| `%02d` | _failure_ | _n/a_ | _n/a_ | _n/a_ | `%s` | _n/a_ | `0` | _n/a_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | %s | _n/a_ | _n/a_ | _n/a_ | _n/a_ |\n",
					combo.ComboIndex,
					combo.FailureStep,
					escapeChainMarkdownCell(combo.FailureReason),
				)
			}
		}
	}

	return b.String()
}

func formatInputSnapshot(snapshot map[string]any) string {
	if len(snapshot) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, snapshot[key]))
	}
	return strings.Join(parts, "; ")
}

func summarizeRawPayload(value string) string {
	if len(value) <= 120 {
		return value
	}
	return value[:120] + "...(truncated)"
}

func summarizeStepAttempts(attempts []liveProviderChainStepAttempt) (int64, int64, int, int64, bool) {
	var latencyTotal int64
	var firstChunk int64
	firstChunkSet := false
	var chunkCount int
	var bytesOut int64
	streamingUsed := false
	for _, attempt := range attempts {
		latencyTotal += attempt.AttemptLatencyMS
		chunkCount += attempt.ChunkCount
		bytesOut += attempt.BytesOut
		streamingUsed = streamingUsed || attempt.StreamingUsed
		if attempt.FirstChunkMS > 0 && (!firstChunkSet || attempt.FirstChunkMS < firstChunk) {
			firstChunk = attempt.FirstChunkMS
			firstChunkSet = true
		}
	}
	if !firstChunkSet {
		firstChunk = 0
	}
	return latencyTotal, firstChunk, chunkCount, bytesOut, streamingUsed
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func escapeChainMarkdownCell(value string) string {
	v := strings.ReplaceAll(value, "\n", " ")
	v = strings.ReplaceAll(v, "|", "\\|")
	return strings.TrimSpace(v)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
