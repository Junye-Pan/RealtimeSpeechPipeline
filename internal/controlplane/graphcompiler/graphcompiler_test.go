package graphcompiler

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCompileDefaults(t *testing.T) {
	t.Parallel()

	service := NewService()
	out, err := service.Compile(Input{
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
	})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if out.GraphDefinitionRef != "graph/default" {
		t.Fatalf("unexpected graph definition ref: %+v", out)
	}
	if out.GraphCompilationSnapshot != defaultGraphCompilationSnapshot {
		t.Fatalf("unexpected graph compilation snapshot: %+v", out)
	}
	if out.GraphFingerprint == "" {
		t.Fatalf("expected non-empty graph fingerprint")
	}
}

func TestCompileUsesBackendOutput(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.Backend = stubBackend{
		compileFn: func(Input) (Output, error) {
			return Output{
				GraphDefinitionRef:       "graph/backend",
				GraphCompilationSnapshot: "graph-compiler/backend",
				GraphFingerprint:         "graph-fingerprint/backend",
			}, nil
		},
	}

	out, err := service.Compile(Input{
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
	})
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	if out.GraphDefinitionRef != "graph/backend" || out.GraphCompilationSnapshot != "graph-compiler/backend" || out.GraphFingerprint != "graph-fingerprint/backend" {
		t.Fatalf("unexpected backend output: %+v", out)
	}
}

func TestCompileRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	service := NewService()
	service.SupportedPipelineVersions = map[string]struct{}{"pipeline-v1": {}}
	_, err := service.Compile(Input{
		PipelineVersion:    "pipeline-v2",
		GraphDefinitionRef: "graph/default",
	})
	if err == nil {
		t.Fatalf("expected unsupported version error")
	}
	if !strings.Contains(err.Error(), ErrorCodeUnsupportedVersion) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileValidatesRequiredInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input Input
		code  string
	}{
		{
			name: "missing pipeline",
			input: Input{
				GraphDefinitionRef: "graph/default",
			},
			code: ErrorCodeEmptyInput,
		},
		{
			name: "missing graph ref",
			input: Input{
				PipelineVersion: "pipeline-v1",
			},
			code: ErrorCodeInvalidRef,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewService().Compile(tt.input)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.code) {
				t.Fatalf("expected error code %s, got %v", tt.code, err)
			}
		})
	}
}

func TestCompileIsDeterministic(t *testing.T) {
	t.Parallel()

	service := NewService()
	in := Input{
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
	}
	first, err := service.Compile(in)
	if err != nil {
		t.Fatalf("compile first run failed: %v", err)
	}
	second, err := service.Compile(in)
	if err != nil {
		t.Fatalf("compile second run failed: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic compile outputs: first=%+v second=%+v", first, second)
	}
}

func TestCompileWrapsBackendErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("backend unavailable")
	service := NewService()
	service.Backend = stubBackend{
		compileFn: func(Input) (Output, error) {
			return Output{}, sentinel
		},
	}
	_, err := service.Compile(Input{
		PipelineVersion:    "pipeline-v1",
		GraphDefinitionRef: "graph/default",
	})
	if err == nil {
		t.Fatalf("expected backend error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected backend error wrapping, got %v", err)
	}
}

type stubBackend struct {
	compileFn func(in Input) (Output, error)
}

func (s stubBackend) Compile(in Input) (Output, error) {
	return s.compileFn(in)
}
