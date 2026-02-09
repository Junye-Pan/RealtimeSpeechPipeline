package gemini

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "llm-gemini"

type Config struct {
	APIKey   string
	Endpoint string
	Prompt   string
	Timeout  time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:   os.Getenv("RSPP_LLM_GEMINI_API_KEY"),
		Endpoint: defaultString(os.Getenv("RSPP_LLM_GEMINI_ENDPOINT"), "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent"),
		Prompt:   defaultString(os.Getenv("RSPP_LLM_GEMINI_PROMPT"), "Reply with the word: ok"),
		Timeout:  10 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return httpadapter.New(httpadapter.Config{
		ProviderID:       ProviderID,
		Modality:         contracts.ModalityLLM,
		Endpoint:         cfg.Endpoint,
		APIKey:           cfg.APIKey,
		QueryAPIKeyParam: "key",
		Timeout:          cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"contents": []map[string]any{
					{"parts": []map[string]any{{"text": cfg.Prompt}}},
				},
			}
		},
	})
}

func NewAdapterFromEnv() (contracts.Adapter, error) {
	return NewAdapter(ConfigFromEnv())
}

func defaultString(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
