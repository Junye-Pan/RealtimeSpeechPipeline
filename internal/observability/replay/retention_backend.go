package replay

import (
	"errors"
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

var (
	ErrReplayArtifactStoreRequired = errors.New("replay artifact store is required")
)

// BackendRetentionPolicyResolver resolves policy from a backend source with deterministic fallback.
type BackendRetentionPolicyResolver struct {
	Backend  RetentionPolicyResolver
	Fallback RetentionPolicyResolver
}

// RetentionSweepDetailedResult captures retention sweep totals plus class-level deletion counts.
type RetentionSweepDetailedResult struct {
	Summary        RetentionSweepResult
	DeletedByClass map[eventabi.PayloadClass]int
}

// ResolveRetentionPolicy resolves tenant-scoped policy from backend or fallback defaults.
func (r BackendRetentionPolicyResolver) ResolveRetentionPolicy(tenantID string) (RetentionPolicy, error) {
	if tenantID == "" {
		return RetentionPolicy{}, fmt.Errorf("tenant_id is required")
	}

	if r.Backend != nil {
		policy, err := r.Backend.ResolveRetentionPolicy(tenantID)
		if err != nil {
			return RetentionPolicy{}, fmt.Errorf("resolve retention policy backend: %w", err)
		}
		return normalizeResolvedRetentionPolicy(policy, tenantID)
	}

	if r.Fallback != nil {
		policy, err := r.Fallback.ResolveRetentionPolicy(tenantID)
		if err != nil {
			return RetentionPolicy{}, fmt.Errorf("resolve retention policy fallback: %w", err)
		}
		return normalizeResolvedRetentionPolicy(policy, tenantID)
	}

	return DefaultRetentionPolicy(tenantID), nil
}

// EnforceTenantRetentionWithResolver resolves tenant policy and applies retention to a store.
func EnforceTenantRetentionWithResolver(
	store *InMemoryArtifactStore,
	resolver RetentionPolicyResolver,
	tenantID string,
	nowMS int64,
) (RetentionSweepResult, error) {
	detailed, err := EnforceTenantRetentionWithResolverDetailed(store, resolver, tenantID, nowMS)
	if err != nil {
		return RetentionSweepResult{}, err
	}
	return detailed.Summary, nil
}

// EnforceTenantRetentionWithResolverDetailed resolves tenant policy and returns class-level deletion counts.
func EnforceTenantRetentionWithResolverDetailed(
	store *InMemoryArtifactStore,
	resolver RetentionPolicyResolver,
	tenantID string,
	nowMS int64,
) (RetentionSweepDetailedResult, error) {
	if store == nil {
		return RetentionSweepDetailedResult{}, ErrReplayArtifactStoreRequired
	}
	if tenantID == "" {
		return RetentionSweepDetailedResult{}, fmt.Errorf("tenant_id is required")
	}
	if resolver == nil {
		resolver = BackendRetentionPolicyResolver{}
	}

	policy, err := resolver.ResolveRetentionPolicy(tenantID)
	if err != nil {
		return RetentionSweepDetailedResult{}, fmt.Errorf("resolve tenant retention policy: %w", err)
	}
	normalizedPolicy, err := normalizeResolvedRetentionPolicy(policy, tenantID)
	if err != nil {
		return RetentionSweepDetailedResult{}, err
	}

	beforeRecords := store.Snapshot()
	summary, err := store.EnforceRetention(normalizedPolicy, nowMS)
	if err != nil {
		return RetentionSweepDetailedResult{}, err
	}
	afterRecords := store.Snapshot()

	return RetentionSweepDetailedResult{
		Summary:        summary,
		DeletedByClass: deletedByClass(normalizedPolicy.TenantID, beforeRecords, afterRecords),
	}, nil
}

func normalizeResolvedRetentionPolicy(policy RetentionPolicy, tenantID string) (RetentionPolicy, error) {
	if policy.TenantID == "" {
		policy.TenantID = tenantID
	}
	if policy.TenantID != tenantID {
		return RetentionPolicy{}, fmt.Errorf("retention policy tenant mismatch: expected %s got %s", tenantID, policy.TenantID)
	}
	if err := policy.Validate(); err != nil {
		return RetentionPolicy{}, err
	}
	return policy, nil
}

type replayArtifactFingerprint struct {
	ArtifactID   string
	TenantID     string
	SessionID    string
	TurnID       string
	PayloadClass eventabi.PayloadClass
	RecordedAtMS int64
	State        ArtifactState
}

func deletedByClass(tenantID string, beforeRecords []ReplayArtifactRecord, afterRecords []ReplayArtifactRecord) map[eventabi.PayloadClass]int {
	beforeCounts := tenantArtifactCountsByFingerprint(tenantID, beforeRecords)
	afterCounts := tenantArtifactCountsByFingerprint(tenantID, afterRecords)
	deletedByClass := make(map[eventabi.PayloadClass]int)

	for fingerprint, beforeCount := range beforeCounts {
		afterCount := afterCounts[fingerprint]
		deletedCount := beforeCount - afterCount
		if deletedCount <= 0 {
			continue
		}
		deletedByClass[fingerprint.PayloadClass] += deletedCount
	}
	return deletedByClass
}

func tenantArtifactCountsByFingerprint(tenantID string, records []ReplayArtifactRecord) map[replayArtifactFingerprint]int {
	counts := make(map[replayArtifactFingerprint]int, len(records))
	for _, record := range records {
		if record.TenantID != tenantID {
			continue
		}
		fingerprint := replayArtifactFingerprint{
			ArtifactID:   record.ArtifactID,
			TenantID:     record.TenantID,
			SessionID:    record.SessionID,
			TurnID:       record.TurnID,
			PayloadClass: record.PayloadClass,
			RecordedAtMS: record.RecordedAtMS,
			State:        normalizeArtifactState(record.State),
		}
		counts[fingerprint]++
	}
	return counts
}
