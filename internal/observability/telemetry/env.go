package telemetry

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// EnvTelemetryEnabled toggles OR-01 runtime telemetry emission.
	EnvTelemetryEnabled = "RSPP_TELEMETRY_ENABLED"
	// EnvTelemetryOTLPHTTPEndpoint sets OTLP/HTTP endpoint base URL.
	EnvTelemetryOTLPHTTPEndpoint = "RSPP_TELEMETRY_OTLP_HTTP_ENDPOINT"
	// EnvTelemetryQueueCapacity sets in-memory queue capacity.
	EnvTelemetryQueueCapacity = "RSPP_TELEMETRY_QUEUE_CAPACITY"
	// EnvTelemetryDropSampleRate sets deterministic debug-log sample rate.
	EnvTelemetryDropSampleRate = "RSPP_TELEMETRY_DROP_SAMPLE_RATE"
	// EnvTelemetryExportTimeoutMS sets export timeout in milliseconds.
	EnvTelemetryExportTimeoutMS = "RSPP_TELEMETRY_EXPORT_TIMEOUT_MS"
)

// RuntimeConfig captures env-configured telemetry settings.
type RuntimeConfig struct {
	Enabled          bool
	OTLPHTTPEndpoint string
	QueueCapacity    int
	LogSampleRate    int
	ExportTimeoutMS  int
}

// RuntimeConfigFromEnv parses telemetry config from environment.
func RuntimeConfigFromEnv() (RuntimeConfig, error) {
	cfg := RuntimeConfig{
		Enabled:          true,
		OTLPHTTPEndpoint: strings.TrimSpace(os.Getenv(EnvTelemetryOTLPHTTPEndpoint)),
		QueueCapacity:    256,
		LogSampleRate:    1,
		ExportTimeoutMS:  200,
	}

	if raw := strings.TrimSpace(os.Getenv(EnvTelemetryEnabled)); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return RuntimeConfig{}, fmt.Errorf("%s parse error: %w", EnvTelemetryEnabled, err)
		}
		cfg.Enabled = enabled
	}
	if raw := strings.TrimSpace(os.Getenv(EnvTelemetryQueueCapacity)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return RuntimeConfig{}, fmt.Errorf("%s must be integer >=1", EnvTelemetryQueueCapacity)
		}
		cfg.QueueCapacity = v
	}
	if raw := strings.TrimSpace(os.Getenv(EnvTelemetryDropSampleRate)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return RuntimeConfig{}, fmt.Errorf("%s must be integer >=1", EnvTelemetryDropSampleRate)
		}
		cfg.LogSampleRate = v
	}
	if raw := strings.TrimSpace(os.Getenv(EnvTelemetryExportTimeoutMS)); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return RuntimeConfig{}, fmt.Errorf("%s must be integer >=1", EnvTelemetryExportTimeoutMS)
		}
		cfg.ExportTimeoutMS = v
	}

	return cfg, nil
}

// NewPipelineFromEnv creates a telemetry pipeline from environment settings.
func NewPipelineFromEnv() (*Pipeline, error) {
	cfg, err := RuntimeConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, nil
	}

	sink := Sink(discardSink{})
	if cfg.OTLPHTTPEndpoint != "" {
		httpSink, err := NewOTLPHTTPSink(OTLPHTTPSinkConfig{
			Endpoint: cfg.OTLPHTTPEndpoint,
			Client:   &http.Client{Timeout: time.Duration(cfg.ExportTimeoutMS) * time.Millisecond},
		})
		if err != nil {
			return nil, err
		}
		sink = httpSink
	}

	return NewPipeline(sink, Config{
		QueueCapacity: cfg.QueueCapacity,
		LogSampleRate: cfg.LogSampleRate,
		ExportTimeout: time.Duration(cfg.ExportTimeoutMS) * time.Millisecond,
	}), nil
}
