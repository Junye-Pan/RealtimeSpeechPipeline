package contract_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	llmanthropic "github.com/tiger/realtime-speech-pipeline/providers/llm/anthropic"
	llmcohere "github.com/tiger/realtime-speech-pipeline/providers/llm/cohere"
	llmgemini "github.com/tiger/realtime-speech-pipeline/providers/llm/gemini"
	sttassemblyai "github.com/tiger/realtime-speech-pipeline/providers/stt/assemblyai"
	sttdeepgram "github.com/tiger/realtime-speech-pipeline/providers/stt/deepgram"
	sttgoogle "github.com/tiger/realtime-speech-pipeline/providers/stt/google"
	ttselevenlabs "github.com/tiger/realtime-speech-pipeline/providers/tts/elevenlabs"
	ttsgoogle "github.com/tiger/realtime-speech-pipeline/providers/tts/google"
	ttspolly "github.com/tiger/realtime-speech-pipeline/providers/tts/polly"
)

func TestProviderAdapterContractConformance(t *testing.T) {
	t.Parallel()

	adapters := buildProviderAdaptersForContract(t)
	if len(adapters) < 9 {
		t.Fatalf("expected all MVP provider adapters, got %d", len(adapters))
	}

	counts := map[contracts.Modality]int{}
	for _, adapter := range adapters {
		counts[adapter.Modality()]++
	}
	for _, modality := range []contracts.Modality{contracts.ModalitySTT, contracts.ModalityLLM, contracts.ModalityTTS} {
		if counts[modality] < 3 {
			t.Fatalf("expected at least 3 adapters for modality %s, got %d", modality, counts[modality])
		}
	}

	for _, adapter := range adapters {
		adapter := adapter
		t.Run(adapter.ProviderID(), func(t *testing.T) {
			t.Parallel()
			req := contractInvocationRequest(adapter.ProviderID(), adapter.Modality())
			req.CancelRequested = true

			outcome, err := adapter.Invoke(req)
			if err != nil {
				t.Fatalf("invoke cancel-path returned error: %v", err)
			}
			if outcome.Class != contracts.OutcomeCancelled {
				t.Fatalf("expected cancelled outcome for cancel-path invoke, got %s", outcome.Class)
			}
			if err := outcome.Validate(); err != nil {
				t.Fatalf("cancel outcome failed validation: %v", err)
			}

			if streaming, ok := adapter.(contracts.StreamingAdapter); ok {
				streamOutcome, streamErr := streaming.InvokeStream(req, contracts.NoopStreamObserver{})
				if streamErr != nil {
					t.Fatalf("invoke stream cancel-path returned error: %v", streamErr)
				}
				if streamOutcome.Class != contracts.OutcomeCancelled {
					t.Fatalf("expected cancelled outcome for cancel-path stream invoke, got %s", streamOutcome.Class)
				}
				if err := streamOutcome.Validate(); err != nil {
					t.Fatalf("stream cancel outcome failed validation: %v", err)
				}
			}
		})
	}
}

func buildProviderAdaptersForContract(t *testing.T) []contracts.Adapter {
	t.Helper()

	constructors := []struct {
		name string
		new  func() (contracts.Adapter, error)
	}{
		{name: llmanthropic.ProviderID, new: llmanthropic.NewAdapterFromEnv},
		{name: llmcohere.ProviderID, new: llmcohere.NewAdapterFromEnv},
		{name: llmgemini.ProviderID, new: llmgemini.NewAdapterFromEnv},
		{name: sttassemblyai.ProviderID, new: sttassemblyai.NewAdapterFromEnv},
		{name: sttdeepgram.ProviderID, new: sttdeepgram.NewAdapterFromEnv},
		{name: sttgoogle.ProviderID, new: sttgoogle.NewAdapterFromEnv},
		{name: ttselevenlabs.ProviderID, new: ttselevenlabs.NewAdapterFromEnv},
		{name: ttsgoogle.ProviderID, new: ttsgoogle.NewAdapterFromEnv},
		{name: ttspolly.ProviderID, new: ttspolly.NewAdapterFromEnv},
	}

	adapters := make([]contracts.Adapter, 0, len(constructors))
	for _, constructor := range constructors {
		adapter, err := constructor.new()
		if err != nil {
			t.Fatalf("build %s adapter: %v", constructor.name, err)
		}
		adapters = append(adapters, adapter)
	}
	return adapters
}

func contractInvocationRequest(providerID string, modality contracts.Modality) contracts.InvocationRequest {
	return contracts.InvocationRequest{
		SessionID:            "sess-provider-contract",
		TurnID:               "turn-provider-contract",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-provider-contract",
		ProviderInvocationID: "pvi/provider-contract",
		ProviderID:           providerID,
		Modality:             modality,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	}
}
