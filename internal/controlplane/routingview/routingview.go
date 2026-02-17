package routingview

import "fmt"

const (
	defaultRoutingViewSnapshot      = "routing-view/v1"
	defaultAdmissionPolicySnapshot  = "admission-policy/v1"
	defaultABICompatibilitySnapshot = "abi-compat/v1"
	defaultTransportKind            = "livekit"
	defaultTransportEndpoint        = "wss://runtime.rspp.local/livekit/pipeline-v1"
	defaultRuntimeID                = "runtime-default"
)

// Snapshot is the CP-08 runtime-consumed routing snapshot bundle.
type Snapshot struct {
	RoutingViewSnapshot      string
	AdmissionPolicySnapshot  string
	ABICompatibilitySnapshot string
	TransportKind            string
	TransportEndpoint        string
	RuntimeID                string
}

// Validate enforces required CP-08 snapshot references.
func (s Snapshot) Validate() error {
	if s.RoutingViewSnapshot == "" {
		return fmt.Errorf("routing_view_snapshot is required")
	}
	if s.AdmissionPolicySnapshot == "" {
		return fmt.Errorf("admission_policy_snapshot is required")
	}
	if s.ABICompatibilitySnapshot == "" {
		return fmt.Errorf("abi_compatibility_snapshot is required")
	}
	if s.TransportKind == "" {
		return fmt.Errorf("transport_kind is required")
	}
	if s.TransportEndpoint == "" {
		return fmt.Errorf("transport_endpoint is required")
	}
	if s.RuntimeID == "" {
		return fmt.Errorf("runtime_id is required")
	}
	return nil
}

// Input models CP-08 routing snapshot selection context.
type Input struct {
	SessionID       string
	PipelineVersion string
	AuthorityEpoch  int64
	TenantID        string
	TransportKind   string
}

// Backend resolves routing snapshots from a snapshot-fed control-plane source.
type Backend interface {
	GetSnapshot(in Input) (Snapshot, error)
}

// Service returns deterministic CP-08 snapshot references.
type Service struct {
	DefaultSnapshot Snapshot
	Backend         Backend
}

// NewService returns the baseline CP-08 snapshot publisher.
func NewService() Service {
	return Service{
		DefaultSnapshot: Snapshot{
			RoutingViewSnapshot:      defaultRoutingViewSnapshot,
			AdmissionPolicySnapshot:  defaultAdmissionPolicySnapshot,
			ABICompatibilitySnapshot: defaultABICompatibilitySnapshot,
		},
	}
}

// GetSnapshot returns a read-optimized routing snapshot for runtime turn-start.
func (s Service) GetSnapshot(in Input) (Snapshot, error) {
	if in.SessionID == "" {
		return Snapshot{}, fmt.Errorf("session_id is required")
	}
	if in.AuthorityEpoch < 0 {
		return Snapshot{}, fmt.Errorf("authority_epoch must be >=0")
	}

	if s.Backend != nil {
		snapshot, err := s.Backend.GetSnapshot(in)
		if err != nil {
			return Snapshot{}, fmt.Errorf("resolve routing snapshot backend: %w", err)
		}
		return s.normalizeSnapshot(snapshot)
	}

	return s.normalizeSnapshot(s.DefaultSnapshot)
}

func (s Service) normalizeSnapshot(snapshot Snapshot) (Snapshot, error) {
	if snapshot.RoutingViewSnapshot == "" {
		snapshot.RoutingViewSnapshot = defaultRoutingViewSnapshot
	}
	if snapshot.AdmissionPolicySnapshot == "" {
		snapshot.AdmissionPolicySnapshot = defaultAdmissionPolicySnapshot
	}
	if snapshot.ABICompatibilitySnapshot == "" {
		snapshot.ABICompatibilitySnapshot = defaultABICompatibilitySnapshot
	}
	if snapshot.TransportKind == "" {
		snapshot.TransportKind = defaultTransportKind
	}
	if snapshot.TransportEndpoint == "" {
		snapshot.TransportEndpoint = defaultTransportEndpoint
	}
	if snapshot.RuntimeID == "" {
		snapshot.RuntimeID = defaultRuntimeID
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}
