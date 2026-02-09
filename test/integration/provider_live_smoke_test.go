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
