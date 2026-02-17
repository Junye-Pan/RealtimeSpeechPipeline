package distribution

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

func TestHTTPAdapterConfigFromEnvRequiresEndpoint(t *testing.T) {
	t.Setenv(EnvHTTPAdapterURL, "")
	t.Setenv(EnvHTTPAdapterURLs, "")

	_, err := HTTPAdapterConfigFromEnv()
	if err == nil {
		t.Fatalf("expected missing endpoint error")
	}

	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeInvalidConfig {
		t.Fatalf("expected invalid_config error code, got %s", backendErr.Code)
	}
}

func TestHTTPAdapterConfigFromEnvParsesURLsAndSecurityFields(t *testing.T) {
	t.Setenv(EnvHTTPAdapterURLs, " https://cp-a.local/snapshots,https://cp-b.local/snapshots ")
	t.Setenv(EnvHTTPAdapterURL, "https://ignored-single.local/snapshot")
	t.Setenv(EnvHTTPAdapterAuthBearerToken, "token-abc")
	t.Setenv(EnvHTTPAdapterClientID, "runtime-node-1")
	t.Setenv(EnvHTTPAdapterRetryMaxAttempts, "3")
	t.Setenv(EnvHTTPAdapterRetryBackoffMS, "120")
	t.Setenv(EnvHTTPAdapterRetryMaxBackoffMS, "800")
	t.Setenv(EnvHTTPAdapterCacheTTLMS, "2500")
	t.Setenv(EnvHTTPAdapterMaxStalenessMS, "7000")

	cfg, err := HTTPAdapterConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected config parse error: %v", err)
	}

	if !reflect.DeepEqual(cfg.URLs, []string{"https://cp-a.local/snapshots", "https://cp-b.local/snapshots"}) {
		t.Fatalf("unexpected urls: %+v", cfg.URLs)
	}
	if cfg.URL != "https://cp-a.local/snapshots" {
		t.Fatalf("unexpected primary url: %q", cfg.URL)
	}
	if cfg.AuthBearerToken != "token-abc" || cfg.ClientID != "runtime-node-1" {
		t.Fatalf("unexpected auth/client config: %+v", cfg)
	}
	if cfg.RetryMaxAttempts != 3 {
		t.Fatalf("unexpected retry attempts: %d", cfg.RetryMaxAttempts)
	}
	if cfg.RetryBackoff != 120*time.Millisecond || cfg.RetryMaxBackoff != 800*time.Millisecond {
		t.Fatalf("unexpected retry backoff settings: %+v", cfg)
	}
	if cfg.CacheTTL != 2500*time.Millisecond || cfg.MaxStaleness != 7000*time.Millisecond {
		t.Fatalf("unexpected cache settings: %+v", cfg)
	}
}

func TestHTTPAdapterConfigFromEnvInvalidTimeout(t *testing.T) {
	t.Setenv(EnvHTTPAdapterURL, "http://127.0.0.1/snapshots")
	t.Setenv(EnvHTTPAdapterTimeoutMS, "abc")

	_, err := HTTPAdapterConfigFromEnv()
	if err == nil {
		t.Fatalf("expected invalid timeout parse failure")
	}

	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeInvalidConfig || backendErr.Path != EnvHTTPAdapterTimeoutMS {
		t.Fatalf("expected invalid timeout config error, got %+v", backendErr)
	}
}

func TestNewHTTPBackendsResolvesAllServicesWithAuthHeaders(t *testing.T) {
	t.Parallel()

	var (
		mu                 sync.Mutex
		authorizationSeen  string
		clientIdentitySeen string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorizationSeen = r.Header.Get("Authorization")
		clientIdentitySeen = r.Header.Get("X-RSPP-Client-ID")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version": "cp-snapshot-distribution/v1",
  "registry": {
    "default_pipeline_version": "pipeline-http",
    "records": {
      "pipeline-http": {
        "pipeline_version": "pipeline-http",
        "graph_definition_ref": "graph/http",
        "execution_profile": "simple"
      }
    }
  },
  "rollout": {
    "version_resolution_snapshot": "version-resolution/http",
    "default_pipeline_version": "pipeline-http",
    "by_requested_version": {"pipeline-requested": "pipeline-http"}
  },
  "routing_view": {
    "default": {
      "routing_view_snapshot": "routing-view/http",
      "admission_policy_snapshot": "admission-policy/http",
      "abi_compatibility_snapshot": "abi-compat/http",
      "transport_kind": "livekit",
      "transport_endpoint": "wss://runtime-http.rspp.local/livekit/pipeline-http",
      "runtime_id": "runtime-http-a"
    }
  },
	  "policy": {
	    "default": {
	      "policy_resolution_snapshot": "policy-resolution/http",
	      "allowed_adaptive_actions": ["fallback", "retry"],
	      "budgets": {
	        "turn_budget_ms": 6400,
	        "node_budget_ms_default": 1300,
	        "path_budget_ms_default": 3900,
	        "edge_budget_ms_default": 650
	      },
	      "provider_bindings": {
	        "stt": "stt-http",
	        "llm": "llm-http",
	        "tts": "tts-http"
	      },
	      "edge_buffer_policies": {
	        "default": {
	          "strategy": "drop",
	          "max_queue_items": 36,
	          "max_queue_ms": 210,
	          "max_queue_bytes": 170000,
	          "max_latency_contribution_ms": 85,
	          "watermarks": {
	            "queue_items": {"high": 22, "low": 8}
	          },
	          "lane_handling": {
	            "DataLane": "drop",
	            "ControlLane": "non_blocking_priority",
	            "TelemetryLane": "best_effort_drop"
	          },
	          "defaulting_source": "explicit_edge_config"
	        }
	      },
	      "node_execution_policies": {
	        "provider-heavy": {
	          "concurrency_limit": 1,
	          "fairness_key": "provider-heavy"
	        }
	      },
	      "flow_control": {
	        "mode_by_lane": {
	          "DataLane": "signal",
	          "ControlLane": "signal",
	          "TelemetryLane": "signal"
	        },
	        "watermarks": {
	          "DataLane": {"high": 68, "low": 18},
	          "ControlLane": {"high": 18, "low": 8},
	          "TelemetryLane": {"high": 96, "low": 36}
	        },
	        "shedding_strategy_by_lane": {
	          "DataLane": "drop",
	          "ControlLane": "none",
	          "TelemetryLane": "sample"
	        }
	      },
	      "recording_policy": {
	        "recording_level": "L0",
	        "allowed_replay_modes": ["replay_decisions"]
	      }
	    }
	  },
  "provider_health": {"default": {"provider_health_snapshot": "provider-health/http"}},
  "graph_compiler": {"default": {"graph_compilation_snapshot": "graph-compiler/http", "graph_fingerprint": "graph-fingerprint/http"}},
  "admission": {"default": {"admission_policy_snapshot": "admission-policy/http", "outcome_kind": "admit", "scope": "session", "reason": "cp_admission_allowed", "session_rate_limit_per_min": 100, "session_rate_observed_per_min": 20}},
  "lease": {"default": {"lease_resolution_snapshot": "lease-resolution/http", "authority_epoch": 7, "authority_epoch_valid": true, "authority_authorized": true, "reason": "lease_authorized", "lease_token_id": "lease-token-http", "lease_expires_at_utc": "2026-02-17T00:50:00Z"}}
}`))
	}))
	defer server.Close()

	backends, err := NewHTTPBackends(HTTPAdapterConfig{
		URLs:            []string{server.URL},
		AuthBearerToken: "token-123",
		ClientID:        "runtime-node-7",
	})
	if err != nil {
		t.Fatalf("expected http-backed backends, got %v", err)
	}

	record, err := backends.Registry.ResolvePipelineRecord("pipeline-http")
	if err != nil {
		t.Fatalf("registry resolve: %v", err)
	}
	if record.GraphDefinitionRef != "graph/http" || record.ExecutionProfile != "simple" {
		t.Fatalf("unexpected registry record: %+v", record)
	}

	versionOut, err := backends.Rollout.ResolvePipelineVersion(rollout.ResolveVersionInput{
		SessionID:                "sess-http-1",
		RequestedPipelineVersion: "pipeline-requested",
	})
	if err != nil {
		t.Fatalf("rollout resolve: %v", err)
	}
	if versionOut.PipelineVersion != "pipeline-http" || versionOut.VersionResolutionSnapshot != "version-resolution/http" {
		t.Fatalf("unexpected rollout output: %+v", versionOut)
	}

	routingOut, err := backends.RoutingView.GetSnapshot(routingview.Input{
		SessionID:       "sess-http-1",
		PipelineVersion: "pipeline-http",
		AuthorityEpoch:  1,
	})
	if err != nil {
		t.Fatalf("routing resolve: %v", err)
	}
	if routingOut.RoutingViewSnapshot != "routing-view/http" ||
		routingOut.AdmissionPolicySnapshot != "admission-policy/http" ||
		routingOut.ABICompatibilitySnapshot != "abi-compat/http" {
		t.Fatalf("unexpected routing output: %+v", routingOut)
	}
	if routingOut.TransportKind != "livekit" || routingOut.TransportEndpoint == "" || routingOut.RuntimeID != "runtime-http-a" {
		t.Fatalf("expected routing transport metadata, got %+v", routingOut)
	}

	policyOut, err := backends.Policy.Evaluate(policy.Input{SessionID: "sess-http-1", PipelineVersion: "pipeline-http"})
	if err != nil {
		t.Fatalf("policy resolve: %v", err)
	}
	if policyOut.PolicyResolutionSnapshot != "policy-resolution/http" || len(policyOut.AllowedAdaptiveActions) != 2 {
		t.Fatalf("unexpected policy output: %+v", policyOut)
	}
	if policyOut.ResolvedPolicy.Budgets.TurnBudgetMS != 6400 {
		t.Fatalf("expected policy budgets from HTTP snapshot, got %+v", policyOut.ResolvedPolicy.Budgets)
	}
	if policyOut.ResolvedPolicy.ProviderBindings["llm"] != "llm-http" {
		t.Fatalf("expected policy provider_bindings from HTTP snapshot, got %+v", policyOut.ResolvedPolicy.ProviderBindings)
	}
	if edge, ok := policyOut.ResolvedPolicy.EdgeBufferPolicies["default"]; !ok || edge.MaxQueueItems != 36 {
		t.Fatalf("expected policy edge_buffer_policies from HTTP snapshot, got %+v", policyOut.ResolvedPolicy.EdgeBufferPolicies)
	}
	if nodePolicy, ok := policyOut.ResolvedPolicy.NodeExecutionPolicies["provider-heavy"]; !ok || nodePolicy.ConcurrencyLimit != 1 || nodePolicy.FairnessKey != "provider-heavy" {
		t.Fatalf("expected policy node_execution_policies from HTTP snapshot, got %+v", policyOut.ResolvedPolicy.NodeExecutionPolicies)
	}
	if err := policyOut.ResolvedPolicy.FlowControl.Validate(); err != nil {
		t.Fatalf("expected flow_control from HTTP snapshot to validate, got %v", err)
	}
	if err := policyOut.ResolvedPolicy.RecordingPolicy.Validate(); err != nil {
		t.Fatalf("expected recording_policy from HTTP snapshot to validate, got %v", err)
	}

	providerHealthOut, err := backends.ProviderHealth.GetSnapshot(providerhealth.Input{Scope: "sess-http-1", PipelineVersion: "pipeline-http"})
	if err != nil {
		t.Fatalf("provider health resolve: %v", err)
	}
	if providerHealthOut.ProviderHealthSnapshot != "provider-health/http" {
		t.Fatalf("unexpected provider health output: %+v", providerHealthOut)
	}

	graphOut, err := backends.GraphCompiler.Compile(graphcompiler.Input{
		PipelineVersion:    "pipeline-http",
		GraphDefinitionRef: "graph/http",
		ExecutionProfile:   "simple",
	})
	if err != nil {
		t.Fatalf("graph compiler resolve: %v", err)
	}
	if graphOut.GraphCompilationSnapshot != "graph-compiler/http" || graphOut.GraphFingerprint != "graph-fingerprint/http" {
		t.Fatalf("unexpected graph compiler output: %+v", graphOut)
	}

	admissionOut, err := backends.Admission.Evaluate(admission.Input{
		SessionID:       "sess-http-1",
		TurnID:          "turn-http-1",
		PipelineVersion: "pipeline-http",
	})
	if err != nil {
		t.Fatalf("admission resolve: %v", err)
	}
	if admissionOut.AdmissionPolicySnapshot != "admission-policy/http" || admissionOut.OutcomeKind != controlplane.OutcomeAdmit {
		t.Fatalf("unexpected admission output: %+v", admissionOut)
	}
	if admissionOut.SessionRateLimitPerMin != 100 || admissionOut.SessionRateObservedPM != 20 {
		t.Fatalf("expected admission quota fields from HTTP snapshot, got %+v", admissionOut)
	}

	leaseOut, err := backends.Lease.Resolve(lease.Input{
		SessionID:               "sess-http-1",
		PipelineVersion:         "pipeline-http",
		RequestedAuthorityEpoch: 3,
	})
	if err != nil {
		t.Fatalf("lease resolve: %v", err)
	}
	if leaseOut.LeaseResolutionSnapshot != "lease-resolution/http" || leaseOut.AuthorityEpoch != 7 {
		t.Fatalf("unexpected lease output: %+v", leaseOut)
	}
	if leaseOut.AuthorityEpochValid == nil || !*leaseOut.AuthorityEpochValid || leaseOut.AuthorityAuthorized == nil || !*leaseOut.AuthorityAuthorized {
		t.Fatalf("expected lease authority booleans to be true, got %+v", leaseOut)
	}
	if leaseOut.LeaseTokenID != "lease-token-http" || leaseOut.LeaseExpiresAtUTC != "2026-02-17T00:50:00Z" {
		t.Fatalf("expected lease token metadata from HTTP snapshot, got %+v", leaseOut)
	}

	mu.Lock()
	defer mu.Unlock()
	if authorizationSeen != "Bearer token-123" {
		t.Fatalf("expected bearer authorization header, got %q", authorizationSeen)
	}
	if clientIdentitySeen != "runtime-node-7" {
		t.Fatalf("expected client identity header, got %q", clientIdentitySeen)
	}
}

func TestNewHTTPBackendsAdmissionTenantOverridePrecedence(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version": "cp-snapshot-distribution/v1",
  "admission": {
    "default": {
      "admission_policy_snapshot": "admission-policy/default",
      "outcome_kind": "admit",
      "scope": "session",
      "reason": "cp_admission_allowed"
    },
    "by_pipeline": {
      "pipeline-v1": {
        "admission_policy_snapshot": "admission-policy/pipeline-v1",
        "outcome_kind": "reject",
        "scope": "session",
        "reason": "cp_admission_reject_policy"
      }
    },
    "by_tenant": {
      "tenant-gold": {
        "admission_policy_snapshot": "admission-policy/tenant-gold",
        "outcome_kind": "defer",
        "scope": "tenant",
        "reason": "cp_admission_defer_capacity",
        "session_rate_limit_per_min": 2,
        "session_rate_observed_per_min": 3
      }
    }
  }
}`))
	}))
	defer server.Close()

	backends, err := NewHTTPBackends(HTTPAdapterConfig{
		URLs: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("expected http-backed backends, got %v", err)
	}

	byPipeline, err := backends.Admission.Evaluate(admission.Input{
		SessionID:       "sess-http-admission-pipeline-1",
		TurnID:          "turn-http-admission-pipeline-1",
		PipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("pipeline admission evaluate: %v", err)
	}
	if byPipeline.AdmissionPolicySnapshot != "admission-policy/pipeline-v1" ||
		byPipeline.OutcomeKind != controlplane.OutcomeReject ||
		byPipeline.Scope != controlplane.ScopeSession ||
		byPipeline.Reason != "cp_admission_reject_policy" {
		t.Fatalf("unexpected pipeline admission output: %+v", byPipeline)
	}

	byTenant, err := backends.Admission.Evaluate(admission.Input{
		TenantID:        "tenant-gold",
		SessionID:       "sess-http-admission-tenant-1",
		TurnID:          "turn-http-admission-tenant-1",
		PipelineVersion: "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("tenant admission evaluate: %v", err)
	}
	if byTenant.AdmissionPolicySnapshot != "admission-policy/tenant-gold" ||
		byTenant.OutcomeKind != controlplane.OutcomeDefer ||
		byTenant.Scope != controlplane.ScopeTenant ||
		byTenant.Reason != "cp_admission_defer_capacity" {
		t.Fatalf("unexpected tenant admission output: %+v", byTenant)
	}
	if byTenant.SessionRateLimitPerMin != 2 || byTenant.SessionRateObservedPM != 3 {
		t.Fatalf("expected tenant quota fields in admission output, got %+v", byTenant)
	}
}

func TestNewHTTPBackendsFailsOverAcrossOrderedEndpoints(t *testing.T) {
	t.Parallel()

	firstCalls := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstCalls++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer firstServer.Close()

	secondCalls := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "registry":{"records":{"pipeline-v1":{"graph_definition_ref":"graph/failover","execution_profile":"simple"}}}
}`))
	}))
	defer secondServer.Close()

	backends, err := NewHTTPBackends(HTTPAdapterConfig{
		URLs:             []string{firstServer.URL, secondServer.URL},
		RetryMaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("expected failover to succeed, got %v", err)
	}

	record, err := backends.Registry.ResolvePipelineRecord("pipeline-v1")
	if err != nil {
		t.Fatalf("registry resolve after failover: %v", err)
	}
	if record.GraphDefinitionRef != "graph/failover" {
		t.Fatalf("expected data from second endpoint, got %+v", record)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("expected ordered failover first->second once each, got first=%d second=%d", firstCalls, secondCalls)
	}
}

func TestNewHTTPBackendsRetriesWithBackoff(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"transient"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "registry":{"records":{"pipeline-v1":{"graph_definition_ref":"graph/retry","execution_profile":"simple"}}}
}`))
	}))
	defer server.Close()

	sleeps := make([]time.Duration, 0, 4)
	backends, err := NewHTTPBackends(HTTPAdapterConfig{
		URLs:             []string{server.URL},
		RetryMaxAttempts: 3,
		RetryBackoff:     10 * time.Millisecond,
		RetryMaxBackoff:  100 * time.Millisecond,
		Sleep: func(d time.Duration) {
			sleeps = append(sleeps, d)
		},
	})
	if err != nil {
		t.Fatalf("expected retry to recover endpoint, got %v", err)
	}

	record, err := backends.Registry.ResolvePipelineRecord("pipeline-v1")
	if err != nil {
		t.Fatalf("registry resolve after retries: %v", err)
	}
	if record.GraphDefinitionRef != "graph/retry" {
		t.Fatalf("unexpected record after retries: %+v", record)
	}
	if attempts != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", attempts)
	}
	if !reflect.DeepEqual(sleeps, []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}) {
		t.Fatalf("unexpected retry sleeps: %+v", sleeps)
	}
}

func TestNewHTTPBackendsRefreshFailureServesStaleWithinBound(t *testing.T) {
	t.Parallel()

	clock := newFakeClock(time.Unix(0, 0).UTC())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "registry":{"records":{"pipeline-v1":{"graph_definition_ref":"graph/cache-v1","execution_profile":"simple"}}}
}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"refresh failed"}`))
	}))
	defer server.Close()

	backends, err := NewHTTPBackends(HTTPAdapterConfig{
		URLs:             []string{server.URL},
		RetryMaxAttempts: 1,
		CacheTTL:         time.Second,
		MaxStaleness:     5 * time.Second,
		Now:              clock.Now,
		Sleep:            func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	clock.Advance(2 * time.Second)
	record, err := backends.Registry.ResolvePipelineRecord("pipeline-v1")
	if err != nil {
		t.Fatalf("expected stale serve fallback, got %v", err)
	}
	if record.GraphDefinitionRef != "graph/cache-v1" {
		t.Fatalf("expected cached snapshot record, got %+v", record)
	}
	if requests != 2 {
		t.Fatalf("expected one initial fetch and one failed refresh, got %d", requests)
	}
}

func TestNewHTTPBackendsRefreshFailureBeyondBoundFails(t *testing.T) {
	t.Parallel()

	clock := newFakeClock(time.Unix(0, 0).UTC())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "registry":{"records":{"pipeline-v1":{"graph_definition_ref":"graph/cache-v1","execution_profile":"simple"}}}
}`))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"refresh failed"}`))
	}))
	defer server.Close()

	backends, err := NewHTTPBackends(HTTPAdapterConfig{
		URLs:             []string{server.URL},
		RetryMaxAttempts: 1,
		CacheTTL:         time.Second,
		MaxStaleness:     3 * time.Second,
		Now:              clock.Now,
		Sleep:            func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}

	clock.Advance(10 * time.Second)
	_, err = backends.Registry.ResolvePipelineRecord("pipeline-v1")
	if err == nil {
		t.Fatalf("expected refresh failure beyond staleness window")
	}
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeReadArtifact {
		t.Fatalf("expected artifact_read_failed code, got %s", backendErr.Code)
	}
	if requests != 2 {
		t.Fatalf("expected one initial fetch and one refresh attempt, got %d", requests)
	}
}

func TestNewHTTPBackendsAllEndpointsStaleReturnsStaleError(t *testing.T) {
	t.Parallel()

	stalePayload := `{"schema_version":"cp-snapshot-distribution/v1","stale":true}`
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stalePayload))
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(stalePayload))
	}))
	defer serverB.Close()

	_, err := NewHTTPBackends(HTTPAdapterConfig{
		URLs:             []string{serverA.URL, serverB.URL},
		RetryMaxAttempts: 1,
	})
	if err == nil {
		t.Fatalf("expected stale endpoint chain failure")
	}
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeSnapshotStale {
		t.Fatalf("expected snapshot_stale code, got %s", backendErr.Code)
	}
}

func TestNewHTTPBackendsFromEnvUsesURLsPrecedence(t *testing.T) {
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "registry":{"records":{"pipeline-v1":{"graph_definition_ref":"graph/url-list","execution_profile":"simple"}}}
}`))
	}))
	defer serverB.Close()

	t.Setenv(EnvHTTPAdapterURLs, serverA.URL+","+serverB.URL)
	t.Setenv(EnvHTTPAdapterURL, "https://single-url-should-be-ignored.local")
	t.Setenv(EnvHTTPAdapterRetryMaxAttempts, "1")

	backends, err := NewHTTPBackendsFromEnv()
	if err != nil {
		t.Fatalf("expected env-backed failover URLs to load, got %v", err)
	}

	record, err := backends.Registry.ResolvePipelineRecord("pipeline-v1")
	if err != nil {
		t.Fatalf("registry resolve: %v", err)
	}
	if record.GraphDefinitionRef != "graph/url-list" {
		t.Fatalf("expected endpoint list to drive resolution, got %+v", record)
	}
}

type fakeClock struct {
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}
