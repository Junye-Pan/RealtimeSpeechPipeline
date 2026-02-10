package rollout

import "fmt"

const defaultVersionResolutionSnapshot = "version-resolution/v1"

// ResolveVersionInput models CP-09 rollout resolution inputs.
type ResolveVersionInput struct {
	SessionID                string
	RequestedPipelineVersion string
	RegistryPipelineVersion  string
}

// ResolveVersionOutput is the immutable turn-start pipeline version decision.
type ResolveVersionOutput struct {
	PipelineVersion           string
	VersionResolutionSnapshot string
}

// Backend resolves version decisions from a snapshot-fed control-plane source.
type Backend interface {
	ResolvePipelineVersion(in ResolveVersionInput) (ResolveVersionOutput, error)
}

// Service resolves rollout/version references for runtime plan freezing.
type Service struct {
	DefaultVersionResolutionSnapshot string
	Backend                          Backend
}

// NewService returns the deterministic CP-09 baseline resolver.
func NewService() Service {
	return Service{DefaultVersionResolutionSnapshot: defaultVersionResolutionSnapshot}
}

// ResolvePipelineVersion deterministically resolves turn-start pipeline version and snapshot.
func (s Service) ResolvePipelineVersion(in ResolveVersionInput) (ResolveVersionOutput, error) {
	if in.SessionID == "" {
		return ResolveVersionOutput{}, fmt.Errorf("session_id is required")
	}

	if s.Backend != nil {
		out, err := s.Backend.ResolvePipelineVersion(in)
		if err != nil {
			return ResolveVersionOutput{}, fmt.Errorf("resolve pipeline version backend: %w", err)
		}
		return s.normalizeOutput(in, out)
	}

	version := in.RegistryPipelineVersion
	if version == "" {
		version = in.RequestedPipelineVersion
	}
	if version == "" {
		return ResolveVersionOutput{}, fmt.Errorf("pipeline_version is required")
	}

	return s.normalizeOutput(in, ResolveVersionOutput{PipelineVersion: version})
}

func (s Service) normalizeOutput(in ResolveVersionInput, out ResolveVersionOutput) (ResolveVersionOutput, error) {
	version := out.PipelineVersion
	if version == "" {
		version = in.RegistryPipelineVersion
	}
	if version == "" {
		version = in.RequestedPipelineVersion
	}
	if version == "" {
		return ResolveVersionOutput{}, fmt.Errorf("pipeline_version is required")
	}

	snapshot := s.DefaultVersionResolutionSnapshot
	if snapshot == "" {
		snapshot = defaultVersionResolutionSnapshot
	}
	if out.VersionResolutionSnapshot != "" {
		snapshot = out.VersionResolutionSnapshot
	}

	return ResolveVersionOutput{
		PipelineVersion:           version,
		VersionResolutionSnapshot: snapshot,
	}, nil
}
