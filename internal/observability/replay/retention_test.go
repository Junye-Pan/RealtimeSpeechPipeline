package replay

import (
	"errors"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestRetentionPolicyValidateRejectsPIIWindowBeyondLimit(t *testing.T) {
	t.Parallel()

	policy := DefaultRetentionPolicy("tenant-a")
	policy.MaxRetentionByClassMS[eventabi.PayloadPII] = policy.PIIRetentionLimitMS + 1

	if err := policy.Validate(); err == nil {
		t.Fatalf("expected PII retention policy limit violation")
	}
}

func TestStaticRetentionPolicyResolverResolveRetentionPolicy(t *testing.T) {
	t.Parallel()

	defaultPolicy := DefaultRetentionPolicy("tenant-default")
	resolver := StaticRetentionPolicyResolver{DefaultPolicy: defaultPolicy}

	resolved, err := resolver.ResolveRetentionPolicy("tenant-a")
	if err != nil {
		t.Fatalf("expected tenant policy resolution, got %v", err)
	}
	if resolved.TenantID != "tenant-a" {
		t.Fatalf("expected resolved tenant scope to be rewritten, got %q", resolved.TenantID)
	}
}

func TestInMemoryArtifactStoreEnforceRetention(t *testing.T) {
	t.Parallel()

	store := NewInMemoryArtifactStore()
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-expired-pii",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		PayloadClass: eventabi.PayloadPII,
		RecordedAtMS: 1_000,
	})
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-retained-metadata",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-2",
		PayloadClass: eventabi.PayloadMetadata,
		RecordedAtMS: 9_000,
	})
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-other-tenant",
		TenantID:     "tenant-b",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		PayloadClass: eventabi.PayloadPII,
		RecordedAtMS: 1_000,
	})

	policy := DefaultRetentionPolicy("tenant-a")
	policy.MaxRetentionByClassMS[eventabi.PayloadPII] = 500
	policy.MaxRetentionByClassMS[eventabi.PayloadMetadata] = 5_000

	result, err := store.EnforceRetention(policy, 10_000)
	if err != nil {
		t.Fatalf("expected retention enforcement to succeed, got %v", err)
	}
	if result.EvaluatedArtifacts != 2 {
		t.Fatalf("expected 2 evaluated artifacts, got %+v", result)
	}
	if result.ExpiredArtifacts != 1 || result.DeletedArtifacts != 1 {
		t.Fatalf("expected one expired/deleted artifact, got %+v", result)
	}

	records := store.Snapshot()
	if len(records) != 2 {
		t.Fatalf("expected 2 retained records, got %d", len(records))
	}
	if !hasArtifact(records, "artifact-retained-metadata") || !hasArtifact(records, "artifact-other-tenant") {
		t.Fatalf("expected retained artifacts after sweep, got %+v", records)
	}
}

func TestInMemoryArtifactStoreDeleteHardDelete(t *testing.T) {
	t.Parallel()

	store := NewInMemoryArtifactStore()
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-turn-1-a",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		PayloadClass: eventabi.PayloadMetadata,
		RecordedAtMS: 100,
	})
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-turn-1-b",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		PayloadClass: eventabi.PayloadTextRaw,
		RecordedAtMS: 100,
	})
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-other-turn",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-2",
		PayloadClass: eventabi.PayloadMetadata,
		RecordedAtMS: 100,
	})

	result, err := store.Delete(DeletionRequest{
		Scope: DeletionScope{
			TenantID: "tenant-a",
			TurnID:   "turn-1",
		},
		Mode:          DeletionModeHardDelete,
		RequestedBy:   "operator-1",
		RequestedAtMS: 1_000,
	})
	if err != nil {
		t.Fatalf("expected hard delete request to succeed, got %v", err)
	}
	if result.MatchedArtifacts != 2 || result.DeletedArtifacts != 2 {
		t.Fatalf("expected two matched/deleted artifacts, got %+v", result)
	}

	records := store.Snapshot()
	if len(records) != 1 || records[0].ArtifactID != "artifact-other-turn" {
		t.Fatalf("unexpected records after hard delete: %+v", records)
	}
}

func TestInMemoryArtifactStoreDeleteCryptoInaccessible(t *testing.T) {
	t.Parallel()

	store := NewInMemoryArtifactStore()
	mustAddArtifact(t, store, ReplayArtifactRecord{
		ArtifactID:   "artifact-crypto",
		TenantID:     "tenant-a",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		PayloadClass: eventabi.PayloadMetadata,
		RecordedAtMS: 100,
	})

	result, err := store.Delete(DeletionRequest{
		Scope: DeletionScope{
			TenantID:  "tenant-a",
			SessionID: "session-1",
		},
		Mode:          DeletionModeCryptoInaccessible,
		RequestedBy:   "operator-1",
		RequestedAtMS: 1_000,
	})
	if err != nil {
		t.Fatalf("expected crypto-inaccessible delete request to succeed, got %v", err)
	}
	if result.MatchedArtifacts != 1 || result.CryptographicallyInaccessibleArtifacts != 1 {
		t.Fatalf("expected one cryptographically inaccessible artifact, got %+v", result)
	}

	if _, err := store.Read("tenant-a", "artifact-crypto"); !errors.Is(err, ErrReplayArtifactCryptographicallyInaccessible) {
		t.Fatalf("expected artifact to be cryptographically inaccessible, got %v", err)
	}

	records := store.Snapshot()
	if len(records) != 1 || records[0].State != ArtifactStateCryptographicallyInaccessible {
		t.Fatalf("unexpected records after crypto-inaccessible delete: %+v", records)
	}
}

func TestDeletionScopeValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		scope   DeletionScope
		wantErr bool
	}{
		{
			name: "valid_session_scope",
			scope: DeletionScope{
				TenantID:  "tenant-a",
				SessionID: "session-1",
			},
		},
		{
			name: "valid_turn_scope",
			scope: DeletionScope{
				TenantID: "tenant-a",
				TurnID:   "turn-1",
			},
		},
		{
			name: "missing_selector",
			scope: DeletionScope{
				TenantID: "tenant-a",
			},
			wantErr: true,
		},
		{
			name: "both_selectors",
			scope: DeletionScope{
				TenantID:  "tenant-a",
				SessionID: "session-1",
				TurnID:    "turn-1",
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.scope.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected scope validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected scope validation error: %v", err)
			}
		})
	}
}

func mustAddArtifact(t *testing.T, store *InMemoryArtifactStore, record ReplayArtifactRecord) {
	t.Helper()
	if err := store.Add(record); err != nil {
		t.Fatalf("unexpected add artifact error: %v", err)
	}
}

func hasArtifact(records []ReplayArtifactRecord, artifactID string) bool {
	for _, record := range records {
		if record.ArtifactID == artifactID {
			return true
		}
	}
	return false
}
