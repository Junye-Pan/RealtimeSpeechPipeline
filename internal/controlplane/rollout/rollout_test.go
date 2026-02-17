package rollout

import (
	"errors"
	"fmt"
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

func TestResolvePipelineVersionUsesAllowlistCanary(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Policy = Policy{
		BasePipelineVersion:   "pipeline-v1",
		CanaryPipelineVersion: "pipeline-v2",
		CanaryPercentage:      0,
		TenantAllowlist:       []string{"tenant-gold"},
	}

	out, err := service.ResolvePipelineVersion(ResolveVersionInput{
		SessionID:                "sess-1",
		TenantID:                 "tenant-gold",
		RequestedPipelineVersion: "pipeline-requested",
		RegistryPipelineVersion:  "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if out.PipelineVersion != "pipeline-v2" {
		t.Fatalf("expected allowlisted tenant to resolve canary, got %+v", out)
	}
}

func TestResolvePipelineVersionUsesPercentageCanaryDeterministically(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Policy = Policy{
		BasePipelineVersion:   "pipeline-v1",
		CanaryPipelineVersion: "pipeline-v2",
		CanaryPercentage:      50,
	}

	canarySessionID := findSessionForCanaryBucket(t, 50, true)
	stableSessionID := findSessionForCanaryBucket(t, 50, false)

	canaryOut, err := service.ResolvePipelineVersion(ResolveVersionInput{
		SessionID:                canarySessionID,
		RequestedPipelineVersion: "pipeline-requested",
		RegistryPipelineVersion:  "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("unexpected canary resolve error: %v", err)
	}
	if canaryOut.PipelineVersion != "pipeline-v2" {
		t.Fatalf("expected canary session to resolve pipeline-v2, got %+v", canaryOut)
	}

	stableOut, err := service.ResolvePipelineVersion(ResolveVersionInput{
		SessionID:                stableSessionID,
		RequestedPipelineVersion: "pipeline-requested",
		RegistryPipelineVersion:  "pipeline-v1",
	})
	if err != nil {
		t.Fatalf("unexpected stable resolve error: %v", err)
	}
	if stableOut.PipelineVersion != "pipeline-v1" {
		t.Fatalf("expected stable session to resolve pipeline-v1, got %+v", stableOut)
	}
}

func TestResolvePipelineVersionInvalidPolicy(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Policy = Policy{
		BasePipelineVersion:   "pipeline-v1",
		CanaryPipelineVersion: "",
		CanaryPercentage:      10,
	}

	_, err := service.ResolvePipelineVersion(ResolveVersionInput{
		SessionID:                "sess-1",
		RequestedPipelineVersion: "pipeline-requested",
		RegistryPipelineVersion:  "pipeline-v1",
	})
	if err == nil {
		t.Fatalf("expected invalid policy error")
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

func TestShouldRouteToCanaryDeterministicForSameKey(t *testing.T) {
	t.Parallel()

	first := ShouldRouteToCanary("sess-abc", "tenant-1", 10)
	second := ShouldRouteToCanary("sess-abc", "tenant-1", 10)
	if first != second {
		t.Fatalf("expected deterministic canary routing decision")
	}
}

func findSessionForCanaryBucket(t *testing.T, pct int, wantCanary bool) string {
	t.Helper()
	for i := 0; i < 10000; i++ {
		sessionID := fmt.Sprintf("sess-%d", i)
		if ShouldRouteToCanary(sessionID, "", pct) == wantCanary {
			return sessionID
		}
	}
	t.Fatalf("failed to find session for pct=%d wantCanary=%t", pct, wantCanary)
	return ""
}

type stubRolloutBackend struct {
	resolveFn func(in ResolveVersionInput) (ResolveVersionOutput, error)
}

func (s stubRolloutBackend) ResolvePipelineVersion(in ResolveVersionInput) (ResolveVersionOutput, error) {
	return s.resolveFn(in)
}
