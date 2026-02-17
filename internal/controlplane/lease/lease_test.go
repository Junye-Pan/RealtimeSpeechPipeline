package lease

import (
	"errors"
	"testing"
	"time"
)

func TestResolveDefaults(t *testing.T) {
	t.Parallel()

	out, err := NewService().Resolve(Input{SessionID: "sess-1", PipelineVersion: "pipeline-v1", RequestedAuthorityEpoch: 7})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if out.LeaseResolutionSnapshot != defaultLeaseResolutionSnapshot {
		t.Fatalf("unexpected lease snapshot: %+v", out)
	}
	if out.AuthorityEpoch != 7 {
		t.Fatalf("expected requested authority epoch fallback, got %+v", out)
	}
	if out.AuthorityEpochValid == nil || !*out.AuthorityEpochValid {
		t.Fatalf("expected authority epoch valid default, got %+v", out)
	}
	if out.AuthorityAuthorized == nil || !*out.AuthorityAuthorized {
		t.Fatalf("expected authority authorized default, got %+v", out)
	}
	if out.Reason != ReasonLeaseAuthorized {
		t.Fatalf("unexpected default reason: %+v", out)
	}
	if out.LeaseTokenID == "" || out.LeaseExpiresAtUTC == "" {
		t.Fatalf("expected lease token metadata defaults, got %+v", out)
	}
	if _, err := time.Parse(time.RFC3339, out.LeaseExpiresAtUTC); err != nil {
		t.Fatalf("expected RFC3339 lease expiry, got %q (%v)", out.LeaseExpiresAtUTC, err)
	}
}

func TestResolveUsesBackendAndReasonDefaults(t *testing.T) {
	t.Parallel()

	epochInvalid := false
	authorized := true
	service := NewService()
	service.Backend = stubBackend{resolveFn: func(Input) (Output, error) {
		return Output{
			LeaseResolutionSnapshot: "lease-resolution/backend",
			AuthorityEpoch:          11,
			AuthorityEpochValid:     &epochInvalid,
			AuthorityAuthorized:     &authorized,
		}, nil
	}}

	out, err := service.Resolve(Input{SessionID: "sess-1", RequestedAuthorityEpoch: 8})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if out.LeaseResolutionSnapshot != "lease-resolution/backend" || out.AuthorityEpoch != 11 {
		t.Fatalf("unexpected backend output: %+v", out)
	}
	if out.AuthorityEpochValid == nil || *out.AuthorityEpochValid {
		t.Fatalf("expected stale epoch state from backend, got %+v", out)
	}
	if out.Reason != ReasonLeaseStaleEpoch {
		t.Fatalf("expected stale epoch default reason, got %+v", out)
	}
	if out.LeaseTokenID == "" || out.LeaseExpiresAtUTC == "" {
		t.Fatalf("expected lease token metadata defaults when backend omits values, got %+v", out)
	}
}

func TestResolveUsesBackendLeaseTokenMetadata(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubBackend{resolveFn: func(Input) (Output, error) {
		valid := true
		authorized := true
		return Output{
			LeaseResolutionSnapshot: "lease-resolution/backend",
			AuthorityEpoch:          9,
			AuthorityEpochValid:     &valid,
			AuthorityAuthorized:     &authorized,
			LeaseTokenID:            "lease-token-backend",
			LeaseExpiresAtUTC:       "2026-02-17T01:00:00Z",
		}, nil
	}}

	out, err := service.Resolve(Input{SessionID: "sess-1", RequestedAuthorityEpoch: 9})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if out.LeaseTokenID != "lease-token-backend" || out.LeaseExpiresAtUTC != "2026-02-17T01:00:00Z" {
		t.Fatalf("expected backend lease token metadata, got %+v", out)
	}
}

func TestResolveTreatsExpiredLeaseTokenAsDeauthorized(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 1, 0, 0, 0, time.UTC)
	service := NewService()
	service.Now = func() time.Time { return now }
	service.Backend = stubBackend{resolveFn: func(Input) (Output, error) {
		valid := true
		authorized := true
		return Output{
			LeaseResolutionSnapshot: "lease-resolution/backend",
			AuthorityEpoch:          9,
			AuthorityEpochValid:     &valid,
			AuthorityAuthorized:     &authorized,
			LeaseTokenID:            "lease-token-expired",
			LeaseExpiresAtUTC:       "2026-02-17T00:59:00Z",
		}, nil
	}}

	out, err := service.Resolve(Input{SessionID: "sess-1", RequestedAuthorityEpoch: 9})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if out.AuthorityAuthorized == nil || *out.AuthorityAuthorized {
		t.Fatalf("expected expired lease token to deauthorize lease output, got %+v", out)
	}
	if out.Reason != ReasonLeaseDeauthorized {
		t.Fatalf("expected deauthorized reason for expired lease token, got %+v", out)
	}
}

func TestResolveInvalidLeaseExpiryFallsBackToDefaultTTL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.February, 17, 1, 5, 0, 0, time.UTC)
	service := NewService()
	service.Now = func() time.Time { return now }
	service.DefaultLeaseTokenTTL = 90 * time.Second
	service.Backend = stubBackend{resolveFn: func(Input) (Output, error) {
		valid := true
		authorized := true
		return Output{
			LeaseResolutionSnapshot: "lease-resolution/backend",
			AuthorityEpoch:          9,
			AuthorityEpochValid:     &valid,
			AuthorityAuthorized:     &authorized,
			LeaseTokenID:            "lease-token-backend",
			LeaseExpiresAtUTC:       "not-a-timestamp",
		}, nil
	}}

	out, err := service.Resolve(Input{SessionID: "sess-1", RequestedAuthorityEpoch: 9})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	expectedExpiry := now.Add(90 * time.Second).UTC().Format(time.RFC3339)
	if out.LeaseExpiresAtUTC != expectedExpiry {
		t.Fatalf("expected fallback expiry %q, got %+v", expectedExpiry, out)
	}
	if out.AuthorityAuthorized == nil || !*out.AuthorityAuthorized {
		t.Fatalf("expected lease to remain authorized after invalid backend expiry normalization, got %+v", out)
	}
}

func TestResolveRequiresSessionAndValidEpoch(t *testing.T) {
	t.Parallel()

	_, err := NewService().Resolve(Input{})
	if err == nil {
		t.Fatalf("expected missing session error")
	}

	_, err = NewService().Resolve(Input{SessionID: "sess-1", RequestedAuthorityEpoch: -1})
	if err == nil {
		t.Fatalf("expected invalid epoch error")
	}
}

func TestResolveWrapsBackendErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("backend unavailable")
	service := NewService()
	service.Backend = stubBackend{resolveFn: func(Input) (Output, error) {
		return Output{}, sentinel
	}}

	_, err := service.Resolve(Input{SessionID: "sess-1"})
	if err == nil {
		t.Fatalf("expected backend error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected backend error wrapping, got %v", err)
	}
}

type stubBackend struct {
	resolveFn func(in Input) (Output, error)
}

func (s stubBackend) Resolve(in Input) (Output, error) {
	return s.resolveFn(in)
}
