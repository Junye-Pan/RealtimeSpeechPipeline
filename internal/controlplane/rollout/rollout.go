package rollout

import (
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
)

const defaultVersionResolutionSnapshot = "version-resolution/v1"

// ResolveVersionInput models CP-09 rollout resolution inputs.
type ResolveVersionInput struct {
	TenantID                 string
	SessionID                string
	RequestedPipelineVersion string
	RegistryPipelineVersion  string
}

// ResolveVersionOutput is the immutable turn-start pipeline version decision.
type ResolveVersionOutput struct {
	PipelineVersion           string
	VersionResolutionSnapshot string
}

// Policy captures deterministic rollout controls used when backend output does not
// directly resolve an effective pipeline version.
type Policy struct {
	BasePipelineVersion   string
	CanaryPipelineVersion string
	CanaryPercentage      int
	TenantAllowlist       []string
}

// Validate enforces deterministic rollout-policy constraints.
func (p Policy) Validate() error {
	if p.CanaryPercentage < 0 || p.CanaryPercentage > 100 {
		return fmt.Errorf("canary_percentage must be within [0,100]")
	}
	hasCanaryRouting := p.CanaryPercentage > 0 || len(p.TenantAllowlist) > 0
	if hasCanaryRouting && strings.TrimSpace(p.CanaryPipelineVersion) == "" {
		return fmt.Errorf("canary_pipeline_version is required when canary routing is enabled")
	}
	for _, tenantID := range p.TenantAllowlist {
		if strings.TrimSpace(tenantID) == "" {
			return fmt.Errorf("tenant_allowlist cannot contain empty tenant ids")
		}
	}
	return nil
}

// Backend resolves version decisions from a snapshot-fed control-plane source.
type Backend interface {
	ResolvePipelineVersion(in ResolveVersionInput) (ResolveVersionOutput, error)
}

// Service resolves rollout/version references for runtime plan freezing.
type Service struct {
	DefaultVersionResolutionSnapshot string
	Policy                           Policy
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
	return s.normalizeOutput(in, ResolveVersionOutput{})
}

func (s Service) normalizeOutput(in ResolveVersionInput, out ResolveVersionOutput) (ResolveVersionOutput, error) {
	version, err := s.resolvePipelineVersion(in, out.PipelineVersion)
	if err != nil {
		return ResolveVersionOutput{}, err
	}
	version = strings.TrimSpace(version)
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

func (s Service) resolvePipelineVersion(in ResolveVersionInput, backendVersion string) (string, error) {
	if explicit := strings.TrimSpace(backendVersion); explicit != "" {
		return explicit, nil
	}

	policy := normalizePolicy(s.Policy)
	if err := policy.Validate(); err != nil {
		return "", err
	}

	baseVersion := strings.TrimSpace(policy.BasePipelineVersion)
	if baseVersion == "" {
		baseVersion = strings.TrimSpace(in.RegistryPipelineVersion)
	}
	if baseVersion == "" {
		baseVersion = strings.TrimSpace(in.RequestedPipelineVersion)
	}
	if baseVersion == "" {
		return "", fmt.Errorf("pipeline_version is required")
	}

	canaryVersion := strings.TrimSpace(policy.CanaryPipelineVersion)
	if canaryVersion == "" {
		return baseVersion, nil
	}

	if tenantAllowlisted(in.TenantID, policy.TenantAllowlist) {
		return canaryVersion, nil
	}
	if ShouldRouteToCanary(in.SessionID, in.TenantID, policy.CanaryPercentage) {
		return canaryVersion, nil
	}
	return baseVersion, nil
}

func normalizePolicy(in Policy) Policy {
	out := in
	out.BasePipelineVersion = strings.TrimSpace(out.BasePipelineVersion)
	out.CanaryPipelineVersion = strings.TrimSpace(out.CanaryPipelineVersion)

	allowlist := make([]string, 0, len(out.TenantAllowlist))
	for _, tenantID := range out.TenantAllowlist {
		tenantID = strings.TrimSpace(tenantID)
		if tenantID == "" {
			continue
		}
		if slices.Contains(allowlist, tenantID) {
			continue
		}
		allowlist = append(allowlist, tenantID)
	}
	out.TenantAllowlist = allowlist
	return out
}

// ShouldRouteToCanary deterministically maps a session into the canary cohort.
func ShouldRouteToCanary(sessionID string, tenantID string, percentage int) bool {
	if percentage <= 0 {
		return false
	}
	if percentage >= 100 {
		return true
	}

	key := strings.TrimSpace(sessionID)
	if key == "" {
		return false
	}
	tenant := strings.TrimSpace(tenantID)
	if tenant != "" {
		key = tenant + ":" + key
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))
	bucket := int(hasher.Sum32() % 100)
	return bucket < percentage
}

func tenantAllowlisted(tenantID string, allowlist []string) bool {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return false
	}
	for _, candidate := range allowlist {
		if tenantID == strings.TrimSpace(candidate) {
			return true
		}
	}
	return false
}
