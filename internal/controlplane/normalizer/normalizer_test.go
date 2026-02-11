package normalizer

import (
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		in            Input
		want          Output
		wantErrSubstr string
	}{
		{
			name: "applies defaults",
			in: Input{
				Record: registry.PipelineRecord{
					PipelineVersion: "pipeline-v2",
				},
			},
			want: Output{
				PipelineVersion:    "pipeline-v2",
				GraphDefinitionRef: registry.DefaultGraphDefinitionRef,
				ExecutionProfile:   registry.DefaultExecutionProfile,
			},
		},
		{
			name: "accepts explicit simple profile",
			in: Input{
				Record: registry.PipelineRecord{
					PipelineVersion:    "pipeline-v3",
					GraphDefinitionRef: "graph/custom",
					ExecutionProfile:   "simple",
				},
			},
			want: Output{
				PipelineVersion:    "pipeline-v3",
				GraphDefinitionRef: "graph/custom",
				ExecutionProfile:   "simple",
			},
		},
		{
			name:          "rejects missing pipeline version",
			in:            Input{},
			wantErrSubstr: "pipeline_version is required",
		},
		{
			name: "rejects unsupported profile",
			in: Input{
				Record: registry.PipelineRecord{
					PipelineVersion:    "pipeline-v4",
					GraphDefinitionRef: "graph/custom",
					ExecutionProfile:   "advanced",
				},
			},
			wantErrSubstr: "unsupported in MVP",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := (Service{}).Normalize(tc.in)
			if tc.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErrSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected normalize error: %v", err)
			}
			if out != tc.want {
				t.Fatalf("unexpected normalize output: got %+v want %+v", out, tc.want)
			}
		})
	}
}
