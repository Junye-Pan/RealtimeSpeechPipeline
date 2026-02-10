package normalizer

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
)

// Input is the CP-02 normalization input.
type Input struct {
	Record registry.PipelineRecord
}

// Output is the normalized turn-start artifact identity.
type Output struct {
	PipelineVersion    string
	GraphDefinitionRef string
	ExecutionProfile   string
}

// Service applies deterministic CP-02 defaulting.
type Service struct{}

// Normalize applies baseline defaults to a registry pipeline record.
func (Service) Normalize(in Input) (Output, error) {
	record := in.Record
	if record.PipelineVersion == "" {
		return Output{}, fmt.Errorf("pipeline_version is required")
	}
	if record.GraphDefinitionRef == "" {
		record.GraphDefinitionRef = registry.DefaultGraphDefinitionRef
	}
	if record.ExecutionProfile == "" {
		record.ExecutionProfile = registry.DefaultExecutionProfile
	}
	if err := record.Validate(); err != nil {
		return Output{}, err
	}

	return Output{
		PipelineVersion:    record.PipelineVersion,
		GraphDefinitionRef: record.GraphDefinitionRef,
		ExecutionProfile:   record.ExecutionProfile,
	}, nil
}
