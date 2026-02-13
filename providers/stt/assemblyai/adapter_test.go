package assemblyai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestConfigFromEnv_DefaultSpeechModels(t *testing.T) {
	t.Setenv("RSPP_STT_ASSEMBLYAI_SPEECH_MODELS", "")

	cfg := ConfigFromEnv()
	if len(cfg.SpeechModels) != 1 || cfg.SpeechModels[0] != "universal-2" {
		t.Fatalf("expected default speech_models [universal-2], got %+v", cfg.SpeechModels)
	}
}

func TestNewAdapter_IncludesSpeechModels(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{
		APIKey:       "key",
		Endpoint:     srv.URL,
		AudioURL:     "https://example.com/audio.wav",
		SpeechModels: []string{"universal-3-pro", "universal-2"},
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	outcome, err := adapter.Invoke(contracts.InvocationRequest{
		SessionID:            "sess",
		TurnID:               "turn",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt",
		ProviderInvocationID: "pvi/sess/turn/evt/stt",
		ProviderID:           ProviderID,
		Modality:             contracts.ModalitySTT,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected success outcome, got class=%s reason=%s", outcome.Class, outcome.Reason)
	}

	if gotBody["audio_url"] != "https://example.com/audio.wav" {
		t.Fatalf("unexpected audio_url: %v", gotBody["audio_url"])
	}
	models, ok := gotBody["speech_models"].([]any)
	if !ok {
		t.Fatalf("expected speech_models array, got %T", gotBody["speech_models"])
	}
	if len(models) != 2 {
		t.Fatalf("expected two speech_models entries, got %d", len(models))
	}
	if models[0] != "universal-3-pro" || models[1] != "universal-2" {
		t.Fatalf("unexpected speech_models content: %+v", models)
	}
}
