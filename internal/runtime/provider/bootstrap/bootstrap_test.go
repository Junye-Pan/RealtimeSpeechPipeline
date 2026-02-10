package bootstrap

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestBuildWithAdaptersCoverage(t *testing.T) {
	t.Parallel()

	adapters := []contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-b", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-c", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "llm-a", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-b", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-c", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "tts-a", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-b", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-c", Mode: contracts.ModalityTTS},
	}

	runtimeProviders, err := BuildWithAdapters(adapters, Options{MinProvidersPerModality: 3, MaxProvidersPerModality: 5})
	if err != nil {
		t.Fatalf("expected bootstrap success, got %v", err)
	}

	summary, err := Summary(runtimeProviders.Catalog)
	if err != nil {
		t.Fatalf("expected summary success, got %v", err)
	}
	if summary == "" {
		t.Fatalf("expected non-empty provider summary")
	}
}

func TestBuildWithAdaptersRejectsOutOfRangeCoverage(t *testing.T) {
	t.Parallel()

	adapters := []contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-b", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "llm-a", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-b", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-c", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "tts-a", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-b", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-c", Mode: contracts.ModalityTTS},
	}

	if _, err := BuildWithAdapters(adapters, Options{MinProvidersPerModality: 3, MaxProvidersPerModality: 5}); err == nil {
		t.Fatalf("expected coverage validation failure")
	}
}

func TestBuildWithAdaptersDefaultCoverageBounds(t *testing.T) {
	t.Parallel()

	adapters := []contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-b", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-c", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "llm-a", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-b", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-c", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "tts-a", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-b", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-c", Mode: contracts.ModalityTTS},
	}

	if _, err := BuildWithAdapters(adapters, Options{}); err != nil {
		t.Fatalf("expected default bootstrap coverage to accept 3 providers per modality, got %v", err)
	}
}

func TestBuildWithAdaptersDefaultCoverageRejectsAboveFive(t *testing.T) {
	t.Parallel()

	adapters := []contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-b", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-c", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-d", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-e", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-f", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "llm-a", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-b", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-c", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "tts-a", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-b", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-c", Mode: contracts.ModalityTTS},
	}

	if _, err := BuildWithAdapters(adapters, Options{}); err == nil {
		t.Fatalf("expected default bootstrap coverage to reject >5 providers per modality")
	}
}
