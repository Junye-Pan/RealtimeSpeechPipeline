package bootstrap

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
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

// Options controls provider bootstrap invariants.
type Options struct {
	MinProvidersPerModality int
	MaxProvidersPerModality int
	MaxAttemptsPerProvider  int
	MaxCandidateProviders   int
}

// RuntimeProviders contains initialized provider manager components.
type RuntimeProviders struct {
	Catalog    registry.Catalog
	Controller invocation.Controller
}

// BuildMVPProviders creates the canonical 3x3x3 provider catalog.
func BuildMVPProviders() (RuntimeProviders, error) {
	return BuildMVPProvidersWithOptions(Options{})
}

// BuildMVPProvidersWithOptions creates providers with explicit options.
func BuildMVPProvidersWithOptions(opts Options) (RuntimeProviders, error) {
	adapters := make([]contracts.Adapter, 0, 9)

	constructors := []func() (contracts.Adapter, error){
		sttdeepgram.NewAdapterFromEnv,
		sttgoogle.NewAdapterFromEnv,
		sttassemblyai.NewAdapterFromEnv,
		llmanthropic.NewAdapterFromEnv,
		llmgemini.NewAdapterFromEnv,
		llmcohere.NewAdapterFromEnv,
		ttselevenlabs.NewAdapterFromEnv,
		ttsgoogle.NewAdapterFromEnv,
		ttspolly.NewAdapterFromEnv,
	}

	for _, constructor := range constructors {
		adapter, err := constructor()
		if err != nil {
			return RuntimeProviders{}, err
		}
		adapters = append(adapters, adapter)
	}

	return BuildWithAdapters(adapters, opts)
}

// BuildWithAdapters wires registry+controller for a given adapter set.
func BuildWithAdapters(adapters []contracts.Adapter, opts Options) (RuntimeProviders, error) {
	if opts.MinProvidersPerModality < 1 {
		opts.MinProvidersPerModality = 3
	}
	if opts.MaxProvidersPerModality < opts.MinProvidersPerModality {
		opts.MaxProvidersPerModality = 5
	}
	if opts.MaxAttemptsPerProvider < 1 {
		opts.MaxAttemptsPerProvider = 2
	}
	if opts.MaxCandidateProviders < 1 {
		opts.MaxCandidateProviders = opts.MaxProvidersPerModality
	}

	catalog, err := registry.NewCatalog(adapters)
	if err != nil {
		return RuntimeProviders{}, err
	}
	if err := catalog.ValidateCoverage(opts.MinProvidersPerModality, opts.MaxProvidersPerModality); err != nil {
		return RuntimeProviders{}, err
	}

	controller := invocation.NewControllerWithConfig(catalog, invocation.Config{
		MaxAttemptsPerProvider: opts.MaxAttemptsPerProvider,
		MaxCandidateProviders:  opts.MaxCandidateProviders,
	})

	return RuntimeProviders{Catalog: catalog, Controller: controller}, nil
}

// Summary returns deterministic provider counts by modality.
func Summary(catalog registry.Catalog) (string, error) {
	stt, err := catalog.ProviderIDs(contracts.ModalitySTT)
	if err != nil {
		return "", err
	}
	llm, err := catalog.ProviderIDs(contracts.ModalityLLM)
	if err != nil {
		return "", err
	}
	tts, err := catalog.ProviderIDs(contracts.ModalityTTS)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("providers initialized: stt=%d llm=%d tts=%d", len(stt), len(llm), len(tts)), nil
}
