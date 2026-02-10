package registry

import (
	"errors"
	"testing"
)

func TestResolvePipelineRecord(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Records = map[string]PipelineRecord{
		"pipeline-canary": {
			GraphDefinitionRef: "graph/canary",
			ExecutionProfile:   "simple",
		},
	}

	base, err := service.ResolvePipelineRecord("")
	if err != nil {
		t.Fatalf("unexpected default resolve error: %v", err)
	}
	if base.PipelineVersion != DefaultPipelineVersion {
		t.Fatalf("expected default pipeline version %q, got %q", DefaultPipelineVersion, base.PipelineVersion)
	}
	if base.GraphDefinitionRef != DefaultGraphDefinitionRef || base.ExecutionProfile != DefaultExecutionProfile {
		t.Fatalf("unexpected default record: %+v", base)
	}

	canary, err := service.ResolvePipelineRecord("pipeline-canary")
	if err != nil {
		t.Fatalf("unexpected canary resolve error: %v", err)
	}
	if canary.PipelineVersion != "pipeline-canary" {
		t.Fatalf("expected canary pipeline version, got %+v", canary)
	}
	if canary.GraphDefinitionRef != "graph/canary" {
		t.Fatalf("expected canary graph ref, got %+v", canary)
	}
}

func TestResolvePipelineRecordAppliesBaselineDefaults(t *testing.T) {
	t.Parallel()

	service := Service{
		DefaultRecord: PipelineRecord{
			PipelineVersion:    "",
			GraphDefinitionRef: "",
			ExecutionProfile:   "",
		},
	}
	record, err := service.ResolvePipelineRecord("")
	if err != nil {
		t.Fatalf("expected baseline defaults to resolve empty default record, got %v", err)
	}
	if record.PipelineVersion != DefaultPipelineVersion ||
		record.GraphDefinitionRef != DefaultGraphDefinitionRef ||
		record.ExecutionProfile != DefaultExecutionProfile {
		t.Fatalf("expected baseline defaults, got %+v", record)
	}
}

func TestResolvePipelineRecordUsesBackendWhenConfigured(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubRegistryBackend{
		resolveFn: func(version string) (PipelineRecord, error) {
			return PipelineRecord{
				PipelineVersion:    version,
				GraphDefinitionRef: "graph/backend",
				ExecutionProfile:   "",
			}, nil
		},
	}

	record, err := service.ResolvePipelineRecord("pipeline-backend")
	if err != nil {
		t.Fatalf("unexpected backend resolve error: %v", err)
	}
	if record.PipelineVersion != "pipeline-backend" || record.GraphDefinitionRef != "graph/backend" || record.ExecutionProfile != DefaultExecutionProfile {
		t.Fatalf("unexpected backend-resolved record: %+v", record)
	}
}

func TestResolvePipelineRecordBackendError(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubRegistryBackend{
		resolveFn: func(_ string) (PipelineRecord, error) {
			return PipelineRecord{}, errors.New("backend unavailable")
		},
	}

	if _, err := service.ResolvePipelineRecord("pipeline-v1"); err == nil {
		t.Fatalf("expected backend error")
	}
}

type stubRegistryBackend struct {
	resolveFn func(version string) (PipelineRecord, error)
}

func (s stubRegistryBackend) ResolvePipelineRecord(version string) (PipelineRecord, error) {
	return s.resolveFn(version)
}
