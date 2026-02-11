package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// OTLPHTTPSinkConfig defines OTLP/HTTP export settings.
type OTLPHTTPSinkConfig struct {
	Endpoint    string
	ServiceName string
	Client      *http.Client
}

// OTLPHTTPSink emits telemetry over OTLP-friendly HTTP paths.
type OTLPHTTPSink struct {
	baseURL     *url.URL
	serviceName string
	client      *http.Client
}

// NewOTLPHTTPSink creates an OTLP/HTTP sink.
func NewOTLPHTTPSink(cfg OTLPHTTPSinkConfig) (*OTLPHTTPSink, error) {
	rawEndpoint := strings.TrimSpace(cfg.Endpoint)
	if rawEndpoint == "" {
		return nil, fmt.Errorf("otlp endpoint is required")
	}
	parsed, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse otlp endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("otlp endpoint must include scheme and host")
	}

	serviceName := strings.TrimSpace(cfg.ServiceName)
	if serviceName == "" {
		serviceName = "rspp-runtime"
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{}
	}

	return &OTLPHTTPSink{
		baseURL:     parsed,
		serviceName: serviceName,
		client:      client,
	}, nil
}

type otlpEnvelope struct {
	ServiceName string `json:"service_name"`
	Event       Event  `json:"event"`
}

// Export sends one event using kind-specific OTLP/HTTP routes.
func (s *OTLPHTTPSink) Export(ctx context.Context, event Event) error {
	if s == nil || s.baseURL == nil {
		return fmt.Errorf("otlp sink is not configured")
	}

	envelope := otlpEnvelope{
		ServiceName: s.serviceName,
		Event:       event,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal otlp event: %w", err)
	}

	u := *s.baseURL
	basePath := strings.TrimRight(u.Path, "/")
	u.Path = path.Join(basePath, otlpPathForKind(event.Kind))
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build otlp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("otlp export request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("otlp export status %d", resp.StatusCode)
	}
	return nil
}

func otlpPathForKind(kind EventKind) string {
	switch kind {
	case EventKindMetric:
		return "/v1/metrics"
	case EventKindSpan:
		return "/v1/traces"
	default:
		return "/v1/logs"
	}
}
