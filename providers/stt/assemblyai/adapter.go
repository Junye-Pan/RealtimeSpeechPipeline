package assemblyai

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "stt-assemblyai"

type Config struct {
	APIKey   string
	Endpoint string
	AudioURL string
	Timeout  time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:   os.Getenv("RSPP_STT_ASSEMBLYAI_API_KEY"),
		Endpoint: defaultString(os.Getenv("RSPP_STT_ASSEMBLYAI_ENDPOINT"), "https://api.assemblyai.com/v2/transcript"),
		AudioURL: defaultString(os.Getenv("RSPP_STT_ASSEMBLYAI_AUDIO_URL"), "https://static.deepgram.com/examples/Bueller-Life-moves-pretty-fast.wav"),
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
		Timeout:       cfg.Timeout,
		StaticHeaders: map[string]string{"Accept": "application/json"},
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"audio_url": cfg.AudioURL,
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
