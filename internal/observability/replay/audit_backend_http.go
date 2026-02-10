package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

const (
	// EnvReplayAuditHTTPURLs configures ordered comma-separated HTTP endpoints for replay audit appends.
	EnvReplayAuditHTTPURLs = "RSPP_REPLAY_AUDIT_HTTP_URLS"
	// EnvReplayAuditHTTPURL configures a single HTTP endpoint for replay audit appends.
	EnvReplayAuditHTTPURL = "RSPP_REPLAY_AUDIT_HTTP_URL"
	// EnvReplayAuditHTTPTimeoutMS configures replay-audit HTTP timeout in milliseconds.
	EnvReplayAuditHTTPTimeoutMS = "RSPP_REPLAY_AUDIT_HTTP_TIMEOUT_MS"
	// EnvReplayAuditHTTPAuthBearerToken configures replay-audit HTTP bearer auth token.
	EnvReplayAuditHTTPAuthBearerToken = "RSPP_REPLAY_AUDIT_HTTP_AUTH_BEARER_TOKEN"
	// EnvReplayAuditHTTPClientID configures replay-audit HTTP client identity header value.
	EnvReplayAuditHTTPClientID = "RSPP_REPLAY_AUDIT_HTTP_CLIENT_ID"
	// EnvReplayAuditHTTPRetryMaxAttempts configures max attempts per endpoint.
	EnvReplayAuditHTTPRetryMaxAttempts = "RSPP_REPLAY_AUDIT_HTTP_RETRY_MAX_ATTEMPTS"
	// EnvReplayAuditHTTPRetryBackoffMS configures base retry backoff in milliseconds.
	EnvReplayAuditHTTPRetryBackoffMS = "RSPP_REPLAY_AUDIT_HTTP_RETRY_BACKOFF_MS"
	// EnvReplayAuditHTTPRetryMaxBackoffMS configures max retry backoff in milliseconds.
	EnvReplayAuditHTTPRetryMaxBackoffMS = "RSPP_REPLAY_AUDIT_HTTP_RETRY_MAX_BACKOFF_MS"
	// EnvReplayAuditJSONLFallbackRootDir configures JSONL fallback root directory when HTTP backend is enabled.
	EnvReplayAuditJSONLFallbackRootDir = "RSPP_REPLAY_AUDIT_JSONL_FALLBACK_ROOT"

	defaultReplayAuditHTTPTimeoutMS     int64 = 2_000
	defaultReplayAuditRetryMaxAttempts  int64 = 2
	defaultReplayAuditRetryBackoffMS    int64 = 100
	defaultReplayAuditRetryMaxBackoffMS int64 = 1_000
	defaultReplayAuditJSONLFallbackRoot       = ".codex/replay/audit"
)

// HTTPAuditBackendConfig configures HTTP replay-audit append behavior.
type HTTPAuditBackendConfig struct {
	URL             string
	URLs            []string
	Timeout         time.Duration
	Client          *http.Client
	AuthBearerToken string
	ClientID        string

	RetryMaxAttempts int
	RetryBackoff     time.Duration
	RetryMaxBackoff  time.Duration
	Sleep            func(time.Duration)
}

// HTTPAuditBackend appends replay-audit events to ordered HTTP endpoints.
type HTTPAuditBackend struct {
	Config HTTPAuditBackendConfig
}

// HTTPAuditBackendResolver resolves tenant-scoped HTTP replay-audit backends.
type HTTPAuditBackendResolver struct {
	Config HTTPAuditBackendConfig
}

// FallbackAuditBackendResolver resolves primary/fallback tenant backends.
type FallbackAuditBackendResolver struct {
	Primary  ImmutableReplayAuditBackendResolver
	Fallback ImmutableReplayAuditBackendResolver
}

type fallbackAuditSink struct {
	primary  ImmutableReplayAuditSink
	fallback ImmutableReplayAuditSink
}

// HTTPAuditBackendConfigFromEnv resolves HTTP replay-audit backend config from env.
func HTTPAuditBackendConfigFromEnv() (HTTPAuditBackendConfig, error) {
	urls := parseReplayAuditOrderedEndpoints(strings.TrimSpace(os.Getenv(EnvReplayAuditHTTPURLs)))
	if len(urls) == 0 {
		if single := strings.TrimSpace(os.Getenv(EnvReplayAuditHTTPURL)); single != "" {
			urls = []string{single}
		}
	}
	if len(urls) == 0 {
		return HTTPAuditBackendConfig{}, fmt.Errorf("replay audit http endpoint is required")
	}

	timeout, err := parseReplayAuditPositiveDurationEnvMS(EnvReplayAuditHTTPTimeoutMS, defaultReplayAuditHTTPTimeoutMS)
	if err != nil {
		return HTTPAuditBackendConfig{}, err
	}
	retryMaxAttempts, err := parseReplayAuditPositiveIntEnv(EnvReplayAuditHTTPRetryMaxAttempts, defaultReplayAuditRetryMaxAttempts)
	if err != nil {
		return HTTPAuditBackendConfig{}, err
	}
	retryBackoff, err := parseReplayAuditPositiveDurationEnvMS(EnvReplayAuditHTTPRetryBackoffMS, defaultReplayAuditRetryBackoffMS)
	if err != nil {
		return HTTPAuditBackendConfig{}, err
	}
	retryMaxBackoff, err := parseReplayAuditPositiveDurationEnvMS(EnvReplayAuditHTTPRetryMaxBackoffMS, defaultReplayAuditRetryMaxBackoffMS)
	if err != nil {
		return HTTPAuditBackendConfig{}, err
	}

	return HTTPAuditBackendConfig{
		URL:              urls[0],
		URLs:             urls,
		Timeout:          timeout,
		AuthBearerToken:  strings.TrimSpace(os.Getenv(EnvReplayAuditHTTPAuthBearerToken)),
		ClientID:         strings.TrimSpace(os.Getenv(EnvReplayAuditHTTPClientID)),
		RetryMaxAttempts: retryMaxAttempts,
		RetryBackoff:     retryBackoff,
		RetryMaxBackoff:  retryMaxBackoff,
	}, nil
}

// NewReplayAuditBackendResolverFromEnv builds an env-configured replay-audit backend resolver.
func NewReplayAuditBackendResolverFromEnv() (ImmutableReplayAuditBackendResolver, error) {
	fallbackRoot := strings.TrimSpace(os.Getenv(EnvReplayAuditJSONLFallbackRootDir))
	if fallbackRoot == "" {
		fallbackRoot = defaultReplayAuditJSONLFallbackRoot
	}
	fallback := JSONLFileAuditBackendResolver{RootDir: fallbackRoot}

	if strings.TrimSpace(os.Getenv(EnvReplayAuditHTTPURLs)) == "" && strings.TrimSpace(os.Getenv(EnvReplayAuditHTTPURL)) == "" {
		return fallback, nil
	}

	cfg, err := HTTPAuditBackendConfigFromEnv()
	if err != nil {
		return nil, err
	}

	return FallbackAuditBackendResolver{
		Primary:  HTTPAuditBackendResolver{Config: cfg},
		Fallback: fallback,
	}, nil
}

// ResolveReplayAuditBackend resolves tenant-scoped HTTP replay-audit backend.
func (r HTTPAuditBackendResolver) ResolveReplayAuditBackend(_ string) (ImmutableReplayAuditSink, error) {
	normalized, err := normalizeHTTPAuditBackendConfig(r.Config)
	if err != nil {
		return nil, err
	}
	return &HTTPAuditBackend{Config: normalized}, nil
}

// ResolveReplayAuditBackend resolves tenant backend from primary/fallback resolvers.
func (r FallbackAuditBackendResolver) ResolveReplayAuditBackend(tenantID string) (ImmutableReplayAuditSink, error) {
	var primarySink ImmutableReplayAuditSink
	if r.Primary != nil {
		backend, err := r.Primary.ResolveReplayAuditBackend(tenantID)
		if err != nil {
			if r.Fallback == nil {
				return nil, fmt.Errorf("resolve primary replay audit backend: %w", err)
			}
		} else {
			primarySink = backend
		}
	}

	var fallbackSink ImmutableReplayAuditSink
	if r.Fallback != nil {
		backend, err := r.Fallback.ResolveReplayAuditBackend(tenantID)
		if err != nil {
			if primarySink == nil {
				return nil, fmt.Errorf("resolve fallback replay audit backend: %w", err)
			}
		} else {
			fallbackSink = backend
		}
	}

	switch {
	case primarySink != nil && fallbackSink != nil:
		return fallbackAuditSink{primary: primarySink, fallback: fallbackSink}, nil
	case primarySink != nil:
		return primarySink, nil
	case fallbackSink != nil:
		return fallbackSink, nil
	default:
		return nil, ErrReplayAuditBackendNil
	}
}

func (s fallbackAuditSink) AppendReplayAuditEvent(event obs.ReplayAuditEvent) error {
	primaryErr := error(nil)
	if s.primary != nil {
		if err := s.primary.AppendReplayAuditEvent(event); err == nil {
			return nil
		} else {
			primaryErr = err
		}
	}

	if s.fallback != nil {
		if err := s.fallback.AppendReplayAuditEvent(event); err == nil {
			return nil
		} else if primaryErr != nil {
			return fmt.Errorf("append replay audit event failed for primary (%v) and fallback (%w)", primaryErr, err)
		} else {
			return fmt.Errorf("append replay audit event to fallback backend: %w", err)
		}
	}

	if primaryErr != nil {
		return fmt.Errorf("append replay audit event to primary backend: %w", primaryErr)
	}
	return ErrReplayAuditBackendNil
}

// AppendReplayAuditEvent appends one validated replay-audit event to HTTP endpoints.
func (b *HTTPAuditBackend) AppendReplayAuditEvent(event obs.ReplayAuditEvent) error {
	if b == nil {
		return fmt.Errorf("http replay audit backend is required")
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid replay audit event: %w", err)
	}

	cfg, err := normalizeHTTPAuditBackendConfig(b.Config)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal replay audit event: %w", err)
	}

	endpointErrors := make([]error, 0, len(cfg.URLs))
	for _, endpoint := range cfg.URLs {
		appendErr := b.appendWithRetry(cfg, endpoint, payload, event.Request.TenantID)
		if appendErr == nil {
			return nil
		}
		endpointErrors = append(endpointErrors, appendErr)
	}

	if len(endpointErrors) == 0 {
		return fmt.Errorf("append replay audit event: no configured endpoints")
	}
	return fmt.Errorf("append replay audit event: all configured endpoints failed: %w", endpointErrors[0])
}

func (b *HTTPAuditBackend) appendWithRetry(cfg HTTPAuditBackendConfig, endpoint string, payload []byte, tenantID string) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.RetryMaxAttempts; attempt++ {
		err := appendReplayAuditEventOnce(cfg, endpoint, payload, tenantID)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == cfg.RetryMaxAttempts || !shouldRetryReplayAuditError(err) {
			break
		}
		cfg.Sleep(replayAuditRetryBackoffDuration(cfg.RetryBackoff, cfg.RetryMaxBackoff, attempt))
	}
	if lastErr == nil {
		return fmt.Errorf("append replay audit event: endpoint %s failed", endpoint)
	}
	return lastErr
}

func appendReplayAuditEventOnce(cfg HTTPAuditBackendConfig, endpoint string, payload []byte, tenantID string) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build replay audit request %s: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if cfg.AuthBearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthBearerToken)
	}
	if cfg.ClientID != "" {
		req.Header.Set("X-RSPP-Client-ID", cfg.ClientID)
	}
	if trimmedTenant := strings.TrimSpace(tenantID); trimmedTenant != "" {
		req.Header.Set("X-RSPP-Tenant-ID", trimmedTenant)
	}

	resp, err := cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("append replay audit event %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return replayAuditHTTPStatusError{StatusCode: resp.StatusCode}
	}
	return nil
}

func normalizeHTTPAuditBackendConfig(cfg HTTPAuditBackendConfig) (HTTPAuditBackendConfig, error) {
	urls := append([]string(nil), cfg.URLs...)
	if len(urls) == 0 {
		if single := strings.TrimSpace(cfg.URL); single != "" {
			urls = []string{single}
		}
	}
	if len(urls) == 0 {
		return HTTPAuditBackendConfig{}, fmt.Errorf("replay audit http endpoint is required")
	}

	normalizedURLs := make([]string, 0, len(urls))
	for _, raw := range urls {
		u, err := normalizeReplayAuditURL(raw)
		if err != nil {
			return HTTPAuditBackendConfig{}, err
		}
		normalizedURLs = append(normalizedURLs, u)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = time.Duration(defaultReplayAuditHTTPTimeoutMS) * time.Millisecond
	}
	retryMaxAttempts := cfg.RetryMaxAttempts
	if retryMaxAttempts < 1 {
		retryMaxAttempts = int(defaultReplayAuditRetryMaxAttempts)
	}
	retryBackoff := cfg.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = time.Duration(defaultReplayAuditRetryBackoffMS) * time.Millisecond
	}
	retryMaxBackoff := cfg.RetryMaxBackoff
	if retryMaxBackoff <= 0 {
		retryMaxBackoff = time.Duration(defaultReplayAuditRetryMaxBackoffMS) * time.Millisecond
	}
	if retryMaxBackoff < retryBackoff {
		retryMaxBackoff = retryBackoff
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	sleep := cfg.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	return HTTPAuditBackendConfig{
		URL:              normalizedURLs[0],
		URLs:             normalizedURLs,
		Timeout:          timeout,
		Client:           client,
		AuthBearerToken:  strings.TrimSpace(cfg.AuthBearerToken),
		ClientID:         strings.TrimSpace(cfg.ClientID),
		RetryMaxAttempts: retryMaxAttempts,
		RetryBackoff:     retryBackoff,
		RetryMaxBackoff:  retryMaxBackoff,
		Sleep:            sleep,
	}, nil
}

func parseReplayAuditOrderedEndpoints(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func parseReplayAuditPositiveDurationEnvMS(name string, fallbackMS int64) (time.Duration, error) {
	fallback := time.Duration(fallbackMS) * time.Millisecond
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if parsed < 1 {
		return 0, fmt.Errorf("parse %s: value must be >=1ms", name)
	}
	return time.Duration(parsed) * time.Millisecond, nil
}

func parseReplayAuditPositiveIntEnv(name string, fallback int64) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return int(fallback), nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if parsed < 1 {
		return 0, fmt.Errorf("parse %s: value must be >=1", name)
	}
	return int(parsed), nil
}

func normalizeReplayAuditURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("replay audit endpoint is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse replay audit endpoint %s: %w", trimmed, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("replay audit endpoint %s must use http or https", trimmed)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("replay audit endpoint %s host is required", trimmed)
	}
	return trimmed, nil
}

func replayAuditRetryBackoffDuration(base time.Duration, max time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = time.Duration(defaultReplayAuditRetryBackoffMS) * time.Millisecond
	}
	if max < base {
		max = base
	}
	if attempt < 1 {
		return base
	}

	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}

func shouldRetryReplayAuditError(err error) bool {
	var statusErr replayAuditHTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= http.StatusInternalServerError
	}
	return true
}

type replayAuditHTTPStatusError struct {
	StatusCode int
}

func (e replayAuditHTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status_code=%d", e.StatusCode)
}
