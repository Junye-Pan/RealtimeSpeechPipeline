package google

import (
	"os"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/providers/common/httpadapter"
)

const ProviderID = "stt-google"

type Config struct {
	APIKey      string
	Endpoint    string
	Language    string
	Model       string
	AudioURI    string
	Timeout     time.Duration
	EnablePunc  bool
	SampleRate  int
	ChannelMode int
}

func ConfigFromEnv() Config {
	return Config{
		APIKey:      os.Getenv("RSPP_STT_GOOGLE_API_KEY"),
		Endpoint:    defaultString(os.Getenv("RSPP_STT_GOOGLE_ENDPOINT"), "https://speech.googleapis.com/v1/speech:recognize"),
		Language:    defaultString(os.Getenv("RSPP_STT_GOOGLE_LANGUAGE"), "en-US"),
		Model:       defaultString(os.Getenv("RSPP_STT_GOOGLE_MODEL"), "latest_long"),
		AudioURI:    defaultString(os.Getenv("RSPP_STT_GOOGLE_AUDIO_URI"), "gs://cloud-samples-data/speech/brooklyn_bridge.raw"),
		Timeout:     10 * time.Second,
		EnablePunc:  true,
		SampleRate:  16000,
		ChannelMode: 1,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return httpadapter.New(httpadapter.Config{
		ProviderID:       ProviderID,
		Modality:         contracts.ModalitySTT,
		Endpoint:         cfg.Endpoint,
		APIKey:           cfg.APIKey,
		QueryAPIKeyParam: "key",
		Timeout:          cfg.Timeout,
		BuildBody: func(req contracts.InvocationRequest) any {
			return map[string]any{
				"config": map[string]any{
					"languageCode":               cfg.Language,
					"model":                      cfg.Model,
					"enableAutomaticPunctuation": cfg.EnablePunc,
					"audioChannelCount":          cfg.ChannelMode,
					"sampleRateHertz":            cfg.SampleRate,
				},
				"audio": map[string]any{
					"uri": cfg.AudioURI,
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
