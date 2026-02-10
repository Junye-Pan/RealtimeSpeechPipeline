package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/distribution"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
)

func TestRunRetentionSweepUsesBackendPolicyResolver(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "store.json")
	policyPath := filepath.Join(tmp, "policy.json")
	reportPath := filepath.Join(tmp, "report.json")

	mustWriteJSON(t, storePath, retentionStoreArtifact{
		Records: []replay.ReplayArtifactRecord{
			{
				ArtifactID:   "expired-metadata-a",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-1",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 100,
			},
			{
				ArtifactID:   "expired-pii-a",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-2",
				PayloadClass: eventabi.PayloadPII,
				RecordedAtMS: 100,
			},
			{
				ArtifactID:   "retained-a",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-3",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 995,
			},
			{
				ArtifactID:   "other-tenant",
				TenantID:     "tenant-b",
				SessionID:    "session-9",
				TurnID:       "turn-9",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 1,
			},
		},
	})

	policy := replay.DefaultRetentionPolicy("tenant-a")
	policy.MaxRetentionByClassMS[eventabi.PayloadMetadata] = 10
	policy.MaxRetentionByClassMS[eventabi.PayloadPII] = 20
	policy.PIIRetentionLimitMS = 20
	mustWriteJSON(t, policyPath, retentionPolicyArtifact{
		TenantPolicies: map[string]retentionPolicyArtifactPolicy{
			"tenant-a": runtimeToArtifactPolicy(policy),
		},
	})

	var stdout bytes.Buffer
	if err := run([]string{
		"retention-sweep",
		"-store", storePath,
		"-policy", policyPath,
		"-report", reportPath,
		"-tenants", "tenant-a",
		"-now-ms", "1000",
	}, &stdout, &bytes.Buffer{}, fixedNow()); err != nil {
		t.Fatalf("unexpected retention sweep error: %v", err)
	}

	storeArtifact := mustReadStoreArtifact(t, storePath)
	if len(storeArtifact.Records) != 2 {
		t.Fatalf("expected two records after sweep, got %+v", storeArtifact.Records)
	}
	if containsArtifact(storeArtifact.Records, "expired-metadata-a") || containsArtifact(storeArtifact.Records, "expired-pii-a") {
		t.Fatalf("expected expired artifacts to be deleted, got %+v", storeArtifact.Records)
	}
	if !containsArtifact(storeArtifact.Records, "retained-a") || !containsArtifact(storeArtifact.Records, "other-tenant") {
		t.Fatalf("expected retained artifacts missing, got %+v", storeArtifact.Records)
	}

	report := mustReadSweepReport(t, reportPath)
	if report.Runs != 1 || len(report.RunResults) != 1 {
		t.Fatalf("unexpected run metadata: %+v", report)
	}
	if report.PolicySource != string(retentionPolicySourceArtifactOverride) || report.PolicyFallbackReason != "" {
		t.Fatalf("expected explicit policy artifact source metadata, got source=%q fallback=%q", report.PolicySource, report.PolicyFallbackReason)
	}
	if report.RunResults[0].TotalDeleted != 2 {
		t.Fatalf("expected two deletions in report, got %+v", report.RunResults[0])
	}
	if report.RunResults[0].PolicySource != string(retentionPolicySourceArtifactOverride) || report.RunResults[0].PolicyFallbackReason != "" {
		t.Fatalf("expected per-run explicit policy artifact source metadata, got %+v", report.RunResults[0])
	}
	if len(report.RunResults[0].TenantResults) != 1 {
		t.Fatalf("expected one tenant result, got %+v", report.RunResults[0].TenantResults)
	}

	tenantResult := report.RunResults[0].TenantResults[0]
	assertAllPayloadClassesPresent(t, tenantResult.DeletedByClass)
	assertClassCount(t, tenantResult.DeletedByClass, eventabi.PayloadMetadata, 1)
	assertClassCount(t, tenantResult.DeletedByClass, eventabi.PayloadPII, 1)
	assertClassCount(t, tenantResult.DeletedByClass, eventabi.PayloadAudioRaw, 0)

	assertAllPayloadClassesPresent(t, report.RunResults[0].DeletedByClass)
	assertClassCount(t, report.RunResults[0].DeletedByClass, eventabi.PayloadMetadata, 1)
	assertClassCount(t, report.RunResults[0].DeletedByClass, eventabi.PayloadPII, 1)
}

func TestRunRetentionSweepUsesFallbackDefaultsWithoutPolicyFile(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "store.json")
	reportPath := filepath.Join(tmp, "report.json")
	t.Setenv(distribution.EnvFileAdapterPath, "")
	t.Setenv(distribution.EnvHTTPAdapterURL, "")
	t.Setenv(distribution.EnvHTTPAdapterURLs, "")

	mustWriteJSON(t, storePath, retentionStoreArtifact{
		Records: []replay.ReplayArtifactRecord{
			{
				ArtifactID:   "metadata-a",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-1",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 1_000,
			},
		},
	})

	if err := run([]string{
		"retention-sweep",
		"-store", storePath,
		"-report", reportPath,
		"-tenants", "tenant-a",
		"-now-ms", "2000",
	}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow()); err != nil {
		t.Fatalf("unexpected fallback retention sweep error: %v", err)
	}

	storeArtifact := mustReadStoreArtifact(t, storePath)
	if len(storeArtifact.Records) != 1 {
		t.Fatalf("expected record to remain under fallback default retention, got %+v", storeArtifact.Records)
	}

	report := mustReadSweepReport(t, reportPath)
	if report.PolicySource != string(retentionPolicySourceDefaultFallback) {
		t.Fatalf("expected fallback policy source without configured backends, got %s", report.PolicySource)
	}
	if report.PolicyFallbackReason != retentionPolicyFallbackReasonCPDistributionUnconfigured {
		t.Fatalf("expected unconfigured fallback reason, got %q", report.PolicyFallbackReason)
	}
	if report.RunResults[0].TotalDeleted != 0 {
		t.Fatalf("expected zero deletions under fallback default retention, got %+v", report.RunResults[0])
	}
	if report.RunResults[0].PolicySource != string(retentionPolicySourceDefaultFallback) ||
		report.RunResults[0].PolicyFallbackReason != retentionPolicyFallbackReasonCPDistributionUnconfigured {
		t.Fatalf("unexpected per-run fallback source metadata: %+v", report.RunResults[0])
	}
	assertAllPayloadClassesPresent(t, report.RunResults[0].DeletedByClass)
	for _, class := range payloadClasses() {
		assertClassCount(t, report.RunResults[0].DeletedByClass, class, 0)
		assertClassCount(t, report.RunResults[0].TenantResults[0].DeletedByClass, class, 0)
	}
}

func TestRunRetentionSweepScheduledRunsUseIntervalOffsets(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "store.json")
	policyPath := filepath.Join(tmp, "policy.json")
	reportPath := filepath.Join(tmp, "report.json")

	mustWriteJSON(t, storePath, retentionStoreArtifact{
		Records: []replay.ReplayArtifactRecord{
			{
				ArtifactID:   "metadata-a",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-1",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 0,
			},
		},
	})

	policy := replay.DefaultRetentionPolicy("tenant-a")
	policy.MaxRetentionByClassMS[eventabi.PayloadMetadata] = 100
	mustWriteJSON(t, policyPath, retentionPolicyArtifact{
		TenantPolicies: map[string]retentionPolicyArtifactPolicy{
			"tenant-a": runtimeToArtifactPolicy(policy),
		},
	})

	if err := run([]string{
		"retention-sweep",
		"-store", storePath,
		"-policy", policyPath,
		"-report", reportPath,
		"-tenants", "tenant-a",
		"-now-ms", "50",
		"-interval-ms", "100",
		"-runs", "2",
	}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow()); err != nil {
		t.Fatalf("unexpected scheduled retention sweep error: %v", err)
	}

	report := mustReadSweepReport(t, reportPath)
	if len(report.RunResults) != 2 {
		t.Fatalf("expected two run results, got %+v", report)
	}
	if report.PolicySource != string(retentionPolicySourceArtifactOverride) {
		t.Fatalf("expected policy source to stay explicit artifact across runs, got %s", report.PolicySource)
	}
	if report.RunResults[0].RunAtMS != 50 || report.RunResults[1].RunAtMS != 150 {
		t.Fatalf("expected interval-offset run timestamps, got %+v", report.RunResults)
	}
	if report.RunResults[0].TotalDeleted != 0 || report.RunResults[1].TotalDeleted != 1 {
		t.Fatalf("expected deletion only in second run, got %+v", report.RunResults)
	}
	assertClassCount(t, report.RunResults[0].DeletedByClass, eventabi.PayloadMetadata, 0)
	assertClassCount(t, report.RunResults[1].DeletedByClass, eventabi.PayloadMetadata, 1)

	storeArtifact := mustReadStoreArtifact(t, storePath)
	if len(storeArtifact.Records) != 0 {
		t.Fatalf("expected record deleted after second run, got %+v", storeArtifact.Records)
	}
}

func TestRunRetentionSweepUsesCPDistributionFilePolicySnapshot(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "store.json")
	reportPath := filepath.Join(tmp, "report.json")
	distributionPath := filepath.Join(tmp, "cp-distribution.json")

	mustWriteJSON(t, storePath, retentionStoreArtifact{
		Records: []replay.ReplayArtifactRecord{
			{
				ArtifactID:   "metadata-expired",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-1",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 100,
			},
		},
	})

	mustWriteJSON(t, distributionPath, map[string]any{
		"schema_version": "cp-snapshot-distribution/v1",
		"retention": map[string]any{
			"tenant_policies": map[string]any{
				"tenant-a": map[string]any{
					"tenant_id":              "tenant-a",
					"default_retention_ms":   10,
					"pii_retention_limit_ms": 10,
					"phi_retention_limit_ms": 10,
					"max_retention_by_class_ms": map[string]any{
						"audio_raw":       10,
						"text_raw":        10,
						"PII":             10,
						"PHI":             10,
						"derived_summary": 10,
						"metadata":        10,
					},
				},
			},
		},
	})

	t.Setenv(distribution.EnvFileAdapterPath, distributionPath)
	t.Setenv(distribution.EnvHTTPAdapterURL, "")
	t.Setenv(distribution.EnvHTTPAdapterURLs, "")

	if err := run([]string{
		"retention-sweep",
		"-store", storePath,
		"-report", reportPath,
		"-tenants", "tenant-a",
		"-now-ms", "1000",
	}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow()); err != nil {
		t.Fatalf("unexpected retention sweep error: %v", err)
	}

	storeArtifact := mustReadStoreArtifact(t, storePath)
	if len(storeArtifact.Records) != 0 {
		t.Fatalf("expected metadata artifact deleted by cp distribution retention policy, got %+v", storeArtifact.Records)
	}

	report := mustReadSweepReport(t, reportPath)
	if report.PolicySource != string(retentionPolicySourceCPDistributionFile) || report.PolicyFallbackReason != "" {
		t.Fatalf("expected cp distribution file policy source, got source=%q fallback=%q", report.PolicySource, report.PolicyFallbackReason)
	}
	if report.RunResults[0].PolicySource != string(retentionPolicySourceCPDistributionFile) || report.RunResults[0].PolicyFallbackReason != "" {
		t.Fatalf("unexpected run source metadata: %+v", report.RunResults[0])
	}
}

func TestRunRetentionSweepUsesCPDistributionHTTPPolicySnapshot(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "store.json")
	reportPath := filepath.Join(tmp, "report.json")

	mustWriteJSON(t, storePath, retentionStoreArtifact{
		Records: []replay.ReplayArtifactRecord{
			{
				ArtifactID:   "metadata-expired",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-1",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 100,
			},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "retention": {
    "tenant_policies": {
      "tenant-a": {
        "tenant_id": "tenant-a",
        "default_retention_ms": 10,
        "pii_retention_limit_ms": 10,
        "phi_retention_limit_ms": 10,
        "max_retention_by_class_ms": {
          "audio_raw": 10,
          "text_raw": 10,
          "PII": 10,
          "PHI": 10,
          "derived_summary": 10,
          "metadata": 10
        }
      }
    }
  }
}`))
	}))
	defer server.Close()

	t.Setenv(distribution.EnvHTTPAdapterURLs, server.URL)
	t.Setenv(distribution.EnvHTTPAdapterURL, "")
	t.Setenv(distribution.EnvFileAdapterPath, "")

	if err := run([]string{
		"retention-sweep",
		"-store", storePath,
		"-report", reportPath,
		"-tenants", "tenant-a",
		"-now-ms", "1000",
	}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow()); err != nil {
		t.Fatalf("unexpected retention sweep error: %v", err)
	}

	storeArtifact := mustReadStoreArtifact(t, storePath)
	if len(storeArtifact.Records) != 0 {
		t.Fatalf("expected metadata artifact deleted by cp http retention policy, got %+v", storeArtifact.Records)
	}

	report := mustReadSweepReport(t, reportPath)
	if report.PolicySource != string(retentionPolicySourceCPDistributionHTTP) || report.PolicyFallbackReason != "" {
		t.Fatalf("expected cp distribution http policy source, got source=%q fallback=%q", report.PolicySource, report.PolicyFallbackReason)
	}
}

func TestRunRetentionSweepCPDistributionOutageRecoveryAcrossRuns(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "store.json")
	reportPath := filepath.Join(tmp, "report.json")

	mustWriteJSON(t, storePath, retentionStoreArtifact{
		Records: []replay.ReplayArtifactRecord{
			{
				ArtifactID:   "metadata-target",
				TenantID:     "tenant-a",
				SessionID:    "session-1",
				TurnID:       "turn-1",
				PayloadClass: eventabi.PayloadMetadata,
				RecordedAtMS: 100,
			},
		},
	})

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporary outage"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "schema_version":"cp-snapshot-distribution/v1",
  "retention": {
    "tenant_policies": {
      "tenant-a": {
        "tenant_id": "tenant-a",
        "default_retention_ms": 10,
        "pii_retention_limit_ms": 10,
        "phi_retention_limit_ms": 10,
        "max_retention_by_class_ms": {
          "audio_raw": 10,
          "text_raw": 10,
          "PII": 10,
          "PHI": 10,
          "derived_summary": 10,
          "metadata": 10
        }
      }
    }
  }
}`))
	}))
	defer server.Close()

	t.Setenv(distribution.EnvHTTPAdapterURLs, server.URL)
	t.Setenv(distribution.EnvHTTPAdapterURL, "")
	t.Setenv(distribution.EnvFileAdapterPath, "")
	t.Setenv(distribution.EnvHTTPAdapterRetryMaxAttempts, "1")

	if err := run([]string{
		"retention-sweep",
		"-store", storePath,
		"-report", reportPath,
		"-tenants", "tenant-a",
		"-now-ms", "1000",
		"-runs", "2",
		"-interval-ms", "0",
	}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow()); err != nil {
		t.Fatalf("unexpected retention sweep error: %v", err)
	}

	report := mustReadSweepReport(t, reportPath)
	if len(report.RunResults) != 2 {
		t.Fatalf("expected two run results, got %+v", report.RunResults)
	}
	if report.PolicySource != string(retentionPolicySourceMixed) {
		t.Fatalf("expected mixed top-level policy source for outage/recovery run set, got %q", report.PolicySource)
	}
	if report.PolicyFallbackReason != retentionPolicyFallbackReasonMixed {
		t.Fatalf("expected mixed top-level fallback reason for outage/recovery run set, got %q", report.PolicyFallbackReason)
	}
	if report.RunResults[0].PolicySource != string(retentionPolicySourceDefaultFallback) ||
		report.RunResults[0].PolicyFallbackReason != retentionPolicyFallbackReasonCPDistributionFetchFailed {
		t.Fatalf("expected first run fallback on outage, got %+v", report.RunResults[0])
	}
	if report.RunResults[1].PolicySource != string(retentionPolicySourceCPDistributionHTTP) ||
		report.RunResults[1].PolicyFallbackReason != "" {
		t.Fatalf("expected second run recovery to cp distribution http source, got %+v", report.RunResults[1])
	}

	storeArtifact := mustReadStoreArtifact(t, storePath)
	if len(storeArtifact.Records) != 0 {
		t.Fatalf("expected artifact deleted after recovery run, got %+v", storeArtifact.Records)
	}
}

func TestRunRetentionSweepPolicyErrorsAreDeterministic(t *testing.T) {
	validPolicy := replay.DefaultRetentionPolicy("tenant-a")
	validPolicy.MaxRetentionByClassMS[eventabi.PayloadMetadata] = 1_000

	tenantMismatchPolicy := validPolicy
	tenantMismatchPolicy.TenantID = "tenant-b"

	validationPolicy := validPolicy
	validationPolicy.PIIRetentionLimitMS = 1_000
	validationPolicy.MaxRetentionByClassMS[eventabi.PayloadPII] = 1_001

	tests := []struct {
		name      string
		policy    any
		policyErr retentionSweepPolicyErrorCode
	}{
		{
			name:      "decode error",
			policy:    "{",
			policyErr: retentionSweepPolicyDecodeErrorCode,
		},
		{
			name:      "artifact invalid",
			policy:    "{}",
			policyErr: retentionSweepPolicyArtifactInvalidErrorCode,
		},
		{
			name: "tenant mismatch",
			policy: retentionPolicyArtifact{
				TenantPolicies: map[string]retentionPolicyArtifactPolicy{
					"tenant-a": runtimeToArtifactPolicy(tenantMismatchPolicy),
				},
			},
			policyErr: retentionSweepPolicyTenantMismatchErrorCode,
		},
		{
			name: "validation error",
			policy: retentionPolicyArtifact{
				TenantPolicies: map[string]retentionPolicyArtifactPolicy{
					"tenant-a": runtimeToArtifactPolicy(validationPolicy),
				},
			},
			policyErr: retentionSweepPolicyValidationErrorCode,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			storePath := filepath.Join(tmp, "store.json")
			policyPath := filepath.Join(tmp, "policy.json")

			mustWriteJSON(t, storePath, retentionStoreArtifact{Records: []replay.ReplayArtifactRecord{}})
			switch policy := tc.policy.(type) {
			case string:
				if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
					t.Fatalf("unexpected write error: %v", err)
				}
			default:
				mustWriteJSON(t, policyPath, policy)
			}

			err := run([]string{
				"retention-sweep",
				"-store", storePath,
				"-policy", policyPath,
				"-tenants", "tenant-a",
			}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow())
			if err == nil {
				t.Fatalf("expected deterministic policy error")
			}

			var policyErr retentionSweepPolicyError
			if !errors.As(err, &policyErr) {
				t.Fatalf("expected retentionSweepPolicyError, got %T (%v)", err, err)
			}
			if policyErr.Code != tc.policyErr {
				t.Fatalf("expected policy error code %s, got %s", tc.policyErr, policyErr.Code)
			}
		})
	}
}

func TestRunRetentionSweepPolicyReadErrorCode(t *testing.T) {
	err := run([]string{
		"retention-sweep",
		"-store", filepath.Join(t.TempDir(), "store.json"),
		"-policy", filepath.Join(t.TempDir(), "missing-policy.json"),
		"-tenants", "tenant-a",
	}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow())
	if err == nil {
		t.Fatalf("expected policy read error")
	}

	var policyErr retentionSweepPolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected retentionSweepPolicyError, got %T (%v)", err, err)
	}
	if policyErr.Code != retentionSweepPolicyReadErrorCode {
		t.Fatalf("expected policy error code %s, got %s", retentionSweepPolicyReadErrorCode, policyErr.Code)
	}
}

func TestRunRetentionSweepRequiresTenants(t *testing.T) {
	err := run([]string{
		"retention-sweep",
		"-store", filepath.Join(t.TempDir(), "store.json"),
	}, &bytes.Buffer{}, &bytes.Buffer{}, fixedNow())
	if err == nil {
		t.Fatalf("expected tenant validation error")
	}
}

func fixedNow() func() time.Time {
	return func() time.Time {
		return time.Date(2026, time.February, 10, 12, 0, 0, 0, time.UTC)
	}
}

func payloadClasses() []eventabi.PayloadClass {
	return []eventabi.PayloadClass{
		eventabi.PayloadAudioRaw,
		eventabi.PayloadTextRaw,
		eventabi.PayloadPII,
		eventabi.PayloadPHI,
		eventabi.PayloadDerivedSummary,
		eventabi.PayloadMetadata,
	}
}

func runtimeToArtifactPolicy(policy replay.RetentionPolicy) retentionPolicyArtifactPolicy {
	return retentionPolicyArtifactPolicy{
		TenantID:              policy.TenantID,
		DefaultRetentionMS:    policy.DefaultRetentionMS,
		PIIRetentionLimitMS:   policy.PIIRetentionLimitMS,
		PHIRetentionLimitMS:   policy.PHIRetentionLimitMS,
		MaxRetentionByClassMS: policy.MaxRetentionByClassMS,
	}
}

func assertAllPayloadClassesPresent(t *testing.T, counters map[string]int) {
	t.Helper()
	for _, class := range payloadClasses() {
		if _, ok := counters[string(class)]; !ok {
			t.Fatalf("expected class %s in counters, got %+v", class, counters)
		}
	}
}

func assertClassCount(t *testing.T, counters map[string]int, class eventabi.PayloadClass, want int) {
	t.Helper()
	got, ok := counters[string(class)]
	if !ok {
		t.Fatalf("expected class %s in counters, got %+v", class, counters)
	}
	if got != want {
		t.Fatalf("expected class %s count=%d, got %d", class, want, got)
	}
}

func mustWriteJSON(t *testing.T, path string, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected json marshal error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("unexpected mkdir error: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
}

func mustReadStoreArtifact(t *testing.T, path string) retentionStoreArtifact {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unexpected store read error: %v", err)
	}
	var artifact retentionStoreArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("unexpected store decode error: %v", err)
	}
	return artifact
}

func mustReadSweepReport(t *testing.T, path string) retentionSweepReport {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unexpected report read error: %v", err)
	}
	var report retentionSweepReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("unexpected report decode error: %v", err)
	}
	return report
}

func containsArtifact(records []replay.ReplayArtifactRecord, artifactID string) bool {
	for _, record := range records {
		if record.ArtifactID == artifactID {
			return true
		}
	}
	return false
}
