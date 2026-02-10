package distribution

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

const (
	// EnvHTTPAdapterURLs configures ordered comma-separated HTTP endpoints for CP distribution fetch.
	EnvHTTPAdapterURLs = "RSPP_CP_DISTRIBUTION_HTTP_URLS"
	// EnvHTTPAdapterURL configures a single HTTP endpoint for CP distribution fetch.
	EnvHTTPAdapterURL = "RSPP_CP_DISTRIBUTION_HTTP_URL"
	// EnvHTTPAdapterTimeoutMS configures HTTP adapter timeout in milliseconds.
	EnvHTTPAdapterTimeoutMS = "RSPP_CP_DISTRIBUTION_HTTP_TIMEOUT_MS"
	// EnvHTTPAdapterAuthBearerToken configures bearer auth token for HTTP snapshot fetch.
	EnvHTTPAdapterAuthBearerToken = "RSPP_CP_DISTRIBUTION_HTTP_AUTH_BEARER_TOKEN"
	// EnvHTTPAdapterClientID configures optional client identity header value.
	EnvHTTPAdapterClientID = "RSPP_CP_DISTRIBUTION_HTTP_CLIENT_ID"
	// EnvHTTPAdapterRetryMaxAttempts configures max attempts per endpoint.
	EnvHTTPAdapterRetryMaxAttempts = "RSPP_CP_DISTRIBUTION_HTTP_RETRY_MAX_ATTEMPTS"
	// EnvHTTPAdapterRetryBackoffMS configures base retry backoff in milliseconds.
	EnvHTTPAdapterRetryBackoffMS = "RSPP_CP_DISTRIBUTION_HTTP_RETRY_BACKOFF_MS"
	// EnvHTTPAdapterRetryMaxBackoffMS configures maximum retry backoff in milliseconds.
	EnvHTTPAdapterRetryMaxBackoffMS = "RSPP_CP_DISTRIBUTION_HTTP_RETRY_MAX_BACKOFF_MS"
	// EnvHTTPAdapterCacheTTLMS configures cache ttl in milliseconds.
	EnvHTTPAdapterCacheTTLMS = "RSPP_CP_DISTRIBUTION_HTTP_CACHE_TTL_MS"
	// EnvHTTPAdapterMaxStalenessMS configures max stale-serving window beyond ttl in milliseconds.
	EnvHTTPAdapterMaxStalenessMS = "RSPP_CP_DISTRIBUTION_HTTP_MAX_STALENESS_MS"

	// defaultHTTPAdapterTimeoutMS keeps service-client snapshot fetch deterministic and bounded.
	defaultHTTPAdapterTimeoutMS int64 = 2_000
	// defaultHTTPAdapterRetryMaxAttempts controls per-endpoint transient recovery.
	defaultHTTPAdapterRetryMaxAttempts int64 = 2
	// defaultHTTPAdapterRetryBackoffMS sets bounded retry pacing baseline.
	defaultHTTPAdapterRetryBackoffMS int64 = 100
	// defaultHTTPAdapterRetryMaxBackoffMS caps exponential retry pacing.
	defaultHTTPAdapterRetryMaxBackoffMS int64 = 1_000
	// defaultHTTPAdapterCacheTTLMS bounds in-memory snapshot reuse before refresh.
	defaultHTTPAdapterCacheTTLMS int64 = 2_000
	// defaultHTTPAdapterMaxStalenessMS bounds stale-serving window after ttl on refresh failure.
	defaultHTTPAdapterMaxStalenessMS int64 = 10_000
)

// HTTPAdapterConfig configures a service-client HTTP CP snapshot distribution adapter.
type HTTPAdapterConfig struct {
	URL             string
	URLs            []string
	Timeout         time.Duration
	Client          *http.Client
	AuthBearerToken string
	ClientID        string

	RetryMaxAttempts int
	RetryBackoff     time.Duration
	RetryMaxBackoff  time.Duration
	CacheTTL         time.Duration
	MaxStaleness     time.Duration

	Now   func() time.Time
	Sleep func(time.Duration)
}

// HTTPAdapterConfigFromEnv resolves adapter config from environment.
func HTTPAdapterConfigFromEnv() (HTTPAdapterConfig, error) {
	var urls []string
	if rawURLs := strings.TrimSpace(os.Getenv(EnvHTTPAdapterURLs)); rawURLs != "" {
		urls = parseOrderedEndpoints(rawURLs)
	}
	if len(urls) == 0 {
		rawURL := strings.TrimSpace(os.Getenv(EnvHTTPAdapterURL))
		if rawURL == "" {
			return HTTPAdapterConfig{}, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: EnvHTTPAdapterURL}
		}
		urls = []string{rawURL}
	}

	timeout, err := parsePositiveDurationEnvMS(EnvHTTPAdapterTimeoutMS, defaultHTTPAdapterTimeoutMS)
	if err != nil {
		return HTTPAdapterConfig{}, err
	}
	retryMaxAttempts, err := parsePositiveIntEnv(EnvHTTPAdapterRetryMaxAttempts, defaultHTTPAdapterRetryMaxAttempts)
	if err != nil {
		return HTTPAdapterConfig{}, err
	}
	retryBackoff, err := parsePositiveDurationEnvMS(EnvHTTPAdapterRetryBackoffMS, defaultHTTPAdapterRetryBackoffMS)
	if err != nil {
		return HTTPAdapterConfig{}, err
	}
	retryMaxBackoff, err := parsePositiveDurationEnvMS(EnvHTTPAdapterRetryMaxBackoffMS, defaultHTTPAdapterRetryMaxBackoffMS)
	if err != nil {
		return HTTPAdapterConfig{}, err
	}
	cacheTTL, err := parsePositiveDurationEnvMS(EnvHTTPAdapterCacheTTLMS, defaultHTTPAdapterCacheTTLMS)
	if err != nil {
		return HTTPAdapterConfig{}, err
	}
	maxStaleness, err := parsePositiveDurationEnvMS(EnvHTTPAdapterMaxStalenessMS, defaultHTTPAdapterMaxStalenessMS)
	if err != nil {
		return HTTPAdapterConfig{}, err
	}

	return HTTPAdapterConfig{
		URL:              urls[0],
		URLs:             urls,
		Timeout:          timeout,
		AuthBearerToken:  strings.TrimSpace(os.Getenv(EnvHTTPAdapterAuthBearerToken)),
		ClientID:         strings.TrimSpace(os.Getenv(EnvHTTPAdapterClientID)),
		RetryMaxAttempts: retryMaxAttempts,
		RetryBackoff:     retryBackoff,
		RetryMaxBackoff:  retryMaxBackoff,
		CacheTTL:         cacheTTL,
		MaxStaleness:     maxStaleness,
	}, nil
}

// NewHTTPBackendsFromEnv builds CP service backends from env-configured HTTP snapshot distribution.
func NewHTTPBackendsFromEnv() (ServiceBackends, error) {
	cfg, err := HTTPAdapterConfigFromEnv()
	if err != nil {
		return ServiceBackends{}, err
	}
	return NewHTTPBackends(cfg)
}

// NewHTTPBackends builds CP service backends from HTTP snapshot distribution endpoints.
func NewHTTPBackends(cfg HTTPAdapterConfig) (ServiceBackends, error) {
	provider, err := newHTTPSnapshotProvider(cfg)
	if err != nil {
		return ServiceBackends{}, err
	}
	// Fail fast at construction: verify at least one endpoint can supply a usable snapshot.
	if _, err := provider.current(); err != nil {
		return ServiceBackends{}, err
	}

	return ServiceBackends{
		Registry:       httpRegistryBackend{provider: provider},
		Rollout:        httpRolloutBackend{provider: provider},
		RoutingView:    httpRoutingBackend{provider: provider},
		Policy:         httpPolicyBackend{provider: provider},
		ProviderHealth: httpProviderHealthBackend{provider: provider},
		GraphCompiler:  httpGraphCompilerBackend{provider: provider},
		Admission:      httpAdmissionBackend{provider: provider},
		Lease:          httpLeaseBackend{provider: provider},
	}, nil
}

type httpSnapshotProvider struct {
	mu    sync.Mutex
	cfg   HTTPAdapterConfig
	cache *httpSnapshotCache
}

type httpSnapshotCache struct {
	adapter   fileAdapter
	fetchedAt time.Time
	expiresAt time.Time
}

func newHTTPSnapshotProvider(cfg HTTPAdapterConfig) (*httpSnapshotProvider, error) {
	normalized, err := normalizeHTTPAdapterConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &httpSnapshotProvider{cfg: normalized}, nil
}

func (p *httpSnapshotProvider) current() (fileAdapter, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.cfg.Now()
	if p.cache != nil && now.Before(p.cache.expiresAt) {
		return p.cache.adapter, nil
	}

	adapter, err := p.fetchSnapshot()
	if err == nil {
		fetchedAt := p.cfg.Now()
		p.cache = &httpSnapshotCache{
			adapter:   adapter,
			fetchedAt: fetchedAt,
			expiresAt: fetchedAt.Add(p.cfg.CacheTTL),
		}
		return adapter, nil
	}

	if p.cache != nil && p.canServeStale(now) {
		return p.cache.adapter, nil
	}
	return fileAdapter{}, err
}

func (p *httpSnapshotProvider) canServeStale(now time.Time) bool {
	if p.cache == nil || p.cfg.MaxStaleness <= 0 {
		return false
	}
	if now.Before(p.cache.expiresAt) {
		return true
	}
	return now.Sub(p.cache.expiresAt) <= p.cfg.MaxStaleness
}

func (p *httpSnapshotProvider) fetchSnapshot() (fileAdapter, error) {
	endpointErrors := make([]error, 0, len(p.cfg.URLs))
	for _, endpoint := range p.cfg.URLs {
		adapter, err := p.fetchFromEndpoint(endpoint)
		if err == nil {
			return adapter, nil
		}
		endpointErrors = append(endpointErrors, err)
	}
	return fileAdapter{}, aggregateEndpointErrors(p.cfg.URLs, endpointErrors)
}

func (p *httpSnapshotProvider) fetchFromEndpoint(endpoint string) (fileAdapter, error) {
	var lastErr error
	for attempt := 1; attempt <= p.cfg.RetryMaxAttempts; attempt++ {
		adapter, err := p.fetchOnce(endpoint)
		if err == nil {
			return adapter, nil
		}
		lastErr = err
		if attempt == p.cfg.RetryMaxAttempts || !shouldRetryEndpointError(err) {
			break
		}
		p.cfg.Sleep(retryBackoffDuration(p.cfg.RetryBackoff, p.cfg.RetryMaxBackoff, attempt))
	}
	if lastErr == nil {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeReadArtifact, Path: endpoint, Cause: fmt.Errorf("fetch failed")}
	}
	return fileAdapter{}, lastErr
}

func (p *httpSnapshotProvider) fetchOnce(endpoint string) (fileAdapter, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: endpoint, Cause: err}
	}
	req.Header.Set("Accept", "application/json")
	if p.cfg.AuthBearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.AuthBearerToken)
	}
	if p.cfg.ClientID != "" {
		req.Header.Set("X-RSPP-Client-ID", p.cfg.ClientID)
	}

	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeReadArtifact, Path: endpoint, Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fileAdapter{}, BackendError{
			Service: "distribution",
			Code:    ErrorCodeReadArtifact,
			Path:    endpoint,
			Cause:   httpStatusError{StatusCode: resp.StatusCode},
		}
	}

	var artifact fileArtifact
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&artifact); err != nil {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeDecodeArtifact, Path: endpoint, Cause: err}
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeDecodeArtifact, Path: endpoint, Cause: err}
	}
	if err := artifact.validate(endpoint); err != nil {
		return fileAdapter{}, err
	}
	if artifactHasAnyStaleSection(artifact) {
		return fileAdapter{}, BackendError{
			Service: "distribution",
			Code:    ErrorCodeSnapshotStale,
			Path:    endpoint,
			Cause:   fmt.Errorf("snapshot marked stale"),
		}
	}

	return fileAdapter{path: endpoint, artifact: artifact}, nil
}

func aggregateEndpointErrors(endpoints []string, endpointErrors []error) error {
	path := strings.Join(endpoints, ",")
	if path == "" {
		path = "endpoints"
	}
	if len(endpointErrors) == 0 {
		return BackendError{
			Service: "distribution",
			Code:    ErrorCodeReadArtifact,
			Path:    path,
			Cause:   fmt.Errorf("no endpoint errors recorded"),
		}
	}

	allStale := true
	for _, err := range endpointErrors {
		var backendErr BackendError
		if !errors.As(err, &backendErr) || backendErr.Code != ErrorCodeSnapshotStale {
			allStale = false
			break
		}
	}
	if allStale {
		return BackendError{
			Service: "distribution",
			Code:    ErrorCodeSnapshotStale,
			Path:    path,
			Cause:   fmt.Errorf("all configured endpoints returned stale snapshots"),
		}
	}

	selected := endpointErrors[0]
	for _, err := range endpointErrors {
		var backendErr BackendError
		if errors.As(err, &backendErr) && backendErr.Code == ErrorCodeSnapshotStale {
			continue
		}
		selected = err
		break
	}

	code := ErrorCodeReadArtifact
	var backendErr BackendError
	if errors.As(selected, &backendErr) {
		code = backendErr.Code
	}
	return BackendError{
		Service: "distribution",
		Code:    code,
		Path:    path,
		Cause:   fmt.Errorf("all configured endpoints failed: %w", selected),
	}
}

func shouldRetryEndpointError(err error) bool {
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		return true
	}
	if backendErr.Code != ErrorCodeReadArtifact {
		return false
	}
	var statusErr httpStatusError
	if errors.As(backendErr.Cause, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= http.StatusInternalServerError
	}
	return true
}

func retryBackoffDuration(base time.Duration, max time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = time.Duration(defaultHTTPAdapterRetryBackoffMS) * time.Millisecond
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

func normalizeHTTPAdapterConfig(cfg HTTPAdapterConfig) (HTTPAdapterConfig, error) {
	urls := append([]string(nil), cfg.URLs...)
	if len(urls) == 0 {
		if trimmedURL := strings.TrimSpace(cfg.URL); trimmedURL != "" {
			urls = []string{trimmedURL}
		}
	}
	if len(urls) == 0 {
		return HTTPAdapterConfig{}, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: "urls", Cause: fmt.Errorf("at least one url is required")}
	}

	normalizedURLs := make([]string, 0, len(urls))
	for _, raw := range urls {
		normalizedURL, err := normalizeHTTPAdapterURL(raw)
		if err != nil {
			return HTTPAdapterConfig{}, err
		}
		normalizedURLs = append(normalizedURLs, normalizedURL)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = time.Duration(defaultHTTPAdapterTimeoutMS) * time.Millisecond
	}

	retryMaxAttempts := cfg.RetryMaxAttempts
	if retryMaxAttempts < 1 {
		retryMaxAttempts = int(defaultHTTPAdapterRetryMaxAttempts)
	}
	retryBackoff := cfg.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = time.Duration(defaultHTTPAdapterRetryBackoffMS) * time.Millisecond
	}
	retryMaxBackoff := cfg.RetryMaxBackoff
	if retryMaxBackoff <= 0 {
		retryMaxBackoff = time.Duration(defaultHTTPAdapterRetryMaxBackoffMS) * time.Millisecond
	}
	if retryMaxBackoff < retryBackoff {
		retryMaxBackoff = retryBackoff
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = time.Duration(defaultHTTPAdapterCacheTTLMS) * time.Millisecond
	}
	maxStaleness := cfg.MaxStaleness
	if maxStaleness <= 0 {
		maxStaleness = time.Duration(defaultHTTPAdapterMaxStalenessMS) * time.Millisecond
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	sleepFn := cfg.Sleep
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	return HTTPAdapterConfig{
		URL:              normalizedURLs[0],
		URLs:             normalizedURLs,
		Timeout:          timeout,
		Client:           client,
		AuthBearerToken:  strings.TrimSpace(cfg.AuthBearerToken),
		ClientID:         strings.TrimSpace(cfg.ClientID),
		RetryMaxAttempts: retryMaxAttempts,
		RetryBackoff:     retryBackoff,
		RetryMaxBackoff:  retryMaxBackoff,
		CacheTTL:         cacheTTL,
		MaxStaleness:     maxStaleness,
		Now:              nowFn,
		Sleep:            sleepFn,
	}, nil
}

func normalizeHTTPAdapterURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: "url", Cause: fmt.Errorf("url is required")}
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: trimmed, Cause: err}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", BackendError{
			Service: "distribution",
			Code:    ErrorCodeInvalidConfig,
			Path:    trimmed,
			Cause:   fmt.Errorf("url scheme must be http or https"),
		}
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: trimmed, Cause: fmt.Errorf("url host is required")}
	}
	return trimmed, nil
}

func parseOrderedEndpoints(raw string) []string {
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

func parsePositiveDurationEnvMS(name string, fallbackMS int64) (time.Duration, error) {
	fallback := time.Duration(fallbackMS) * time.Millisecond
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: name, Cause: err}
	}
	if parsed < 1 {
		return 0, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: name, Cause: fmt.Errorf("value must be >=1ms")}
	}
	return time.Duration(parsed) * time.Millisecond, nil
}

func parsePositiveIntEnv(name string, fallback int64) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return int(fallback), nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: name, Cause: err}
	}
	if parsed < 1 {
		return 0, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: name, Cause: fmt.Errorf("value must be >=1")}
	}
	return int(parsed), nil
}

func artifactHasAnyStaleSection(artifact fileArtifact) bool {
	return artifact.Stale ||
		artifact.Registry.Stale ||
		artifact.Rollout.Stale ||
		artifact.RoutingView.Stale ||
		artifact.Policy.Stale ||
		artifact.ProviderHealth.Stale ||
		artifact.GraphCompiler.Stale ||
		artifact.Admission.Stale ||
		artifact.Lease.Stale
}

type httpStatusError struct {
	StatusCode int
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("unexpected status_code=%d", e.StatusCode)
}

func ensureSingleJSONObject(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected trailing JSON payload")
}

type httpRegistryBackend struct {
	provider *httpSnapshotProvider
}

func (b httpRegistryBackend) ResolvePipelineRecord(pipelineVersion string) (registry.PipelineRecord, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return registry.PipelineRecord{}, err
	}
	return fileRegistryBackend{adapter: adapter}.ResolvePipelineRecord(pipelineVersion)
}

type httpRolloutBackend struct {
	provider *httpSnapshotProvider
}

func (b httpRolloutBackend) ResolvePipelineVersion(in rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return rollout.ResolveVersionOutput{}, err
	}
	return fileRolloutBackend{adapter: adapter}.ResolvePipelineVersion(in)
}

type httpRoutingBackend struct {
	provider *httpSnapshotProvider
}

func (b httpRoutingBackend) GetSnapshot(in routingview.Input) (routingview.Snapshot, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return routingview.Snapshot{}, err
	}
	return fileRoutingBackend{adapter: adapter}.GetSnapshot(in)
}

type httpPolicyBackend struct {
	provider *httpSnapshotProvider
}

func (b httpPolicyBackend) Evaluate(in policy.Input) (policy.Output, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return policy.Output{}, err
	}
	return filePolicyBackend{adapter: adapter}.Evaluate(in)
}

type httpProviderHealthBackend struct {
	provider *httpSnapshotProvider
}

func (b httpProviderHealthBackend) GetSnapshot(in providerhealth.Input) (providerhealth.Output, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return providerhealth.Output{}, err
	}
	return fileProviderHealthBackend{adapter: adapter}.GetSnapshot(in)
}

type httpGraphCompilerBackend struct {
	provider *httpSnapshotProvider
}

func (b httpGraphCompilerBackend) Compile(in graphcompiler.Input) (graphcompiler.Output, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return graphcompiler.Output{}, err
	}
	return fileGraphCompilerBackend{adapter: adapter}.Compile(in)
}

type httpAdmissionBackend struct {
	provider *httpSnapshotProvider
}

func (b httpAdmissionBackend) Evaluate(in admission.Input) (admission.Output, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return admission.Output{}, err
	}
	return fileAdmissionBackend{adapter: adapter}.Evaluate(in)
}

type httpLeaseBackend struct {
	provider *httpSnapshotProvider
}

func (b httpLeaseBackend) Resolve(in lease.Input) (lease.Output, error) {
	adapter, err := b.provider.current()
	if err != nil {
		return lease.Output{}, err
	}
	return fileLeaseBackend{adapter: adapter}.Resolve(in)
}
