package registry

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestCandidatesPreferredProviderFirst(t *testing.T) {
	t.Parallel()

	catalog, err := NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-c", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-b", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "llm-a", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-b", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-c", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "tts-a", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-b", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-c", Mode: contracts.ModalityTTS},
	})
	if err != nil {
		t.Fatalf("unexpected catalog build error: %v", err)
	}

	candidates, err := catalog.Candidates(contracts.ModalitySTT, "stt-b", 5)
	if err != nil {
		t.Fatalf("unexpected candidates error: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
	if candidates[0].ProviderID() != "stt-b" {
		t.Fatalf("expected preferred provider first, got %s", candidates[0].ProviderID())
	}
}

func TestCandidatesUnknownPreferredProviderFails(t *testing.T) {
	t.Parallel()

	catalog, err := NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
	})
	if err != nil {
		t.Fatalf("unexpected catalog build error: %v", err)
	}

	_, err = catalog.Candidates(contracts.ModalitySTT, "stt-z", 5)
	if err == nil {
		t.Fatalf("expected unknown preferred provider to fail")
	}
}

func TestValidateCoverageRequiresThreeToFiveProvidersPerModality(t *testing.T) {
	t.Parallel()

	catalog, err := NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-b", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "stt-c", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "llm-a", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-b", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "llm-c", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "tts-a", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-b", Mode: contracts.ModalityTTS},
		contracts.StaticAdapter{ID: "tts-c", Mode: contracts.ModalityTTS},
	})
	if err != nil {
		t.Fatalf("unexpected catalog build error: %v", err)
	}

	if err := catalog.ValidateCoverage(3, 5); err != nil {
		t.Fatalf("expected coverage check to pass, got %v", err)
	}
}
