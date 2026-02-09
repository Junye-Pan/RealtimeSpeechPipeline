package elevenlabs

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "tts-elevenlabs"

type Config struct {
	APIKey   string
	Endpoint string
	VoiceID  string
	ModelID  string
	Text     string
	Timeout  time.Duration
}

func ConfigFromEnv() Config {
	voiceID := defaultString(os.Getenv("RSPP_TTS_ELEVENLABS_VOICE_ID"), "EXAVITQu4vr4xnSDxMaL")
	return Config{
		APIKey:   os.Getenv("RSPP_TTS_ELEVENLABS_API_KEY"),
		Endpoint: defaultString(os.Getenv("RSPP_TTS_ELEVENLABS_ENDPOINT"), "https://api.elevenlabs.io/v1/text-to-speech/"+voiceID),
		VoiceID:  voiceID,
		ModelID:  defaultString(os.Getenv("RSPP_TTS_ELEVENLABS_MODEL"), "eleven_multilingual_v2"),
		Text:     defaultString(os.Getenv("RSPP_TTS_ELEVENLABS_TEXT"), "Realtime speech pipeline live smoke test."),
		Timeout:  15 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return httpadapter.New(httpadapter.Config{
		ProviderID:    ProviderID,
		Modality:      contracts.ModalityTTS,
		Endpoint:      cfg.Endpoint,
		APIKey:        cfg.APIKey,
		APIKeyHeader:  "xi-api-key",
		Timeout:       cfg.Timeout,
		StaticHeaders: map[string]string{"Accept": "audio/mpeg"},
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"model_id": cfg.ModelID,
				"text":     cfg.Text,
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
