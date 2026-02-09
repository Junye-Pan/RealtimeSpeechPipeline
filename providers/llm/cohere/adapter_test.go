package cohere

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestConfigFromEnv_DefaultsToOpenRouter(t *testing.T) {
	t.Setenv("RSPP_LLM_COHERE_ENDPOINT", "")
	t.Setenv("RSPP_LLM_COHERE_MODEL", "")
	t.Setenv("RSPP_LLM_COHERE_OPENROUTER", "")

	cfg := ConfigFromEnv()
	if cfg.Endpoint != "https://openrouter.ai/api/v1/chat/completions" {
		t.Fatalf("unexpected endpoint default: %q", cfg.Endpoint)
	}
	if cfg.Model != "cohere/command-r-08-2024" {
		t.Fatalf("unexpected model default: %q", cfg.Model)
	}
}

func TestNewAdapter_OpenRouterPayloadAndHeaders(t *testing.T) {
	var gotAuth string
	var gotTitle string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotTitle = r.Header.Get("X-Title")
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{
		APIKey:          "test-key",
		Endpoint:        srv.URL,
		Model:           "cohere/command-r-08-2024",
		Prompt:          "Reply with the word: ok",
		OpenRouter:      true,
		OpenRouterTitle: "RealtimeSpeechPipeline",
		MaxTokens:       16,
		Timeout:         time.Second,
	})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	outcome, err := adapter.Invoke(contracts.InvocationRequest{
		SessionID:            "sess",
		TurnID:               "turn",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt",
		ProviderInvocationID: "pvi/sess/turn/evt/llm",
		ProviderID:           ProviderID,
		Modality:             contracts.ModalityLLM,
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

	if gotAuth != "Bearer test-key" {
		t.Fatalf("unexpected authorization header: %q", gotAuth)
	}
	if gotTitle != "RealtimeSpeechPipeline" {
		t.Fatalf("unexpected x-title header: %q", gotTitle)
	}
	if gotBody["model"] != "cohere/command-r-08-2024" {
		t.Fatalf("unexpected model in body: %v", gotBody["model"])
	}
	if gotBody["max_tokens"] != float64(16) {
		t.Fatalf("expected max_tokens=16, got %v", gotBody["max_tokens"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected messages payload, got %T", gotBody["messages"])
	}
	first, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first message object, got %T", msgs[0])
	}
	if first["role"] != "user" {
		t.Fatalf("unexpected role: %v", first["role"])
	}
	if first["content"] != "Reply with the word: ok" {
		t.Fatalf("unexpected content: %v", first["content"])
	}
}
