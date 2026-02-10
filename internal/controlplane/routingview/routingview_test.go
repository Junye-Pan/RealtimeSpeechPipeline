package routingview

import (
	"errors"
	"testing"
)

func TestGetSnapshot(t *testing.T) {
	t.Parallel()

	out, err := NewService().GetSnapshot(Input{
		SessionID:      "sess-1",
		AuthorityEpoch: 7,
	})
	if err != nil {
		t.Fatalf("unexpected get snapshot error: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("expected valid snapshot, got %v", err)
	}
}

func TestGetSnapshotRejectsNegativeEpoch(t *testing.T) {
	t.Parallel()

	_, err := NewService().GetSnapshot(Input{
		SessionID:      "sess-1",
		AuthorityEpoch: -1,
	})
	if err == nil {
		t.Fatalf("expected authority_epoch validation error")
	}
}

func TestGetSnapshotUsesBackendWhenConfigured(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubRoutingBackend{
		resolveFn: func(Input) (Snapshot, error) {
			return Snapshot{
				RoutingViewSnapshot:      "routing-view/backend",
				AdmissionPolicySnapshot:  "admission-policy/backend",
				ABICompatibilitySnapshot: "abi-compat/backend",
			}, nil
		},
	}

	out, err := service.GetSnapshot(Input{
		SessionID:      "sess-1",
		AuthorityEpoch: 7,
	})
	if err != nil {
		t.Fatalf("unexpected backend get snapshot error: %v", err)
	}
	if out.RoutingViewSnapshot != "routing-view/backend" || out.AdmissionPolicySnapshot != "admission-policy/backend" || out.ABICompatibilitySnapshot != "abi-compat/backend" {
		t.Fatalf("unexpected backend snapshot: %+v", out)
	}
}

func TestGetSnapshotBackendError(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubRoutingBackend{
		resolveFn: func(Input) (Snapshot, error) {
			return Snapshot{}, errors.New("backend unavailable")
		},
	}

	if _, err := service.GetSnapshot(Input{
		SessionID:      "sess-1",
		AuthorityEpoch: 7,
	}); err == nil {
		t.Fatalf("expected backend error")
	}
}

type stubRoutingBackend struct {
	resolveFn func(in Input) (Snapshot, error)
}

func (s stubRoutingBackend) GetSnapshot(in Input) (Snapshot, error) {
	return s.resolveFn(in)
}
