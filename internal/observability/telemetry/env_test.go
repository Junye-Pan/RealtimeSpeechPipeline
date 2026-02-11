package telemetry

import "testing"

func TestRuntimeConfigFromEnvDefaults(t *testing.T) {
	cfg, err := RuntimeConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected default env parse error: %v", err)
	}
	if !cfg.Enabled || cfg.QueueCapacity != 256 || cfg.LogSampleRate != 1 || cfg.ExportTimeoutMS != 200 {
		t.Fatalf("unexpected default config: %+v", cfg)
	}
}

func TestRuntimeConfigFromEnvRejectsInvalidValues(t *testing.T) {
	t.Run("invalid_enabled", func(t *testing.T) {
		t.Setenv(EnvTelemetryEnabled, "not-bool")
		if _, err := RuntimeConfigFromEnv(); err == nil {
			t.Fatalf("expected invalid enabled parse error")
		}
	})

	t.Run("invalid_queue_capacity", func(t *testing.T) {
		t.Setenv(EnvTelemetryQueueCapacity, "0")
		if _, err := RuntimeConfigFromEnv(); err == nil {
			t.Fatalf("expected queue capacity validation error")
		}
	})

	t.Run("invalid_sample_rate", func(t *testing.T) {
		t.Setenv(EnvTelemetryDropSampleRate, "-1")
		if _, err := RuntimeConfigFromEnv(); err == nil {
			t.Fatalf("expected sample rate validation error")
		}
	})

	t.Run("invalid_timeout", func(t *testing.T) {
		t.Setenv(EnvTelemetryExportTimeoutMS, "abc")
		if _, err := RuntimeConfigFromEnv(); err == nil {
			t.Fatalf("expected timeout validation error")
		}
	})
}

func TestNewPipelineFromEnv(t *testing.T) {
	t.Run("disabled_returns_nil", func(t *testing.T) {
		t.Setenv(EnvTelemetryEnabled, "false")
		pipeline, err := NewPipelineFromEnv()
		if err != nil {
			t.Fatalf("unexpected disabled env error: %v", err)
		}
		if pipeline != nil {
			t.Fatalf("expected nil pipeline when telemetry disabled")
		}
	})

	t.Run("invalid_endpoint_fails", func(t *testing.T) {
		t.Setenv(EnvTelemetryEnabled, "true")
		t.Setenv(EnvTelemetryOTLPHTTPEndpoint, "localhost:4318")
		if _, err := NewPipelineFromEnv(); err == nil {
			t.Fatalf("expected invalid endpoint to fail")
		}
	})

	t.Run("enabled_without_endpoint_uses_local_pipeline", func(t *testing.T) {
		t.Setenv(EnvTelemetryEnabled, "true")
		t.Setenv(EnvTelemetryOTLPHTTPEndpoint, "")
		pipeline, err := NewPipelineFromEnv()
		if err != nil {
			t.Fatalf("unexpected pipeline creation error: %v", err)
		}
		if pipeline == nil {
			t.Fatalf("expected non-nil pipeline when telemetry enabled")
		}
		if err := pipeline.Close(); err != nil {
			t.Fatalf("unexpected close error: %v", err)
		}
	})
}
