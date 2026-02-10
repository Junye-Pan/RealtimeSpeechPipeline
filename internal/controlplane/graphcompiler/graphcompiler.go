package graphcompiler

import (
	"fmt"
	"strings"
)

const defaultGraphCompilationSnapshot = "graph-compiler/v1"

const (
	// ErrorCodeInvalidRef classifies malformed or missing graph references.
	ErrorCodeInvalidRef = "GRAPH_COMPILE_INVALID_REF"
	// ErrorCodeUnsupportedVersion classifies unsupported pipeline versions.
	ErrorCodeUnsupportedVersion = "GRAPH_COMPILE_UNSUPPORTED_VERSION"
	// ErrorCodeEmptyInput classifies missing mandatory compile input fields.
	ErrorCodeEmptyInput = "GRAPH_COMPILE_EMPTY_INPUT"
)

// Input captures deterministic CP-03 graph compilation context.
type Input struct {
	PipelineVersion    string
	GraphDefinitionRef string
	ExecutionProfile   string
}

// Output is the CP-03 compiled graph artifact reference.
type Output struct {
	GraphDefinitionRef       string
	GraphCompilationSnapshot string
	GraphFingerprint         string
}

// Backend resolves graph compilation output from a snapshot-fed control-plane source.
type Backend interface {
	Compile(in Input) (Output, error)
}

// Service compiles graph references for runtime plan materialization.
type Service struct {
	DefaultGraphCompilationSnapshot string
	SupportedPipelineVersions       map[string]struct{}
	Backend                         Backend
}

// NewService returns deterministic CP-03 baseline compiler defaults.
func NewService() Service {
	return Service{DefaultGraphCompilationSnapshot: defaultGraphCompilationSnapshot}
}

// Compile resolves deterministic graph compilation output for turn-start.
func (s Service) Compile(in Input) (Output, error) {
	if strings.TrimSpace(in.PipelineVersion) == "" {
		return Output{}, fmt.Errorf("%s: pipeline_version is required", ErrorCodeEmptyInput)
	}
	if strings.TrimSpace(in.GraphDefinitionRef) == "" {
		return Output{}, fmt.Errorf("%s: graph_definition_ref is required", ErrorCodeInvalidRef)
	}
	if len(s.SupportedPipelineVersions) > 0 {
		if _, ok := s.SupportedPipelineVersions[in.PipelineVersion]; !ok {
			return Output{}, fmt.Errorf("%s: pipeline_version=%s", ErrorCodeUnsupportedVersion, in.PipelineVersion)
		}
	}

	if s.Backend != nil {
		out, err := s.Backend.Compile(in)
		if err != nil {
			return Output{}, fmt.Errorf("compile graph backend: %w", err)
		}
		return s.normalizeOutput(in, out), nil
	}

	return s.normalizeOutput(in, Output{}), nil
}

func (s Service) normalizeOutput(in Input, out Output) Output {
	graphRef := strings.TrimSpace(out.GraphDefinitionRef)
	if graphRef == "" {
		graphRef = strings.TrimSpace(in.GraphDefinitionRef)
	}

	snapshot := strings.TrimSpace(s.DefaultGraphCompilationSnapshot)
	if snapshot == "" {
		snapshot = defaultGraphCompilationSnapshot
	}
	if custom := strings.TrimSpace(out.GraphCompilationSnapshot); custom != "" {
		snapshot = custom
	}

	fingerprint := strings.TrimSpace(out.GraphFingerprint)
	if fingerprint == "" {
		fingerprint = buildFingerprint(in.PipelineVersion, graphRef, in.ExecutionProfile)
	}

	return Output{
		GraphDefinitionRef:       graphRef,
		GraphCompilationSnapshot: snapshot,
		GraphFingerprint:         fingerprint,
	}
}

func buildFingerprint(pipelineVersion, graphDefinitionRef, executionProfile string) string {
	pipeline := strings.TrimSpace(pipelineVersion)
	graph := strings.TrimSpace(graphDefinitionRef)
	profile := strings.TrimSpace(executionProfile)
	if profile == "" {
		profile = "default"
	}
	return fmt.Sprintf("graph-fingerprint/%s/%s/%s", pipeline, graph, profile)
}
