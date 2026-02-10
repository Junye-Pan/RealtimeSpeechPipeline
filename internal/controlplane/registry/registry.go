package registry

import "fmt"

const (
	// DefaultPipelineVersion is the baseline pipeline version resolved by CP-01.
	DefaultPipelineVersion = "pipeline-v1"
	// DefaultGraphDefinitionRef is the baseline graph reference resolved by CP-01.
	DefaultGraphDefinitionRef = "graph/default"
	// DefaultExecutionProfile is the baseline execution profile resolved by CP-01.
	DefaultExecutionProfile = "simple"
)

// PipelineRecord is the registry-owned pipeline artifact pointer set.
type PipelineRecord struct {
	PipelineVersion    string
	GraphDefinitionRef string
	ExecutionProfile   string
}

// Validate enforces baseline CP-01 contract requirements.
func (r PipelineRecord) Validate() error {
	if r.PipelineVersion == "" {
		return fmt.Errorf("pipeline_version is required")
	}
	if r.GraphDefinitionRef == "" {
		return fmt.Errorf("graph_definition_ref is required")
	}
	if r.ExecutionProfile == "" {
		return fmt.Errorf("execution_profile is required")
	}
	return nil
}

// Backend resolves pipeline records from a snapshot-fed control-plane source.
type Backend interface {
	ResolvePipelineRecord(pipelineVersion string) (PipelineRecord, error)
}

// Service resolves immutable pipeline references for runtime turn-start consumption.
type Service struct {
	DefaultRecord PipelineRecord
	Records       map[string]PipelineRecord
	Backend       Backend
}

// NewService returns the deterministic CP-01 baseline resolver.
func NewService() Service {
	return Service{
		DefaultRecord: PipelineRecord{
			PipelineVersion:    DefaultPipelineVersion,
			GraphDefinitionRef: DefaultGraphDefinitionRef,
			ExecutionProfile:   DefaultExecutionProfile,
		},
	}
}

// ResolvePipelineRecord resolves the pipeline artifact record for a version.
func (s Service) ResolvePipelineRecord(pipelineVersion string) (PipelineRecord, error) {
	defaultRecord := s.DefaultRecord
	if defaultRecord.PipelineVersion == "" {
		defaultRecord.PipelineVersion = DefaultPipelineVersion
	}
	if defaultRecord.GraphDefinitionRef == "" {
		defaultRecord.GraphDefinitionRef = DefaultGraphDefinitionRef
	}
	if defaultRecord.ExecutionProfile == "" {
		defaultRecord.ExecutionProfile = DefaultExecutionProfile
	}
	if err := defaultRecord.Validate(); err != nil {
		return PipelineRecord{}, err
	}

	version := pipelineVersion
	if version == "" {
		version = defaultRecord.PipelineVersion
	}

	if s.Backend != nil {
		record, err := s.Backend.ResolvePipelineRecord(version)
		if err != nil {
			return PipelineRecord{}, fmt.Errorf("resolve pipeline record backend: %w", err)
		}
		return applyDefaults(record, version, defaultRecord)
	}

	if record, ok := s.Records[version]; ok {
		return applyDefaults(record, version, defaultRecord)
	}

	return applyDefaults(PipelineRecord{}, version, defaultRecord)
}

func applyDefaults(record PipelineRecord, version string, defaults PipelineRecord) (PipelineRecord, error) {
	if record.PipelineVersion == "" {
		record.PipelineVersion = version
	}
	if record.GraphDefinitionRef == "" {
		record.GraphDefinitionRef = defaults.GraphDefinitionRef
	}
	if record.ExecutionProfile == "" {
		record.ExecutionProfile = defaults.ExecutionProfile
	}
	if err := record.Validate(); err != nil {
		return PipelineRecord{}, err
	}
	return record, nil
}
