package replay

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

func TestHTTPAuditBackendConfigFromEnvParsesFields(t *testing.T) {
	t.Setenv(EnvReplayAuditHTTPURLs, " https://audit-a.local/v1/audit,https://audit-b.local/v1/audit ")
	t.Setenv(EnvReplayAuditHTTPAuthBearerToken, "token-abc")
	t.Setenv(EnvReplayAuditHTTPClientID, "runtime-1")
	t.Setenv(EnvReplayAuditHTTPRetryMaxAttempts, "3")
	t.Setenv(EnvReplayAuditHTTPRetryBackoffMS, "150")
	t.Setenv(EnvReplayAuditHTTPRetryMaxBackoffMS, "800")
	t.Setenv(EnvReplayAuditHTTPTimeoutMS, "2500")

	cfg, err := HTTPAuditBackendConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected config parse error: %v", err)
	}
	if !reflect.DeepEqual(cfg.URLs, []string{"https://audit-a.local/v1/audit", "https://audit-b.local/v1/audit"}) {
		t.Fatalf("unexpected urls: %+v", cfg.URLs)
	}
	if cfg.URL != "https://audit-a.local/v1/audit" {
		t.Fatalf("unexpected primary url: %q", cfg.URL)
	}
	if cfg.AuthBearerToken != "token-abc" || cfg.ClientID != "runtime-1" {
		t.Fatalf("unexpected auth/client fields: %+v", cfg)
	}
	if cfg.RetryMaxAttempts != 3 {
		t.Fatalf("unexpected retry attempts: %d", cfg.RetryMaxAttempts)
	}
	if cfg.RetryBackoff != 150*time.Millisecond || cfg.RetryMaxBackoff != 800*time.Millisecond || cfg.Timeout != 2500*time.Millisecond {
		t.Fatalf("unexpected timeout/backoff fields: %+v", cfg)
	}
}

func TestHTTPAuditBackendAppendReplayAuditEventPropagatesHeaders(t *testing.T) {
	t.Parallel()

	var (
		mu            sync.Mutex
		authHeader    string
		clientID      string
		tenantHeader  string
		receivedEvent obs.ReplayAuditEvent
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		mu.Lock()
		authHeader = r.Header.Get("Authorization")
		clientID = r.Header.Get("X-RSPP-Client-ID")
		tenantHeader = r.Header.Get("X-RSPP-Tenant-ID")
		mu.Unlock()
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&receivedEvent); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	backend := &HTTPAuditBackend{
		Config: HTTPAuditBackendConfig{
			URLs:             []string{server.URL},
			AuthBearerToken:  "token-123",
			ClientID:         "runtime-node-4",
			RetryMaxAttempts: 1,
		},
	}

	event := replayAuditEventFixture("tenant-a", 123)
	if err := backend.AppendReplayAuditEvent(event); err != nil {
		t.Fatalf("unexpected append error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if authHeader != "Bearer token-123" {
		t.Fatalf("expected bearer header, got %q", authHeader)
	}
	if clientID != "runtime-node-4" {
		t.Fatalf("expected client id header, got %q", clientID)
	}
	if tenantHeader != "tenant-a" {
		t.Fatalf("expected tenant header, got %q", tenantHeader)
	}
	if receivedEvent.TimestampMS != event.TimestampMS || receivedEvent.Request.TenantID != event.Request.TenantID {
		t.Fatalf("unexpected encoded replay audit event: %+v", receivedEvent)
	}
}

func TestHTTPAuditBackendAppendRetriesWithBackoff(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"transient"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var sleeps []time.Duration
	backend := &HTTPAuditBackend{
		Config: HTTPAuditBackendConfig{
			URLs:             []string{server.URL},
			RetryMaxAttempts: 3,
			RetryBackoff:     10 * time.Millisecond,
			RetryMaxBackoff:  100 * time.Millisecond,
			Sleep: func(d time.Duration) {
				sleeps = append(sleeps, d)
			},
		},
	}

	if err := backend.AppendReplayAuditEvent(replayAuditEventFixture("tenant-a", 1)); err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if !reflect.DeepEqual(sleeps, []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}) {
		t.Fatalf("unexpected retry backoff sequence: %+v", sleeps)
	}
}

func TestHTTPAuditBackendAppendFailsOverAcrossOrderedEndpoints(t *testing.T) {
	t.Parallel()

	firstCalls := 0
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer first.Close()

	secondCalls := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer second.Close()

	backend := &HTTPAuditBackend{
		Config: HTTPAuditBackendConfig{
			URLs:             []string{first.URL, second.URL},
			RetryMaxAttempts: 1,
		},
	}

	if err := backend.AppendReplayAuditEvent(replayAuditEventFixture("tenant-a", 1)); err != nil {
		t.Fatalf("expected failover append success, got %v", err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("expected first+second endpoint calls once each, got first=%d second=%d", firstCalls, secondCalls)
	}
}

func TestFallbackAuditBackendResolverFallsBackToJSONL(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "audit")
	resolver := FallbackAuditBackendResolver{
		Primary: stubReplayAuditBackendResolver{
			resolveFn: func(string) (ImmutableReplayAuditSink, error) {
				return errorAuditSink{err: errors.New("primary unavailable")}, nil
			},
		},
		Fallback: JSONLFileAuditBackendResolver{RootDir: root},
	}

	sink, err := resolver.ResolveReplayAuditBackend("tenant-a")
	if err != nil {
		t.Fatalf("unexpected fallback resolver error: %v", err)
	}
	event := replayAuditEventFixture("tenant-a", 77)
	if err := sink.AppendReplayAuditEvent(event); err != nil {
		t.Fatalf("expected fallback append success, got %v", err)
	}

	path, err := (JSONLFileAuditBackendResolver{RootDir: root}).TenantLogPath("tenant-a")
	if err != nil {
		t.Fatalf("unexpected tenant path error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fallback jsonl path: %v", err)
	}
	if !strings.Contains(string(raw), "\"timestamp_ms\":77") {
		t.Fatalf("expected fallback jsonl entry, got %q", string(raw))
	}
}

func TestFallbackAuditBackendResolverFailsWhenPrimaryAndFallbackFail(t *testing.T) {
	t.Parallel()

	resolver := FallbackAuditBackendResolver{
		Primary: stubReplayAuditBackendResolver{
			resolveFn: func(string) (ImmutableReplayAuditSink, error) {
				return errorAuditSink{err: errors.New("primary append failed")}, nil
			},
		},
		Fallback: stubReplayAuditBackendResolver{
			resolveFn: func(string) (ImmutableReplayAuditSink, error) {
				return errorAuditSink{err: errors.New("fallback append failed")}, nil
			},
		},
	}

	sink, err := resolver.ResolveReplayAuditBackend("tenant-a")
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if err := sink.AppendReplayAuditEvent(replayAuditEventFixture("tenant-a", 1)); err == nil {
		t.Fatalf("expected append failure when both primary and fallback sinks fail")
	}
}

func TestNewReplayAuditBackendResolverFromEnvWithoutHTTPUsesJSONLFallback(t *testing.T) {
	t.Setenv(EnvReplayAuditHTTPURLs, "")
	t.Setenv(EnvReplayAuditHTTPURL, "")
	root := filepath.Join(t.TempDir(), "audit")
	t.Setenv(EnvReplayAuditJSONLFallbackRootDir, root)

	resolver, err := NewReplayAuditBackendResolverFromEnv()
	if err != nil {
		t.Fatalf("unexpected resolver from env error: %v", err)
	}

	sink, err := resolver.ResolveReplayAuditBackend("tenant-a")
	if err != nil {
		t.Fatalf("unexpected sink resolution error: %v", err)
	}
	if err := sink.AppendReplayAuditEvent(replayAuditEventFixture("tenant-a", 9)); err != nil {
		t.Fatalf("unexpected jsonl fallback append error: %v", err)
	}
}

type errorAuditSink struct {
	err error
}

func (s errorAuditSink) AppendReplayAuditEvent(obs.ReplayAuditEvent) error {
	return s.err
}
