package distribution

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

const (
	// EnvFileAdapterPath configures the file-backed CP distribution artifact path.
	EnvFileAdapterPath = "RSPP_CP_DISTRIBUTION_PATH"
	// SchemaVersionV1 is the expected schema version for file-backed CP distribution artifacts.
	SchemaVersionV1 = "cp-snapshot-distribution/v1"
)

// ErrorCode classifies deterministic file-backed CP distribution adapter failures.
type ErrorCode string

const (
	ErrorCodeInvalidConfig   ErrorCode = "invalid_config"
	ErrorCodeReadArtifact    ErrorCode = "artifact_read_failed"
	ErrorCodeDecodeArtifact  ErrorCode = "artifact_decode_failed"
	ErrorCodeInvalidArtifact ErrorCode = "artifact_invalid"
	ErrorCodeSnapshotMissing ErrorCode = "snapshot_missing"
	ErrorCodeSnapshotStale   ErrorCode = "snapshot_stale"
)

// BackendError is a deterministic file-backed CP distribution backend error.
type BackendError struct {
	Service string
	Code    ErrorCode
	Path    string
	Cause   error
}

func (e BackendError) Error() string {
	service := strings.TrimSpace(e.Service)
	if service == "" {
		service = "control_plane_distribution"
	}
	if e.Cause == nil {
		if strings.TrimSpace(e.Path) == "" {
			return fmt.Sprintf("%s: %s", service, e.Code)
		}
		return fmt.Sprintf("%s: %s (%s)", service, e.Code, e.Path)
	}
	if strings.TrimSpace(e.Path) == "" {
		return fmt.Sprintf("%s: %s: %v", service, e.Code, e.Cause)
	}
	return fmt.Sprintf("%s: %s (%s): %v", service, e.Code, e.Path, e.Cause)
}

func (e BackendError) Unwrap() error {
	return e.Cause
}

// StaleSnapshot marks snapshot-fetch stale failures for deterministic pre-turn handling.
func (e BackendError) StaleSnapshot() bool {
	return e.Code == ErrorCodeSnapshotStale
}

// FileAdapterConfig configures a file-backed CP snapshot distribution adapter.
type FileAdapterConfig struct {
	Path string
}

// FileAdapterConfigFromEnv resolves adapter config from environment.
func FileAdapterConfigFromEnv() (FileAdapterConfig, error) {
	path := strings.TrimSpace(os.Getenv(EnvFileAdapterPath))
	if path == "" {
		return FileAdapterConfig{}, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: EnvFileAdapterPath}
	}
	return FileAdapterConfig{Path: path}, nil
}

// ServiceBackends groups concrete CP backends loaded from distribution artifacts.
type ServiceBackends struct {
	Registry       registry.Backend
	Rollout        rollout.Backend
	RoutingView    routingview.Backend
	Policy         policy.Backend
	ProviderHealth providerhealth.Backend
	GraphCompiler  graphcompiler.Backend
	Admission      admission.Backend
	Lease          lease.Backend
}

// NewFileBackendsFromEnv builds CP service backends from an env-configured artifact.
func NewFileBackendsFromEnv() (ServiceBackends, error) {
	cfg, err := FileAdapterConfigFromEnv()
	if err != nil {
		return ServiceBackends{}, err
	}
	return NewFileBackends(cfg)
}

// NewFileBackends builds CP service backends from a file-backed distribution artifact.
func NewFileBackends(cfg FileAdapterConfig) (ServiceBackends, error) {
	adapter, err := newFileAdapter(cfg)
	if err != nil {
		return ServiceBackends{}, err
	}
	return serviceBackendsFromAdapter(adapter), nil
}

type fileAdapter struct {
	path     string
	artifact fileArtifact
}

func newFileAdapter(cfg FileAdapterConfig) (fileAdapter, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeInvalidConfig, Path: "path"}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeReadArtifact, Path: path, Cause: err}
	}

	var artifact fileArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return fileAdapter{}, BackendError{Service: "distribution", Code: ErrorCodeDecodeArtifact, Path: path, Cause: err}
	}
	if err := artifact.validate(path); err != nil {
		return fileAdapter{}, err
	}

	return fileAdapter{path: path, artifact: artifact}, nil
}

func serviceBackendsFromAdapter(adapter fileAdapter) ServiceBackends {
	return ServiceBackends{
		Registry:       fileRegistryBackend{adapter: adapter},
		Rollout:        fileRolloutBackend{adapter: adapter},
		RoutingView:    fileRoutingBackend{adapter: adapter},
		Policy:         filePolicyBackend{adapter: adapter},
		ProviderHealth: fileProviderHealthBackend{adapter: adapter},
		GraphCompiler:  fileGraphCompilerBackend{adapter: adapter},
		Admission:      fileAdmissionBackend{adapter: adapter},
		Lease:          fileLeaseBackend{adapter: adapter},
	}
}

type fileArtifact struct {
	SchemaVersion  string                    `json:"schema_version"`
	Stale          bool                      `json:"stale,omitempty"`
	Registry       fileRegistrySection       `json:"registry"`
	Rollout        fileRolloutSection        `json:"rollout"`
	RoutingView    fileRoutingSection        `json:"routing_view"`
	Policy         filePolicySection         `json:"policy"`
	ProviderHealth fileProviderHealthSection `json:"provider_health"`
	GraphCompiler  fileGraphCompilerSection  `json:"graph_compiler"`
	Admission      fileAdmissionSection      `json:"admission"`
	Lease          fileLeaseSection          `json:"lease"`
}

func (a fileArtifact) validate(path string) error {
	schema := strings.TrimSpace(a.SchemaVersion)
	if schema == "" {
		return BackendError{Service: "distribution", Code: ErrorCodeInvalidArtifact, Path: path, Cause: fmt.Errorf("schema_version is required")}
	}
	if schema != SchemaVersionV1 {
		return BackendError{Service: "distribution", Code: ErrorCodeInvalidArtifact, Path: path, Cause: fmt.Errorf("unsupported schema_version %q", schema)}
	}
	return nil
}

type fileRegistrySection struct {
	Stale                  bool                          `json:"stale,omitempty"`
	DefaultPipelineVersion string                        `json:"default_pipeline_version,omitempty"`
	Records                map[string]filePipelineRecord `json:"records,omitempty"`
}

type filePipelineRecord struct {
	PipelineVersion    string `json:"pipeline_version,omitempty"`
	GraphDefinitionRef string `json:"graph_definition_ref,omitempty"`
	ExecutionProfile   string `json:"execution_profile,omitempty"`
}

type fileRolloutSection struct {
	Stale                     bool              `json:"stale,omitempty"`
	DefaultPipelineVersion    string            `json:"default_pipeline_version,omitempty"`
	VersionResolutionSnapshot string            `json:"version_resolution_snapshot,omitempty"`
	ByRequestedVersion        map[string]string `json:"by_requested_version,omitempty"`
}

type fileRoutingSection struct {
	Stale      bool                           `json:"stale,omitempty"`
	Default    fileRoutingSnapshot            `json:"default,omitempty"`
	ByPipeline map[string]fileRoutingSnapshot `json:"by_pipeline,omitempty"`
}

type fileRoutingSnapshot struct {
	RoutingViewSnapshot      string `json:"routing_view_snapshot,omitempty"`
	AdmissionPolicySnapshot  string `json:"admission_policy_snapshot,omitempty"`
	ABICompatibilitySnapshot string `json:"abi_compatibility_snapshot,omitempty"`
}

type filePolicySection struct {
	Stale      bool                        `json:"stale,omitempty"`
	Default    filePolicyOutput            `json:"default,omitempty"`
	ByPipeline map[string]filePolicyOutput `json:"by_pipeline,omitempty"`
}

type filePolicyOutput struct {
	PolicyResolutionSnapshot string   `json:"policy_resolution_snapshot,omitempty"`
	AllowedAdaptiveActions   []string `json:"allowed_adaptive_actions,omitempty"`
}

type fileProviderHealthSection struct {
	Stale      bool                                `json:"stale,omitempty"`
	Default    fileProviderHealthOutput            `json:"default,omitempty"`
	ByPipeline map[string]fileProviderHealthOutput `json:"by_pipeline,omitempty"`
}

type fileProviderHealthOutput struct {
	ProviderHealthSnapshot string `json:"provider_health_snapshot,omitempty"`
}

type fileGraphCompilerSection struct {
	Stale      bool                               `json:"stale,omitempty"`
	Default    fileGraphCompilerOutput            `json:"default,omitempty"`
	ByPipeline map[string]fileGraphCompilerOutput `json:"by_pipeline,omitempty"`
}

type fileGraphCompilerOutput struct {
	GraphDefinitionRef       string `json:"graph_definition_ref,omitempty"`
	GraphCompilationSnapshot string `json:"graph_compilation_snapshot,omitempty"`
	GraphFingerprint         string `json:"graph_fingerprint,omitempty"`
}

type fileAdmissionSection struct {
	Stale      bool                           `json:"stale,omitempty"`
	Default    fileAdmissionOutput            `json:"default,omitempty"`
	ByPipeline map[string]fileAdmissionOutput `json:"by_pipeline,omitempty"`
}

type fileAdmissionOutput struct {
	AdmissionPolicySnapshot string `json:"admission_policy_snapshot,omitempty"`
	OutcomeKind             string `json:"outcome_kind,omitempty"`
	Scope                   string `json:"scope,omitempty"`
	Reason                  string `json:"reason,omitempty"`
}

type fileLeaseSection struct {
	Stale      bool                       `json:"stale,omitempty"`
	Default    fileLeaseOutput            `json:"default,omitempty"`
	ByPipeline map[string]fileLeaseOutput `json:"by_pipeline,omitempty"`
}

type fileLeaseOutput struct {
	LeaseResolutionSnapshot string `json:"lease_resolution_snapshot,omitempty"`
	AuthorityEpoch          *int64 `json:"authority_epoch,omitempty"`
	AuthorityEpochValid     *bool  `json:"authority_epoch_valid,omitempty"`
	AuthorityAuthorized     *bool  `json:"authority_authorized,omitempty"`
	Reason                  string `json:"reason,omitempty"`
}

type fileRegistryBackend struct {
	adapter fileAdapter
}

func (b fileRegistryBackend) ResolvePipelineRecord(pipelineVersion string) (registry.PipelineRecord, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.Registry.Stale {
		return registry.PipelineRecord{}, BackendError{Service: "registry", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	version := strings.TrimSpace(pipelineVersion)
	if version == "" {
		version = strings.TrimSpace(b.adapter.artifact.Registry.DefaultPipelineVersion)
	}
	if version == "" {
		return registry.PipelineRecord{}, BackendError{Service: "registry", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("pipeline_version is required")}
	}

	record, ok := b.adapter.artifact.Registry.Records[version]
	if !ok {
		return registry.PipelineRecord{}, BackendError{Service: "registry", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing record for pipeline_version=%s", version)}
	}

	return registry.PipelineRecord{
		PipelineVersion:    record.PipelineVersion,
		GraphDefinitionRef: record.GraphDefinitionRef,
		ExecutionProfile:   record.ExecutionProfile,
	}, nil
}

type fileRolloutBackend struct {
	adapter fileAdapter
}

func (b fileRolloutBackend) ResolvePipelineVersion(in rollout.ResolveVersionInput) (rollout.ResolveVersionOutput, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.Rollout.Stale {
		return rollout.ResolveVersionOutput{}, BackendError{Service: "rollout", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	version := ""
	if req := strings.TrimSpace(in.RequestedPipelineVersion); req != "" {
		version = strings.TrimSpace(b.adapter.artifact.Rollout.ByRequestedVersion[req])
	}
	if version == "" {
		version = strings.TrimSpace(b.adapter.artifact.Rollout.DefaultPipelineVersion)
	}
	if version == "" {
		return rollout.ResolveVersionOutput{}, BackendError{Service: "rollout", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing rollout pipeline version")}
	}

	return rollout.ResolveVersionOutput{
		PipelineVersion:           version,
		VersionResolutionSnapshot: strings.TrimSpace(b.adapter.artifact.Rollout.VersionResolutionSnapshot),
	}, nil
}

type fileRoutingBackend struct {
	adapter fileAdapter
}

func (b fileRoutingBackend) GetSnapshot(in routingview.Input) (routingview.Snapshot, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.RoutingView.Stale {
		return routingview.Snapshot{}, BackendError{Service: "routing_view", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	snapshot := b.adapter.artifact.RoutingView.Default
	if in.PipelineVersion != "" {
		if byPipeline, ok := b.adapter.artifact.RoutingView.ByPipeline[in.PipelineVersion]; ok {
			snapshot = byPipeline
		}
	}
	if snapshot == (fileRoutingSnapshot{}) {
		return routingview.Snapshot{}, BackendError{Service: "routing_view", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing routing snapshot")}
	}

	return routingview.Snapshot{
		RoutingViewSnapshot:      snapshot.RoutingViewSnapshot,
		AdmissionPolicySnapshot:  snapshot.AdmissionPolicySnapshot,
		ABICompatibilitySnapshot: snapshot.ABICompatibilitySnapshot,
	}, nil
}

type filePolicyBackend struct {
	adapter fileAdapter
}

func (b filePolicyBackend) Evaluate(in policy.Input) (policy.Output, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.Policy.Stale {
		return policy.Output{}, BackendError{Service: "policy", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	out := b.adapter.artifact.Policy.Default
	if in.PipelineVersion != "" {
		if byPipeline, ok := b.adapter.artifact.Policy.ByPipeline[in.PipelineVersion]; ok {
			out = byPipeline
		}
	}
	if out.PolicyResolutionSnapshot == "" && len(out.AllowedAdaptiveActions) == 0 {
		return policy.Output{}, BackendError{Service: "policy", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing policy snapshot")}
	}

	return policy.Output{
		PolicyResolutionSnapshot: out.PolicyResolutionSnapshot,
		AllowedAdaptiveActions:   append([]string(nil), out.AllowedAdaptiveActions...),
	}, nil
}

type fileProviderHealthBackend struct {
	adapter fileAdapter
}

func (b fileProviderHealthBackend) GetSnapshot(in providerhealth.Input) (providerhealth.Output, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.ProviderHealth.Stale {
		return providerhealth.Output{}, BackendError{Service: "provider_health", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	out := b.adapter.artifact.ProviderHealth.Default
	if in.PipelineVersion != "" {
		if byPipeline, ok := b.adapter.artifact.ProviderHealth.ByPipeline[in.PipelineVersion]; ok {
			out = byPipeline
		}
	}
	if out == (fileProviderHealthOutput{}) {
		return providerhealth.Output{}, BackendError{Service: "provider_health", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing provider health snapshot")}
	}

	return providerhealth.Output{ProviderHealthSnapshot: out.ProviderHealthSnapshot}, nil
}

type fileGraphCompilerBackend struct {
	adapter fileAdapter
}

func (b fileGraphCompilerBackend) Compile(in graphcompiler.Input) (graphcompiler.Output, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.GraphCompiler.Stale {
		return graphcompiler.Output{}, BackendError{Service: "graph_compiler", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	out := b.adapter.artifact.GraphCompiler.Default
	if in.PipelineVersion != "" {
		if byPipeline, ok := b.adapter.artifact.GraphCompiler.ByPipeline[in.PipelineVersion]; ok {
			out = byPipeline
		}
	}
	if out == (fileGraphCompilerOutput{}) {
		return graphcompiler.Output{}, BackendError{Service: "graph_compiler", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing graph compiler snapshot")}
	}

	return graphcompiler.Output{
		GraphDefinitionRef:       out.GraphDefinitionRef,
		GraphCompilationSnapshot: out.GraphCompilationSnapshot,
		GraphFingerprint:         out.GraphFingerprint,
	}, nil
}

type fileAdmissionBackend struct {
	adapter fileAdapter
}

func (b fileAdmissionBackend) Evaluate(in admission.Input) (admission.Output, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.Admission.Stale {
		return admission.Output{}, BackendError{Service: "admission", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	out := b.adapter.artifact.Admission.Default
	if in.PipelineVersion != "" {
		if byPipeline, ok := b.adapter.artifact.Admission.ByPipeline[in.PipelineVersion]; ok {
			out = byPipeline
		}
	}
	if out == (fileAdmissionOutput{}) {
		return admission.Output{}, BackendError{Service: "admission", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing admission snapshot")}
	}

	return admission.Output{
		AdmissionPolicySnapshot: out.AdmissionPolicySnapshot,
		OutcomeKind:             controlplane.OutcomeKind(strings.TrimSpace(out.OutcomeKind)),
		Scope:                   controlplane.OutcomeScope(strings.TrimSpace(out.Scope)),
		Reason:                  out.Reason,
	}, nil
}

type fileLeaseBackend struct {
	adapter fileAdapter
}

func (b fileLeaseBackend) Resolve(in lease.Input) (lease.Output, error) {
	if b.adapter.artifact.Stale || b.adapter.artifact.Lease.Stale {
		return lease.Output{}, BackendError{Service: "lease", Code: ErrorCodeSnapshotStale, Path: b.adapter.path}
	}

	out := b.adapter.artifact.Lease.Default
	if in.PipelineVersion != "" {
		if byPipeline, ok := b.adapter.artifact.Lease.ByPipeline[in.PipelineVersion]; ok {
			out = byPipeline
		}
	}
	if out.LeaseResolutionSnapshot == "" && out.AuthorityEpoch == nil && out.AuthorityEpochValid == nil && out.AuthorityAuthorized == nil && out.Reason == "" {
		return lease.Output{}, BackendError{Service: "lease", Code: ErrorCodeSnapshotMissing, Path: b.adapter.path, Cause: fmt.Errorf("missing lease snapshot")}
	}

	resolvedEpoch := int64(0)
	if out.AuthorityEpoch != nil {
		resolvedEpoch = *out.AuthorityEpoch
	}

	return lease.Output{
		LeaseResolutionSnapshot: out.LeaseResolutionSnapshot,
		AuthorityEpoch:          resolvedEpoch,
		AuthorityEpochValid:     cloneBoolPointer(out.AuthorityEpochValid),
		AuthorityAuthorized:     cloneBoolPointer(out.AuthorityAuthorized),
		Reason:                  out.Reason,
	}, nil
}

func cloneBoolPointer(v *bool) *bool {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}
