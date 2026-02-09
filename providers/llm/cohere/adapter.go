package cohere

import (
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "llm-cohere"

type Config struct {
	APIKey            string
	Endpoint          string
	Model             string
	Prompt            string
	OpenRouter        bool
	OpenRouterReferer string
	OpenRouterTitle   string
	MaxTokens         int
	Timeout           time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:            os.Getenv("RSPP_LLM_COHERE_API_KEY"),
		Endpoint:          defaultString(os.Getenv("RSPP_LLM_COHERE_ENDPOINT"), "https://openrouter.ai/api/v1/chat/completions"),
		Model:             defaultString(os.Getenv("RSPP_LLM_COHERE_MODEL"), "cohere/command-r-08-2024"),
		Prompt:            defaultString(os.Getenv("RSPP_LLM_COHERE_PROMPT"), "Reply with the word: ok"),
		OpenRouter:        defaultBool(os.Getenv("RSPP_LLM_COHERE_OPENROUTER"), false),
		OpenRouterReferer: os.Getenv("RSPP_LLM_COHERE_OPENROUTER_REFERER"),
		OpenRouterTitle:   defaultString(os.Getenv("RSPP_LLM_COHERE_OPENROUTER_TITLE"), "RealtimeSpeechPipeline"),
		MaxTokens:         16,
		Timeout:           10 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	openRouter := shouldUseOpenRouter(cfg)
	staticHeaders := map[string]string{}
	if openRouter {
		if cfg.OpenRouterReferer != "" {
			staticHeaders["HTTP-Referer"] = cfg.OpenRouterReferer
		}
		if cfg.OpenRouterTitle != "" {
			staticHeaders["X-Title"] = cfg.OpenRouterTitle
		}
	}

	return httpadapter.New(httpadapter.Config{
		ProviderID:    ProviderID,
		Modality:      contracts.ModalityLLM,
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		APIKeyHeader:  "Authorization",
		APIKeyPrefix:  "Bearer ",
		StaticHeaders: staticHeaders,
		Timeout:       cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			if openRouter {
				return map[string]any{
					"model":      cfg.Model,
					"max_tokens": cfg.MaxTokens,
					"messages": []map[string]any{
						{"role": "user", "content": cfg.Prompt},
					},
				}
			}
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

func defaultBool(v string, fallback bool) bool {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func shouldUseOpenRouter(cfg Config) bool {
	if cfg.OpenRouter {
		return true
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "openrouter.ai")
}
