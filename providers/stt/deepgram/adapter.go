package deepgram

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "stt-deepgram"

type Config struct {
	APIKey   string
	Endpoint string
	Model    string
	AudioURL string
	Timeout  time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:   os.Getenv("RSPP_STT_DEEPGRAM_API_KEY"),
		Endpoint: defaultString(os.Getenv("RSPP_STT_DEEPGRAM_ENDPOINT"), "https://api.deepgram.com/v1/listen"),
		Model:    defaultString(os.Getenv("RSPP_STT_DEEPGRAM_MODEL"), "nova-2"),
		AudioURL: defaultString(os.Getenv("RSPP_STT_DEEPGRAM_AUDIO_URL"), "https://static.deepgram.com/examples/Bueller-Life-moves-pretty-fast.wav"),
		Timeout:  10 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return httpadapter.New(httpadapter.Config{
		ProviderID:    ProviderID,
		Modality:      contracts.ModalitySTT,
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		APIKeyHeader:  "Authorization",
		APIKeyPrefix:  "Token ",
		Timeout:       cfg.Timeout,
		StaticHeaders: map[string]string{"Accept": "application/json"},
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"url":   cfg.AudioURL,
				"model": cfg.Model,
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
