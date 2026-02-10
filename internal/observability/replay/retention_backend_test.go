package replay

import (
	"errors"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestBackendRetentionPolicyResolverUsesBackendPolicy(t *testing.T) {
	t.Parallel()

	resolver := BackendRetentionPolicyResolver{
		Backend: stubRetentionPolicyResolver{
			resolveFn: func(tenantID string) (RetentionPolicy, error) {
				return RetentionPolicy{
					TenantID:            "",
					DefaultRetentionMS:  1_000,
					PIIRetentionLimitMS: 1_000,
					PHIRetentionLimitMS: 1_000,
					MaxRetentionByClassMS: map[eventabi.PayloadClass]int64{
						eventabi.PayloadAudioRaw:       1_000,
						eventabi.PayloadTextRaw:        1_000,
						eventabi.PayloadPII:            500,
						eventabi.PayloadPHI:            500,
						eventabi.PayloadDerivedSummary: 1_000,
						eventabi.PayloadMetadata:       1_000,
					},
				}, nil
			},
		},
	}

	policy, err := resolver.ResolveRetentionPolicy("tenant-a")
	if err != nil {
		t.Fatalf("expected backend retention policy, got %v", err)
	}
	if policy.TenantID != "tenant-a" {
		t.Fatalf("expected normalized tenant_id, got %+v", policy)
	}
}

func TestBackendRetentionPolicyResolverFallbackAndDefault(t *testing.T) {
	t.Parallel()

	fallbackResolver := BackendRetentionPolicyResolver{
		Fallback: StaticRetentionPolicyResolver{
			DefaultPolicy: DefaultRetentionPolicy("tenant-default"),
		},
	}
	fallbackPolicy, err := fallbackResolver.ResolveRetentionPolicy("tenant-b")
	if err != nil {
		t.Fatalf("expected fallback resolution, got %v", err)
	}
	if fallbackPolicy.TenantID != "tenant-b" {
		t.Fatalf("expected fallback policy tenant rewrite, got %+v", fallbackPolicy)
	}

	defaultResolver := BackendRetentionPolicyResolver{}
	defaultPolicy, err := defaultResolver.ResolveRetentionPolicy("tenant-c")
	if err != nil {
		t.Fatalf("expected default policy resolution, got %v", err)
	}
	if defaultPolicy.TenantID != "tenant-c" {
		t.Fatalf("expected default policy tenant, got %+v", defaultPolicy)
	}
}

func TestBackendRetentionPolicyResolverBackendErrorAndMismatch(t *testing.T) {
	t.Parallel()

	errResolver := BackendRetentionPolicyResolver{
		Backend: stubRetentionPolicyResolver{
			resolveFn: func(string) (RetentionPolicy, error) {
				return RetentionPolicy{}, errors.New("backend unavailable")
			},
		},
	}
	if _, err := errResolver.ResolveRetentionPolicy("tenant-a"); err == nil {
		t.Fatalf("expected backend resolution error")
	}

	mismatchResolver := BackendRetentionPolicyResolver{
		Backend: stubRetentionPolicyResolver{
			resolveFn: func(string) (RetentionPolicy, error) {
				p := DefaultRetentionPolicy("tenant-other")
				return p, nil
			},
		},
	}
	if _, err := mismatchResolver.ResolveRetentionPolicy("tenant-a"); err == nil {
		t.Fatalf("expected tenant mismatch error")
	}
}

func TestEnforceTenantRetentionWithResolver(t *testing.T) {
	t.Parallel()

	store := NewInMemoryArtifactStore()
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-expired",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		PayloadClass: eventabi.PayloadMetadata,
		RecordedAtMS: 100,
	})

	resolver := BackendRetentionPolicyResolver{
		Backend: stubRetentionPolicyResolver{
			resolveFn: func(string) (RetentionPolicy, error) {
				policy := DefaultRetentionPolicy("tenant-a")
				policy.MaxRetentionByClassMS[eventabi.PayloadMetadata] = 10
				return policy, nil
			},
		},
	}

	result, err := EnforceTenantRetentionWithResolver(store, resolver, "tenant-a", 1_000)
	if err != nil {
		t.Fatalf("expected retention sweep success, got %v", err)
	}
	if result.DeletedArtifacts != 1 {
		t.Fatalf("expected one deleted artifact, got %+v", result)
	}
}

func TestEnforceTenantRetentionWithResolverDetailedIncludesClassCounts(t *testing.T) {
	t.Parallel()

	store := NewInMemoryArtifactStore()
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "metadata-expired",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		PayloadClass: eventabi.PayloadMetadata,
		RecordedAtMS: 100,
	})
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "pii-expired",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-2",
		PayloadClass: eventabi.PayloadPII,
		RecordedAtMS: 100,
	})
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "text-retained",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-3",
		PayloadClass: eventabi.PayloadTextRaw,
		RecordedAtMS: 990,
	})
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "other-tenant-expired",
		TenantID:     "tenant-b",
		SessionID:    "session-9",
		TurnID:       "turn-9",
		PayloadClass: eventabi.PayloadMetadata,
		RecordedAtMS: 1,
	})

	resolver := BackendRetentionPolicyResolver{
		Backend: stubRetentionPolicyResolver{
			resolveFn: func(string) (RetentionPolicy, error) {
				policy := DefaultRetentionPolicy("tenant-a")
				policy.MaxRetentionByClassMS[eventabi.PayloadMetadata] = 10
				policy.MaxRetentionByClassMS[eventabi.PayloadPII] = 20
				policy.PIIRetentionLimitMS = 20
				policy.MaxRetentionByClassMS[eventabi.PayloadTextRaw] = 50
				return policy, nil
			},
		},
	}

	result, err := EnforceTenantRetentionWithResolverDetailed(store, resolver, "tenant-a", 1_000)
	if err != nil {
		t.Fatalf("expected detailed retention sweep success, got %v", err)
	}
	if result.Summary.DeletedArtifacts != 2 {
		t.Fatalf("expected two deleted artifacts, got %+v", result.Summary)
	}
	if got := result.DeletedByClass[eventabi.PayloadMetadata]; got != 1 {
		t.Fatalf("expected one metadata deletion, got %d", got)
	}
	if got := result.DeletedByClass[eventabi.PayloadPII]; got != 1 {
		t.Fatalf("expected one PII deletion, got %d", got)
	}
	if got := result.DeletedByClass[eventabi.PayloadTextRaw]; got != 0 {
		t.Fatalf("expected zero text_raw deletions, got %d", got)
	}
}

func TestEnforceTenantRetentionWithResolverRequiresStore(t *testing.T) {
	t.Parallel()

	_, err := EnforceTenantRetentionWithResolver(nil, BackendRetentionPolicyResolver{}, "tenant-a", 1_000)
	if !errors.Is(err, ErrReplayArtifactStoreRequired) {
		t.Fatalf("expected missing store error, got %v", err)
	}
}

func TestEnforceTenantRetentionWithResolverDetailedRequiresStore(t *testing.T) {
	t.Parallel()

	_, err := EnforceTenantRetentionWithResolverDetailed(nil, BackendRetentionPolicyResolver{}, "tenant-a", 1_000)
	if !errors.Is(err, ErrReplayArtifactStoreRequired) {
		t.Fatalf("expected missing store error, got %v", err)
	}
}

type stubRetentionPolicyResolver struct {
	resolveFn func(tenantID string) (RetentionPolicy, error)
}

func (s stubRetentionPolicyResolver) ResolveRetentionPolicy(tenantID string) (RetentionPolicy, error) {
	return s.resolveFn(tenantID)
}
