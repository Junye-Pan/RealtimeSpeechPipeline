package google

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "tts-google"

type Config struct {
	APIKey      string
	Endpoint    string
	VoiceName   string
	Language    string
	SampleText  string
	AudioFormat string
	Timeout     time.Duration
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:      os.Getenv("RSPP_TTS_GOOGLE_API_KEY"),
		Endpoint:    defaultString(os.Getenv("RSPP_TTS_GOOGLE_ENDPOINT"), "https://texttospeech.googleapis.com/v1/text:synthesize"),
		VoiceName:   defaultString(os.Getenv("RSPP_TTS_GOOGLE_VOICE"), "en-US-Chirp3-HD-Achernar"),
		Language:    defaultString(os.Getenv("RSPP_TTS_GOOGLE_LANGUAGE"), "en-US"),
		SampleText:  defaultString(os.Getenv("RSPP_TTS_GOOGLE_TEXT"), "Realtime speech pipeline live smoke test."),
		AudioFormat: defaultString(os.Getenv("RSPP_TTS_GOOGLE_AUDIO_ENCODING"), "MP3"),
		Timeout:     15 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return httpadapter.New(httpadapter.Config{
		ProviderID:       ProviderID,
		Modality:         contracts.ModalityTTS,
		Endpoint:         cfg.Endpoint,
		APIKey:           cfg.APIKey,
		QueryAPIKeyParam: "key",
		Timeout:          cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"input":       map[string]any{"text": cfg.SampleText},
				"voice":       map[string]any{"name": cfg.VoiceName, "languageCode": cfg.Language},
				"audioConfig": map[string]any{"audioEncoding": cfg.AudioFormat},
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
