package distribution

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestLoadRetentionPolicySnapshotFromFile(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "retention": {
    "default_policy": {
      "tenant_id": "retention-policy-default",
      "default_retention_ms": 1000,
      "pii_retention_limit_ms": 1000,
      "phi_retention_limit_ms": 500,
      "max_retention_by_class_ms": {
        "audio_raw": 1000,
        "text_raw": 1000,
        "PII": 500,
        "PHI": 250,
        "derived_summary": 1000,
        "metadata": 1000
      }
    },
    "tenant_policies": {
      "tenant-a": {
        "tenant_id": "tenant-a",
        "default_retention_ms": 100,
        "pii_retention_limit_ms": 100,
        "phi_retention_limit_ms": 50,
        "max_retention_by_class_ms": {
          "audio_raw": 100,
          "text_raw": 100,
          "PII": 100,
          "PHI": 50,
          "derived_summary": 100,
          "metadata": 100
        }
      }
    }
  }
}`)

	snapshot, err := LoadRetentionPolicySnapshotFromFile(path)
	if err != nil {
		t.Fatalf("expected retention snapshot from file, got %v", err)
	}
	if snapshot.DefaultPolicy == nil {
		t.Fatalf("expected non-nil default policy")
	}
	if snapshot.DefaultPolicy.MaxRetentionByClassMS[eventabi.PayloadPII] != 500 {
		t.Fatalf("unexpected default pii retention window: %+v", snapshot.DefaultPolicy)
	}
	tenantPolicy, ok := snapshot.TenantPolicies["tenant-a"]
	if !ok {
		t.Fatalf("expected tenant-a policy in snapshot: %+v", snapshot.TenantPolicies)
	}
	if tenantPolicy.DefaultRetentionMS != 100 || tenantPolicy.MaxRetentionByClassMS[eventabi.PayloadMetadata] != 100 {
		t.Fatalf("unexpected tenant-a policy: %+v", tenantPolicy)
	}
}

func TestLoadRetentionPolicySnapshotFromEnvUsesHTTPPrecedence(t *testing.T) {
	filePath := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "retention": {
    "default_policy": {
      "tenant_id": "retention-policy-default",
      "default_retention_ms": 1000,
      "pii_retention_limit_ms": 1000,
      "phi_retention_limit_ms": 500,
        "max_retention_by_class_ms": {
          "audio_raw": 1000,
          "text_raw": 1000,
          "PII": 500,
          "PHI": 250,
          "derived_summary": 1000,
          "metadata": 1000
        }
    }
  }
}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version": "cp-snapshot-distribution/v1",
  "retention": {
    "tenant_policies": {
      "tenant-a": {
        "tenant_id": "tenant-a",
        "default_retention_ms": 200,
        "pii_retention_limit_ms": 200,
        "phi_retention_limit_ms": 100,
        "max_retention_by_class_ms": {
          "audio_raw": 200,
          "text_raw": 200,
          "PII": 200,
          "PHI": 100,
          "derived_summary": 200,
          "metadata": 200
        }
      }
    }
  }
}`))
	}))
	defer server.Close()

	t.Setenv(EnvFileAdapterPath, filePath)
	t.Setenv(EnvHTTPAdapterURL, "")
	t.Setenv(EnvHTTPAdapterURLs, server.URL)

	snapshot, source, err := LoadRetentionPolicySnapshotFromEnv()
	if err != nil {
		t.Fatalf("expected env snapshot from http source, got %v", err)
	}
	if source != RetentionSnapshotSourceHTTP {
		t.Fatalf("expected http source, got %s", source)
	}
	if _, ok := snapshot.TenantPolicies["tenant-a"]; !ok {
		t.Fatalf("expected tenant policy from http source: %+v", snapshot.TenantPolicies)
	}
	if snapshot.DefaultPolicy != nil {
		t.Fatalf("expected file source to be ignored when http env is configured")
	}
}

func TestLoadRetentionPolicySnapshotFromFileStaleClassification(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{
  "schema_version": "cp-snapshot-distribution/v1",
  "retention": {"stale": true}
}`)

	_, err := LoadRetentionPolicySnapshotFromFile(path)
	if err == nil {
		t.Fatalf("expected stale retention snapshot error")
	}
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeSnapshotStale {
		t.Fatalf("expected snapshot_stale code, got %s", backendErr.Code)
	}
}

func TestLoadRetentionPolicySnapshotFromHTTPFailover(t *testing.T) {
	t.Parallel()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "retention": {
    "default_policy": {
      "tenant_id": "retention-policy-default",
      "default_retention_ms": 300,
      "pii_retention_limit_ms": 300,
      "phi_retention_limit_ms": 100,
      "max_retention_by_class_ms": {
        "audio_raw": 300,
        "text_raw": 300,
        "PII": 300,
        "PHI": 100,
        "derived_summary": 300,
        "metadata": 300
      }
    }
  }
}`))
	}))
	defer second.Close()

	snapshot, err := LoadRetentionPolicySnapshotFromHTTP(HTTPAdapterConfig{
		URLs:             []string{first.URL, second.URL},
		RetryMaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("expected http failover snapshot load, got %v", err)
	}
	if snapshot.DefaultPolicy == nil || snapshot.DefaultPolicy.DefaultRetentionMS != 300 {
		t.Fatalf("unexpected snapshot from failover path: %+v", snapshot)
	}
}

func TestLoadRetentionPolicySnapshotFromFileRequiresRetentionSection(t *testing.T) {
	t.Parallel()

	path := writeDistributionArtifact(t, `{"schema_version":"cp-snapshot-distribution/v1"}`)
	_, err := LoadRetentionPolicySnapshotFromFile(path)
	if err == nil {
		t.Fatalf("expected missing retention snapshot error")
	}
	var backendErr BackendError
	if !errors.As(err, &backendErr) {
		t.Fatalf("expected backend error type, got %T", err)
	}
	if backendErr.Code != ErrorCodeSnapshotMissing {
		t.Fatalf("expected snapshot_missing code, got %s", backendErr.Code)
	}
}
