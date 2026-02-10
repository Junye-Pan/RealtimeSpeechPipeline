package normalizer

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
)

func TestNormalizeDefaults(t *testing.T) {
	t.Parallel()

	out, err := (Service{}).Normalize(Input{
		Record: registry.PipelineRecord{
			PipelineVersion: "pipeline-v2",
		},
	})
	if err != nil {
		t.Fatalf("unexpected normalize error: %v", err)
	}
	if out.PipelineVersion != "pipeline-v2" {
		t.Fatalf("unexpected pipeline version: %+v", out)
	}
	if out.GraphDefinitionRef != registry.DefaultGraphDefinitionRef {
		t.Fatalf("expected default graph ref, got %+v", out)
	}
	if out.ExecutionProfile != registry.DefaultExecutionProfile {
		t.Fatalf("expected default execution profile, got %+v", out)
	}
}

func TestNormalizeRequiresPipelineVersion(t *testing.T) {
	t.Parallel()

	if _, err := (Service{}).Normalize(Input{}); err == nil {
		t.Fatalf("expected pipeline_version error")
	}
}
