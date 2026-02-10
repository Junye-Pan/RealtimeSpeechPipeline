package providerhealth

import (
	"errors"
	"testing"
)

func TestGetSnapshot(t *testing.T) {
	t.Parallel()

	out, err := NewService().GetSnapshot(Input{Scope: "tenant-a"})
	if err != nil {
		t.Fatalf("unexpected provider health snapshot error: %v", err)
	}
	if out.ProviderHealthSnapshot == "" {
		t.Fatalf("expected non-empty provider health snapshot")
	}
}

func TestGetSnapshotRequiresScope(t *testing.T) {
	t.Parallel()

	if _, err := NewService().GetSnapshot(Input{}); err == nil {
		t.Fatalf("expected scope validation error")
	}
}

func TestGetSnapshotUsesBackendWhenConfigured(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubProviderHealthBackend{
		getFn: func(Input) (Output, error) {
			return Output{ProviderHealthSnapshot: "provider-health/backend"}, nil
		},
	}

	out, err := service.GetSnapshot(Input{Scope: "tenant-a"})
	if err != nil {
		t.Fatalf("unexpected backend snapshot error: %v", err)
	}
	if out.ProviderHealthSnapshot != "provider-health/backend" {
		t.Fatalf("unexpected backend snapshot output: %+v", out)
	}
}

func TestGetSnapshotBackendError(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubProviderHealthBackend{
		getFn: func(Input) (Output, error) {
			return Output{}, errors.New("backend unavailable")
		},
	}

	if _, err := service.GetSnapshot(Input{Scope: "tenant-a"}); err == nil {
		t.Fatalf("expected backend error")
	}
}

type stubProviderHealthBackend struct {
	getFn func(in Input) (Output, error)
}

func (s stubProviderHealthBackend) GetSnapshot(in Input) (Output, error) {
	return s.getFn(in)
}
