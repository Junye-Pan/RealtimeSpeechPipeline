package anthropic

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "llm-anthropic"

type Config struct {
	APIKey          string
	Endpoint        string
	Model           string
	Prompt          string
	AnthropicVerion string
	MaxTokens       int
	Timeout         time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:          os.Getenv("RSPP_LLM_ANTHROPIC_API_KEY"),
		Endpoint:        defaultString(os.Getenv("RSPP_LLM_ANTHROPIC_ENDPOINT"), "https://api.anthropic.com/v1/messages"),
		Model:           defaultString(os.Getenv("RSPP_LLM_ANTHROPIC_MODEL"), "claude-3-5-haiku-latest"),
		Prompt:          defaultString(os.Getenv("RSPP_LLM_ANTHROPIC_PROMPT"), "Reply with the word: ok"),
		AnthropicVerion: defaultString(os.Getenv("RSPP_LLM_ANTHROPIC_VERSION"), "2023-06-01"),
		MaxTokens:       16,
		Timeout:         10 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return httpadapter.New(httpadapter.Config{
		ProviderID:    ProviderID,
		Modality:      contracts.ModalityLLM,
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		APIKeyHeader:  "x-api-key",
		Timeout:       cfg.Timeout,
		StaticHeaders: map[string]string{"anthropic-version": cfg.AnthropicVerion},
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"model":      cfg.Model,
				"max_tokens": cfg.MaxTokens,
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
