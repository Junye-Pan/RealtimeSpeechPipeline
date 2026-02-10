//go:build liveproviders

package integration_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/bootstrap"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
)

type liveProviderCase struct {
	providerID string
	modality   contracts.Modality
	enableEnv  string
	required   []string
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

	cases := []liveProviderCase{
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

	for _, tc := range cases {
		tc := tc
		t.Run(tc.providerID, func(t *testing.T) {
			t.Parallel()

			if os.Getenv(tc.enableEnv) != "1" {
				t.Skipf("provider %s disabled (%s != 1)", tc.providerID, tc.enableEnv)
			}
			for _, key := range tc.required {
				if os.Getenv(key) == "" {
					t.Skipf("provider %s missing required env %s", tc.providerID, key)
				}
			}

			now := time.Now().UnixMilli()
			result, err := runtimeProviders.Controller.Invoke(invocation.InvocationInput{
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
			if err != nil {
				t.Fatalf("provider %s invocation error: %v", tc.providerID, err)
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

	cases := []liveProviderCase{
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

	var chosen *liveProviderCase
	for i := range cases {
		tc := &cases[i]
		if os.Getenv(tc.enableEnv) != "1" {
			continue
		}
		missing := false
		for _, key := range tc.required {
			if os.Getenv(key) == "" {
				missing = true
				break
			}
		}
		if missing {
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
		result, err := controller.Invoke(invocation.InvocationInput{
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
		if err != nil {
			t.Fatalf("unexpected routing invocation error: %v", err)
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
