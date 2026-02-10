package lease

import (
	"errors"
	"testing"
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
