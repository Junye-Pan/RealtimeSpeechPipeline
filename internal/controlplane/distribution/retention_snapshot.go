package distribution

import (
	"fmt"
	"os"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// RetentionSnapshotSource describes the CP distribution backend used for retention policies.
type RetentionSnapshotSource string

const (
	RetentionSnapshotSourceFile RetentionSnapshotSource = "cp_distribution_file"
	RetentionSnapshotSourceHTTP RetentionSnapshotSource = "cp_distribution_http"
)

// RetentionPolicy mirrors retention policy records distributed through CP snapshots.
type RetentionPolicy struct {
	TenantID              string
	DefaultRetentionMS    int64
	PIIRetentionLimitMS   int64
	PHIRetentionLimitMS   int64
	MaxRetentionByClassMS map[eventabi.PayloadClass]int64
}

// RetentionPolicySnapshot captures distributed default and per-tenant retention policies.
type RetentionPolicySnapshot struct {
	DefaultPolicy  *RetentionPolicy
	TenantPolicies map[string]RetentionPolicy
}

// LoadRetentionPolicySnapshotFromEnv resolves retention policy snapshots from env-configured CP distribution.
func LoadRetentionPolicySnapshotFromEnv() (RetentionPolicySnapshot, RetentionSnapshotSource, error) {
	if strings.TrimSpace(os.Getenv(EnvHTTPAdapterURLs)) != "" || strings.TrimSpace(os.Getenv(EnvHTTPAdapterURL)) != "" {
		cfg, err := HTTPAdapterConfigFromEnv()
		if err != nil {
			return RetentionPolicySnapshot{}, RetentionSnapshotSourceHTTP, err
		}
		snapshot, err := LoadRetentionPolicySnapshotFromHTTP(cfg)
		if err != nil {
			return RetentionPolicySnapshot{}, RetentionSnapshotSourceHTTP, err
		}
		return snapshot, RetentionSnapshotSourceHTTP, nil
	}

	cfg, err := FileAdapterConfigFromEnv()
	if err != nil {
		return RetentionPolicySnapshot{}, RetentionSnapshotSourceFile, err
	}
	snapshot, err := LoadRetentionPolicySnapshotFromFile(cfg.Path)
	if err != nil {
		return RetentionPolicySnapshot{}, RetentionSnapshotSourceFile, err
	}
	return snapshot, RetentionSnapshotSourceFile, nil
}

// LoadRetentionPolicySnapshotFromFile resolves retention policy snapshots from a file-backed CP distribution artifact.
func LoadRetentionPolicySnapshotFromFile(path string) (RetentionPolicySnapshot, error) {
	adapter, err := newFileAdapter(FileAdapterConfig{Path: path})
	if err != nil {
		return RetentionPolicySnapshot{}, err
	}
	return retentionSnapshotFromArtifact(adapter.path, adapter.artifact)
}

// LoadRetentionPolicySnapshotFromHTTP resolves retention policy snapshots from HTTP-backed CP distribution endpoints.
func LoadRetentionPolicySnapshotFromHTTP(cfg HTTPAdapterConfig) (RetentionPolicySnapshot, error) {
	provider, err := newHTTPSnapshotProvider(cfg)
	if err != nil {
		return RetentionPolicySnapshot{}, err
	}
	adapter, err := provider.current()
	if err != nil {
		return RetentionPolicySnapshot{}, err
	}
	return retentionSnapshotFromArtifact(adapter.path, adapter.artifact)
}

func retentionSnapshotFromArtifact(path string, artifact fileArtifact) (RetentionPolicySnapshot, error) {
	if artifact.Stale || artifact.Retention.Stale {
		return RetentionPolicySnapshot{}, BackendError{
			Service: "retention_policy",
			Code:    ErrorCodeSnapshotStale,
			Path:    path,
			Cause:   fmt.Errorf("snapshot marked stale"),
		}
	}

	section := artifact.Retention
	if section.DefaultPolicy == nil && len(section.TenantPolicies) == 0 {
		return RetentionPolicySnapshot{}, BackendError{
			Service: "retention_policy",
			Code:    ErrorCodeSnapshotMissing,
			Path:    path,
			Cause:   fmt.Errorf("missing retention policy snapshot"),
		}
	}

	snapshot := RetentionPolicySnapshot{
		TenantPolicies: make(map[string]RetentionPolicy, len(section.TenantPolicies)),
	}
	if section.DefaultPolicy != nil {
		policy := toRetentionPolicy(*section.DefaultPolicy)
		snapshot.DefaultPolicy = &policy
	}
	for tenantID, policy := range section.TenantPolicies {
		snapshot.TenantPolicies[tenantID] = toRetentionPolicy(policy)
	}
	if len(snapshot.TenantPolicies) == 0 {
		snapshot.TenantPolicies = nil
	}
	return snapshot, nil
}

func toRetentionPolicy(policy fileRetentionPolicy) RetentionPolicy {
	return RetentionPolicy{
		TenantID:              policy.TenantID,
		DefaultRetentionMS:    policy.DefaultRetentionMS,
		PIIRetentionLimitMS:   policy.PIIRetentionLimitMS,
		PHIRetentionLimitMS:   policy.PHIRetentionLimitMS,
		MaxRetentionByClassMS: cloneRetentionWindows(policy.MaxRetentionByClassMS),
	}
}

func cloneRetentionWindows(in map[eventabi.PayloadClass]int64) map[eventabi.PayloadClass]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[eventabi.PayloadClass]int64, len(in))
	for class, window := range in {
		out[class] = window
	}
	return out
}
