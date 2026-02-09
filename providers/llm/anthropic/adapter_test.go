package anthropic

import "testing"

func TestConfigFromEnv_DefaultModelUsesHaiku(t *testing.T) {
	t.Setenv("RSPP_LLM_ANTHROPIC_MODEL", "")

	cfg := ConfigFromEnv()
	if cfg.Model != "claude-3-5-haiku-latest" {
		t.Fatalf("expected default anthropic model to be claude-3-5-haiku-latest, got %q", cfg.Model)
	}
}
