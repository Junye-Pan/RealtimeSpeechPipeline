package turnarbiter

import (
	"errors"
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/admission"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/graphcompiler"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/normalizer"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/policy"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/providerhealth"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

// TurnStartBundleInput captures CP context needed to freeze turn-start runtime inputs.
type TurnStartBundleInput struct {
	SessionID                string
	TurnID                   string
	RequestedPipelineVersion string
	AuthorityEpoch           int64
}

// TurnStartBundle is the runtime-facing control-plane artifact seam for RK-04.
type TurnStartBundle struct {
	PipelineVersion        string
	GraphDefinitionRef     string
	ExecutionProfile       string
	GraphFingerprint       string
	AllowedAdaptiveActions []string
	SnapshotProvenance     controlplane.SnapshotProvenance
	HasCPAdmissionDecision bool
	CPAdmissionOutcomeKind controlplane.OutcomeKind
	CPAdmissionScope       controlplane.OutcomeScope
	CPAdmissionReason      string
	HasLeaseDecision       bool
	LeaseAuthorityEpoch    int64
	LeaseAuthorityValid    bool
	LeaseAuthorityGranted  bool
}

// Validate enforces required turn-start bundle fields.
func (b TurnStartBundle) Validate() error {
	if b.PipelineVersion == "" {
		return fmt.Errorf("pipeline_version is required")
	}
	if b.GraphDefinitionRef == "" {
		return fmt.Errorf("graph_definition_ref is required")
	}
	if b.ExecutionProfile == "" {
		return fmt.Errorf("execution_profile is required")
	}
	if b.AllowedAdaptiveActions == nil {
		return fmt.Errorf("allowed_adaptive_actions is required")
	}
	if err := b.SnapshotProvenance.Validate(); err != nil {
		return err
	}
	if b.HasCPAdmissionDecision {
		if b.CPAdmissionOutcomeKind != controlplane.OutcomeAdmit &&
			b.CPAdmissionOutcomeKind != controlplane.OutcomeReject &&
			b.CPAdmissionOutcomeKind != controlplane.OutcomeDefer {
			return fmt.Errorf("cp_admission_outcome_kind must be admit|reject|defer")
		}
		if b.CPAdmissionScope != "" &&
			b.CPAdmissionScope != controlplane.ScopeTenant &&
			b.CPAdmissionScope != controlplane.ScopeSession {
			return fmt.Errorf("cp_admission_scope must be tenant|session")
		}
		if b.CPAdmissionReason == "" {
			return fmt.Errorf("cp_admission_reason is required")
		}
	}
	if b.HasLeaseDecision {
		if b.LeaseAuthorityEpoch < 0 {
			return fmt.Errorf("lease_authority_epoch must be >=0")
		}
	}
	return nil
}

// TurnStartBundleResolver defines the runtime seam used for CP-derived turn-start data.
type TurnStartBundleResolver interface {
	ResolveTurnStartBundle(in TurnStartBundleInput) (TurnStartBundle, error)
}

type controlPlaneBundleResolver struct {
	registry       registry.Service
	normalizer     normalizer.Service
	rollout        rollout.Service
	routingView    routingview.Service
	policy         policy.Service
	providerHealth providerhealth.Service
	graphCompiler  graphcompiler.Service
	admission      admission.Service
	lease          lease.Service
}

// ControlPlaneBundleServices defines CP service dependencies for turn-start resolution.
type ControlPlaneBundleServices struct {
	Registry       registry.Service
	Normalizer     normalizer.Service
	Rollout        rollout.Service
	RoutingView    routingview.Service
	Policy         policy.Service
	ProviderHealth providerhealth.Service
	GraphCompiler  graphcompiler.Service
	Admission      admission.Service
	Lease          lease.Service
}

func newControlPlaneBundleResolver() TurnStartBundleResolver {
	return NewControlPlaneBundleResolverWithBackends(ControlPlaneBackends{})
}

// NewControlPlaneBundleResolverWithServices builds a resolver with explicit CP service dependencies.
func NewControlPlaneBundleResolverWithServices(services ControlPlaneBundleServices) TurnStartBundleResolver {
	return controlPlaneBundleResolver{
		registry:       services.Registry,
		normalizer:     services.Normalizer,
		rollout:        services.Rollout,
		routingView:    services.RoutingView,
		policy:         services.Policy,
		providerHealth: services.ProviderHealth,
		graphCompiler:  services.GraphCompiler,
		admission:      services.Admission,
		lease:          services.Lease,
	}
}

func (r controlPlaneBundleResolver) ResolveTurnStartBundle(in TurnStartBundleInput) (TurnStartBundle, error) {
	if in.SessionID == "" {
		return TurnStartBundle{}, fmt.Errorf("session_id is required")
	}

	record, err := r.registry.ResolvePipelineRecord(in.RequestedPipelineVersion)
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("resolve pipeline record: %w", err)
	}

	normalized, err := r.normalizer.Normalize(normalizer.Input{Record: record})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("normalize pipeline record: %w", err)
	}

	rolloutResult, err := r.rollout.ResolvePipelineVersion(rollout.ResolveVersionInput{
		SessionID:                in.SessionID,
		RequestedPipelineVersion: in.RequestedPipelineVersion,
		RegistryPipelineVersion:  normalized.PipelineVersion,
	})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("resolve pipeline version: %w", err)
	}

	compiledGraph, err := r.graphCompiler.Compile(graphcompiler.Input{
		PipelineVersion:    rolloutResult.PipelineVersion,
		GraphDefinitionRef: normalized.GraphDefinitionRef,
		ExecutionProfile:   normalized.ExecutionProfile,
	})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("compile graph definition: %w", err)
	}

	routingSnapshot, err := r.routingView.GetSnapshot(routingview.Input{
		SessionID:       in.SessionID,
		PipelineVersion: rolloutResult.PipelineVersion,
		AuthorityEpoch:  in.AuthorityEpoch,
	})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("resolve routing snapshot: %w", err)
	}

	providerHealthSnapshot, err := r.providerHealth.GetSnapshot(providerhealth.Input{
		Scope:           in.SessionID,
		PipelineVersion: rolloutResult.PipelineVersion,
	})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("resolve provider health snapshot: %w", err)
	}

	policyResult, err := r.policy.Evaluate(policy.Input{
		SessionID:              in.SessionID,
		TurnID:                 in.TurnID,
		PipelineVersion:        rolloutResult.PipelineVersion,
		ProviderHealthSnapshot: providerHealthSnapshot.ProviderHealthSnapshot,
	})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("evaluate policy snapshot: %w", err)
	}

	leaseResult, err := r.lease.Resolve(lease.Input{
		SessionID:               in.SessionID,
		PipelineVersion:         rolloutResult.PipelineVersion,
		RequestedAuthorityEpoch: in.AuthorityEpoch,
	})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("resolve lease authority: %w", err)
	}

	admissionResult, err := r.admission.Evaluate(admission.Input{
		SessionID:                in.SessionID,
		TurnID:                   in.TurnID,
		PipelineVersion:          rolloutResult.PipelineVersion,
		PolicyResolutionSnapshot: policyResult.PolicyResolutionSnapshot,
	})
	if err != nil {
		return TurnStartBundle{}, fmt.Errorf("evaluate admission policy: %w", err)
	}

	leaseAuthorityValid := true
	if leaseResult.AuthorityEpochValid != nil {
		leaseAuthorityValid = *leaseResult.AuthorityEpochValid
	}
	leaseAuthorityGranted := true
	if leaseResult.AuthorityAuthorized != nil {
		leaseAuthorityGranted = *leaseResult.AuthorityAuthorized
	}

	cpScope := admissionResult.Scope
	if cpScope == "" {
		cpScope = controlplane.ScopeSession
	}

	bundle := TurnStartBundle{
		PipelineVersion:        rolloutResult.PipelineVersion,
		GraphDefinitionRef:     compiledGraph.GraphDefinitionRef,
		ExecutionProfile:       normalized.ExecutionProfile,
		GraphFingerprint:       compiledGraph.GraphFingerprint,
		AllowedAdaptiveActions: append([]string(nil), policyResult.AllowedAdaptiveActions...),
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       routingSnapshot.RoutingViewSnapshot,
			AdmissionPolicySnapshot:   routingSnapshot.AdmissionPolicySnapshot,
			ABICompatibilitySnapshot:  routingSnapshot.ABICompatibilitySnapshot,
			VersionResolutionSnapshot: rolloutResult.VersionResolutionSnapshot,
			PolicyResolutionSnapshot:  policyResult.PolicyResolutionSnapshot,
			ProviderHealthSnapshot:    providerHealthSnapshot.ProviderHealthSnapshot,
		},
		HasCPAdmissionDecision: true,
		CPAdmissionOutcomeKind: admissionResult.OutcomeKind,
		CPAdmissionScope:       cpScope,
		CPAdmissionReason:      admissionResult.Reason,
		HasLeaseDecision:       true,
		LeaseAuthorityEpoch:    leaseResult.AuthorityEpoch,
		LeaseAuthorityValid:    leaseAuthorityValid,
		LeaseAuthorityGranted:  leaseAuthorityGranted,
	}
	if err := bundle.Validate(); err != nil {
		return TurnStartBundle{}, err
	}
	return bundle, nil
}

func defaultSnapshotProvenance() controlplane.SnapshotProvenance {
	return controlplane.SnapshotProvenance{
		RoutingViewSnapshot:       "routing-view/v1",
		AdmissionPolicySnapshot:   "admission-policy/v1",
		ABICompatibilitySnapshot:  "abi-compat/v1",
		VersionResolutionSnapshot: "version-resolution/v1",
		PolicyResolutionSnapshot:  "policy-resolution/v1",
		ProviderHealthSnapshot:    "provider-health/v1",
	}
}

type staleSnapshotError interface {
	StaleSnapshot() bool
}

func isStaleSnapshotResolutionError(err error) bool {
	if err == nil {
		return false
	}
	var staleErr staleSnapshotError
	return errors.As(err, &staleErr) && staleErr.StaleSnapshot()
}
