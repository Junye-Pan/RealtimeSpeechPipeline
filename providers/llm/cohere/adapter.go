package cohere

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "llm-cohere"

type Config struct {
	APIKey   string
	Endpoint string
	Model    string
	Prompt   string
	Timeout  time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:   os.Getenv("RSPP_LLM_COHERE_API_KEY"),
		Endpoint: defaultString(os.Getenv("RSPP_LLM_COHERE_ENDPOINT"), "https://api.cohere.com/v2/chat"),
		Model:    defaultString(os.Getenv("RSPP_LLM_COHERE_MODEL"), "command-r-plus"),
		Prompt:   defaultString(os.Getenv("RSPP_LLM_COHERE_PROMPT"), "Reply with the word: ok"),
		Timeout:  10 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return httpadapter.New(httpadapter.Config{
		ProviderID:   ProviderID,
		Modality:     contracts.ModalityLLM,
		Endpoint:     cfg.Endpoint,
		APIKey:       cfg.APIKey,
		APIKeyHeader: "Authorization",
		APIKeyPrefix: "Bearer ",
		Timeout:      cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"model": cfg.Model,
				"messages": []map[string]any{
					{"role": "user", "content": cfg.Prompt},
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
