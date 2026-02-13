package livekit

import (
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestConfigValidateRequiresCredentialsWhenProbeEnabled(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		Probe:            true,
		RequestTimeout:   5 * time.Second,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected credential validation failure when probe enabled")
	}
}

func TestConfigValidateAllowsDryRunWithoutCredentials(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		RequestTimeout:   5 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected dry-run validation error: %v", err)
	}
}

func TestConfigValidateRejectsInvalidDefaultClass(t *testing.T) {
	t.Parallel()

	cfg := Config{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadMetadata,
		DryRun:           true,
		RequestTimeout:   5 * time.Second,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid default data class error")
	}
}
