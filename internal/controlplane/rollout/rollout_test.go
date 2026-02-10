package rollout

import (
	"errors"
	"testing"
)

func TestResolvePipelineVersion(t *testing.T) {
	t.Parallel()

	service := NewService()
	out, err := service.ResolvePipelineVersion(ResolveVersionInput{
		SessionID:                "sess-1",
		RequestedPipelineVersion: "pipeline-requested",
		RegistryPipelineVersion:  "pipeline-registry",
	})
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if out.PipelineVersion != "pipeline-registry" {
		t.Fatalf("expected registry pipeline version, got %+v", out)
	}
	if out.VersionResolutionSnapshot == "" {
		t.Fatalf("expected version resolution snapshot")
	}
}

func TestResolvePipelineVersionRequiresSessionID(t *testing.T) {
	t.Parallel()

	if _, err := NewService().ResolvePipelineVersion(ResolveVersionInput{}); err == nil {
		t.Fatalf("expected session_id validation error")
	}
}

func TestResolvePipelineVersionUsesBackendWhenConfigured(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubRolloutBackend{
		resolveFn: func(in ResolveVersionInput) (ResolveVersionOutput, error) {
			return ResolveVersionOutput{
				PipelineVersion:           "",
				VersionResolutionSnapshot: "version-resolution/backend",
			}, nil
		},
	}

	out, err := service.ResolvePipelineVersion(ResolveVersionInput{
		SessionID:                "sess-1",
		RequestedPipelineVersion: "pipeline-requested",
	})
	if err != nil {
		t.Fatalf("unexpected backend resolve error: %v", err)
	}
	if out.PipelineVersion != "pipeline-requested" || out.VersionResolutionSnapshot != "version-resolution/backend" {
		t.Fatalf("unexpected backend rollout result: %+v", out)
	}
}

func TestResolvePipelineVersionBackendError(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubRolloutBackend{
		resolveFn: func(ResolveVersionInput) (ResolveVersionOutput, error) {
			return ResolveVersionOutput{}, errors.New("backend unavailable")
		},
	}

	if _, err := service.ResolvePipelineVersion(ResolveVersionInput{
		SessionID:                "sess-1",
		RequestedPipelineVersion: "pipeline-requested",
	}); err == nil {
		t.Fatalf("expected backend error")
	}
}

type stubRolloutBackend struct {
	resolveFn func(in ResolveVersionInput) (ResolveVersionOutput, error)
}

func (s stubRolloutBackend) ResolvePipelineVersion(in ResolveVersionInput) (ResolveVersionOutput, error) {
	return s.resolveFn(in)
}
