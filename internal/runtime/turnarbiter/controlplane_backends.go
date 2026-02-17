package turnarbiter

import (
	"fmt"
	"os"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/distribution"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/normalizer"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
)

const (
	// ControlPlaneDistributionPathEnv configures the file-backed CP distribution path.
	ControlPlaneDistributionPathEnv = distribution.EnvFileAdapterPath
	// ControlPlaneDistributionHTTPURLsEnv configures ordered HTTP endpoints for CP distribution.
	ControlPlaneDistributionHTTPURLsEnv = distribution.EnvHTTPAdapterURLs
	// ControlPlaneDistributionHTTPURLEnv configures the HTTP-backed CP distribution endpoint.
	ControlPlaneDistributionHTTPURLEnv = distribution.EnvHTTPAdapterURL
	// ControlPlaneBackendFallbackModeEnv configures strict-vs-availability backend fallback behavior.
	ControlPlaneBackendFallbackModeEnv = "RSPP_CP_BACKEND_FALLBACK_MODE"
)

// BackendFallbackMode controls non-stale backend error handling behavior.
type BackendFallbackMode string

const (
	// BackendFallbackModeAvailability keeps availability-mode behavior (fallback defaults on non-stale backend errors).
	BackendFallbackModeAvailability BackendFallbackMode = "availability"
	// BackendFallbackModeStrict propagates backend errors instead of applying fallback defaults.
	BackendFallbackModeStrict BackendFallbackMode = "strict"
)

// ControlPlaneBackends declares optional backend resolvers used by CP turn-start services.
type ControlPlaneBackends struct {
	Registry       registry.Backend
	Rollout        rollout.Backend
	RoutingView    routingview.Backend
	Policy         policy.Backend
	ProviderHealth providerhealth.Backend
	GraphCompiler  graphcompiler.Backend
	Admission      admission.Backend
	Lease          lease.Backend
	FallbackMode   BackendFallbackMode
}

// NewControlPlaneBackendsFromDistributionFile builds CP backends from a file-backed distribution artifact.
func NewControlPlaneBackendsFromDistributionFile(path string) (ControlPlaneBackends, error) {
	fallbackMode, err := backendFallbackModeFromEnv()
	if err != nil {
		return ControlPlaneBackends{}, err
	}
	serviceBackends, err := distribution.NewFileBackends(distribution.FileAdapterConfig{Path: path})
	if err != nil {
		return ControlPlaneBackends{}, fmt.Errorf("load control-plane distribution backends: %w", err)
	}
	out := newControlPlaneBackends(serviceBackends)
	out.FallbackMode = fallbackMode
	return out, nil
}

// NewControlPlaneBackendsFromDistributionHTTP builds CP backends from an HTTP distribution endpoint.
func NewControlPlaneBackendsFromDistributionHTTP(url string) (ControlPlaneBackends, error) {
	fallbackMode, err := backendFallbackModeFromEnv()
	if err != nil {
		return ControlPlaneBackends{}, err
	}
	serviceBackends, err := distribution.NewHTTPBackends(distribution.HTTPAdapterConfig{URL: url})
	if err != nil {
		return ControlPlaneBackends{}, fmt.Errorf("load control-plane distribution http backends: %w", err)
	}
	out := newControlPlaneBackends(serviceBackends)
	out.FallbackMode = fallbackMode
	return out, nil
}

// NewControlPlaneBackendsFromDistributionEnv builds CP backends from env-configured distribution artifacts.
func NewControlPlaneBackendsFromDistributionEnv() (ControlPlaneBackends, error) {
	fallbackMode, err := backendFallbackModeFromEnv()
	if err != nil {
		return ControlPlaneBackends{}, err
	}

	if strings.TrimSpace(os.Getenv(ControlPlaneDistributionHTTPURLsEnv)) != "" || strings.TrimSpace(os.Getenv(ControlPlaneDistributionHTTPURLEnv)) != "" {
		serviceBackends, err := distribution.NewHTTPBackendsFromEnv()
		if err != nil {
			return ControlPlaneBackends{}, fmt.Errorf("load control-plane distribution http backends from env: %w", err)
		}
		out := newControlPlaneBackends(serviceBackends)
		out.FallbackMode = fallbackMode
		return out, nil
	}

	serviceBackends, err := distribution.NewFileBackendsFromEnv()
	if err != nil {
		return ControlPlaneBackends{}, fmt.Errorf("load control-plane distribution backends from env: %w", err)
	}
	out := newControlPlaneBackends(serviceBackends)
	out.FallbackMode = fallbackMode
	return out, nil
}

// NewWithControlPlaneBackendsFromDistributionFile loads CP backends from file and wires arbiter.
func NewWithControlPlaneBackendsFromDistributionFile(recorder *timeline.Recorder, path string) (Arbiter, error) {
	backends, err := NewControlPlaneBackendsFromDistributionFile(path)
	if err != nil {
		return Arbiter{}, err
	}
	return NewWithControlPlaneBackends(recorder, backends), nil
}

// NewWithControlPlaneBackendsFromDistributionHTTP loads CP backends from HTTP and wires arbiter.
func NewWithControlPlaneBackendsFromDistributionHTTP(recorder *timeline.Recorder, url string) (Arbiter, error) {
	backends, err := NewControlPlaneBackendsFromDistributionHTTP(url)
	if err != nil {
		return Arbiter{}, err
	}
	return NewWithControlPlaneBackends(recorder, backends), nil
}

// NewWithControlPlaneBackendsFromDistributionEnv loads CP backends from env and wires arbiter.
func NewWithControlPlaneBackendsFromDistributionEnv(recorder *timeline.Recorder) (Arbiter, error) {
	backends, err := NewControlPlaneBackendsFromDistributionEnv()
	if err != nil {
		return Arbiter{}, err
	}
	return NewWithControlPlaneBackends(recorder, backends), nil
}

// NewControlPlaneBundleResolverWithBackends builds CP services with backend resolver wiring.
func NewControlPlaneBundleResolverWithBackends(backends ControlPlaneBackends) TurnStartBundleResolver {
	wrappedBackends := withFallbackBackends(backends)

	registryService := registry.NewService()
	registryService.Backend = wrappedBackends.Registry

	rolloutService := rollout.NewService()
	rolloutService.Backend = wrappedBackends.Rollout

	routingService := routingview.NewService()
	routingService.Backend = wrappedBackends.RoutingView

	policyService := policy.NewService()
	policyService.Backend = wrappedBackends.Policy

	providerHealthService := providerhealth.NewService()
	providerHealthService.Backend = wrappedBackends.ProviderHealth

	graphCompilerService := graphcompiler.NewService()
	graphCompilerService.Backend = wrappedBackends.GraphCompiler

	admissionService := admission.NewService()
	admissionService.Backend = wrappedBackends.Admission

	leaseService := lease.NewService()
	leaseService.Backend = wrappedBackends.Lease

	return NewControlPlaneBundleResolverWithServices(ControlPlaneBundleServices{
		Registry:       registryService,
		Normalizer:     normalizer.Service{},
		Rollout:        rolloutService,
		RoutingView:    routingService,
		Policy:         policyService,
		ProviderHealth: providerHealthService,
		GraphCompiler:  graphCompilerService,
		Admission:      admissionService,
		Lease:          leaseService,
	})
}

func newControlPlaneBackends(serviceBackends distribution.ServiceBackends) ControlPlaneBackends {
	return ControlPlaneBackends{
		Registry:       serviceBackends.Registry,
		Rollout:        serviceBackends.Rollout,
		RoutingView:    serviceBackends.RoutingView,
		Policy:         serviceBackends.Policy,
		ProviderHealth: serviceBackends.ProviderHealth,
		GraphCompiler:  serviceBackends.GraphCompiler,
		Admission:      serviceBackends.Admission,
		Lease:          serviceBackends.Lease,
	}
}

func withFallbackBackends(backends ControlPlaneBackends) ControlPlaneBackends {
	if normalizeBackendFallbackMode(backends.FallbackMode) == BackendFallbackModeStrict {
		return backends
	}

	out := backends
	if out.Registry != nil {
		out.Registry = fallbackRegistryBackend{backend: out.Registry}
	}
	if out.Rollout != nil {
		out.Rollout = fallbackRolloutBackend{backend: out.Rollout}
	}
	if out.RoutingView != nil {
		out.RoutingView = fallbackRoutingBackend{backend: out.RoutingView}
	}
	if out.Policy != nil {
		out.Policy = fallbackPolicyBackend{backend: out.Policy}
	}
	if out.ProviderHealth != nil {
		out.ProviderHealth = fallbackProviderHealthBackend{backend: out.ProviderHealth}
	}
	if out.GraphCompiler != nil {
		out.GraphCompiler = fallbackGraphCompilerBackend{backend: out.GraphCompiler}
	}
	if out.Admission != nil {
		out.Admission = fallbackAdmissionBackend{backend: out.Admission}
	}
	if out.Lease != nil {
		out.Lease = fallbackLeaseBackend{backend: out.Lease}
	}
	return out
}

type fallbackRegistryBackend struct {
	backend registry.Backend
}

func (b fallbackRegistryBackend) ResolvePipelineRecord(pipelineVersion string) (registry.PipelineRecord, error) {
	record, err := b.backend.ResolvePipelineRecord(pipelineVersion)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return registry.PipelineRecord{}, err
		}
		return registry.PipelineRecord{}, nil
	}
	return record, nil
}

type fallbackRolloutBackend struct {
	backend rollout.Backend
}

func (b fallbackRolloutBackend) ResolvePipelineVersion(in rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
	out, err := b.backend.ResolvePipelineVersion(in)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return rollout.ResolveVersionOutput{}, err
		}
		return rollout.ResolveVersionOutput{}, nil
	}
	return out, nil
}

type fallbackRoutingBackend struct {
	backend routingview.Backend
}

func (b fallbackRoutingBackend) GetSnapshot(in routingview.Input) (routingview.Snapshot, error) {
	snapshot, err := b.backend.GetSnapshot(in)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return routingview.Snapshot{}, err
		}
		return routingview.Snapshot{}, nil
	}
	return snapshot, nil
}

type fallbackPolicyBackend struct {
	backend policy.Backend
}

func (b fallbackPolicyBackend) Evaluate(in policy.Input) (policy.Output, error) {
	out, err := b.backend.Evaluate(in)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return policy.Output{}, err
		}
		return policy.Output{}, nil
	}
	return out, nil
}

type fallbackProviderHealthBackend struct {
	backend providerhealth.Backend
}

func (b fallbackProviderHealthBackend) GetSnapshot(in providerhealth.Input) (providerhealth.Output, error) {
	out, err := b.backend.GetSnapshot(in)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return providerhealth.Output{}, err
		}
		return providerhealth.Output{}, nil
	}
	return out, nil
}

type fallbackGraphCompilerBackend struct {
	backend graphcompiler.Backend
}

func (b fallbackGraphCompilerBackend) Compile(in graphcompiler.Input) (graphcompiler.Output, error) {
	out, err := b.backend.Compile(in)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return graphcompiler.Output{}, err
		}
		return graphcompiler.Output{}, nil
	}
	return out, nil
}

type fallbackAdmissionBackend struct {
	backend admission.Backend
}

func (b fallbackAdmissionBackend) Evaluate(in admission.Input) (admission.Output, error) {
	out, err := b.backend.Evaluate(in)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return admission.Output{}, err
		}
		return admission.Output{}, nil
	}
	return out, nil
}

type fallbackLeaseBackend struct {
	backend lease.Backend
}

func (b fallbackLeaseBackend) Resolve(in lease.Input) (lease.Output, error) {
	out, err := b.backend.Resolve(in)
	if err != nil {
		if isStaleSnapshotResolutionError(err) {
			return lease.Output{}, err
		}
		return lease.Output{}, nil
	}
	return out, nil
}

// NewWithControlPlaneBackends wires arbiter with CP backend resolver integration.
func NewWithControlPlaneBackends(recorder *timeline.Recorder, backends ControlPlaneBackends) Arbiter {
	return NewWithDependencies(recorder, NewControlPlaneBundleResolverWithBackends(backends))
}

func backendFallbackModeFromEnv() (BackendFallbackMode, error) {
	raw := strings.TrimSpace(os.Getenv(ControlPlaneBackendFallbackModeEnv))
	if raw == "" {
		return BackendFallbackModeAvailability, nil
	}
	return parseBackendFallbackMode(raw)
}

func parseBackendFallbackMode(raw string) (BackendFallbackMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(BackendFallbackModeAvailability):
		return BackendFallbackModeAvailability, nil
	case string(BackendFallbackModeStrict):
		return BackendFallbackModeStrict, nil
	default:
		return "", fmt.Errorf("invalid %s %q: expected one of [%s,%s]", ControlPlaneBackendFallbackModeEnv, raw, BackendFallbackModeAvailability, BackendFallbackModeStrict)
	}
}

func normalizeBackendFallbackMode(mode BackendFallbackMode) BackendFallbackMode {
	switch mode {
	case BackendFallbackModeStrict:
		return BackendFallbackModeStrict
	default:
		return BackendFallbackModeAvailability
	}
}
