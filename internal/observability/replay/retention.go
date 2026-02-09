package replay

import (
	"errors"
	"fmt"
	"sync"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

const (
	defaultPIIRetentionLimitMS int64 = 7 * 24 * 60 * 60 * 1000
	defaultPHIRetentionLimitMS int64 = 24 * 60 * 60 * 1000
	defaultClassRetentionMS    int64 = 30 * 24 * 60 * 60 * 1000
)

var (
	ErrReplayArtifactNotFound                      = errors.New("replay artifact not found")
	ErrReplayArtifactCryptographicallyInaccessible = errors.New("replay artifact cryptographically inaccessible")
)

// DeletionMode defines retention/deletion outcomes for replay artifacts.
type DeletionMode string

const (
	DeletionModeHardDelete         DeletionMode = "hard_delete"
	DeletionModeCryptoInaccessible DeletionMode = "crypto_inaccessible"
)

// Validate ensures deletion mode is supported.
func (m DeletionMode) Validate() error {
	switch m {
	case DeletionModeHardDelete, DeletionModeCryptoInaccessible:
		return nil
	default:
		return fmt.Errorf("invalid deletion_mode: %q", m)
	}
}

// DeletionScope defines tenant-scoped session/turn targeting for delete requests.
type DeletionScope struct {
	TenantID  string
	SessionID string
	TurnID    string
}

// Validate enforces tenant scope and a single selector.
func (s DeletionScope) Validate() error {
	if s.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	hasSession := s.SessionID != ""
	hasTurn := s.TurnID != ""
	switch {
	case hasSession && hasTurn:
		return fmt.Errorf("exactly one delete scope selector is required: session_id or turn_id")
	case !hasSession && !hasTurn:
		return fmt.Errorf("delete scope requires session_id or turn_id")
	default:
		return nil
	}
}

func (s DeletionScope) matches(record ReplayArtifactRecord) bool {
	if record.TenantID != s.TenantID {
		return false
	}
	if s.SessionID != "" {
		return record.SessionID == s.SessionID
	}
	return record.TurnID == s.TurnID
}

// DeletionRequest captures baseline replay artifact deletion contract attributes.
type DeletionRequest struct {
	Scope         DeletionScope
	Mode          DeletionMode
	RequestedBy   string
	Reason        string
	RequestedAtMS int64
}

// Validate enforces baseline deletion request requirements.
func (r DeletionRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.Mode.Validate(); err != nil {
		return err
	}
	if r.RequestedBy == "" {
		return fmt.Errorf("requested_by is required")
	}
	if r.RequestedAtMS < 0 {
		return fmt.Errorf("requested_at_ms must be >=0")
	}
	return nil
}

// DeletionResult captures deletion behavior for an applied request.
type DeletionResult struct {
	Scope                                  DeletionScope
	Mode                                   DeletionMode
	MatchedArtifacts                       int
	DeletedArtifacts                       int
	CryptographicallyInaccessibleArtifacts int
}

// RetentionPolicy declares tenant-scoped class retention windows.
type RetentionPolicy struct {
	TenantID              string
	DefaultRetentionMS    int64
	PIIRetentionLimitMS   int64
	PHIRetentionLimitMS   int64
	MaxRetentionByClassMS map[eventabi.PayloadClass]int64
}

// DefaultRetentionPolicy returns a conservative tenant-scoped policy baseline.
func DefaultRetentionPolicy(tenantID string) RetentionPolicy {
	return RetentionPolicy{
		TenantID:            tenantID,
		DefaultRetentionMS:  defaultClassRetentionMS,
		PIIRetentionLimitMS: defaultPIIRetentionLimitMS,
		PHIRetentionLimitMS: defaultPHIRetentionLimitMS,
		MaxRetentionByClassMS: map[eventabi.PayloadClass]int64{
			eventabi.PayloadAudioRaw:       defaultClassRetentionMS,
			eventabi.PayloadTextRaw:        defaultClassRetentionMS,
			eventabi.PayloadPII:            defaultPIIRetentionLimitMS,
			eventabi.PayloadPHI:            defaultPHIRetentionLimitMS,
			eventabi.PayloadDerivedSummary: defaultClassRetentionMS,
			eventabi.PayloadMetadata:       defaultClassRetentionMS,
		},
	}
}

// Validate enforces retention policy invariants.
func (p RetentionPolicy) Validate() error {
	if p.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if p.DefaultRetentionMS < 1 {
		return fmt.Errorf("default_retention_ms must be >=1")
	}
	if p.PIIRetentionLimitMS < 1 {
		return fmt.Errorf("pii_retention_limit_ms must be >=1")
	}
	if p.PHIRetentionLimitMS < 1 {
		return fmt.Errorf("phi_retention_limit_ms must be >=1")
	}
	if len(p.MaxRetentionByClassMS) == 0 {
		return fmt.Errorf("max_retention_by_class_ms must not be empty")
	}

	for class, windowMS := range p.MaxRetentionByClassMS {
		if err := validatePayloadClass(class); err != nil {
			return err
		}
		if windowMS < 1 {
			return fmt.Errorf("retention window for class %s must be >=1", class)
		}
	}

	piiWindow, ok := p.MaxRetentionByClassMS[eventabi.PayloadPII]
	if !ok {
		return fmt.Errorf("retention window for class %s is required", eventabi.PayloadPII)
	}
	if piiWindow > p.PIIRetentionLimitMS {
		return fmt.Errorf("retention window for class %s exceeds policy limit", eventabi.PayloadPII)
	}
	phiWindow, ok := p.MaxRetentionByClassMS[eventabi.PayloadPHI]
	if !ok {
		return fmt.Errorf("retention window for class %s is required", eventabi.PayloadPHI)
	}
	if phiWindow > p.PHIRetentionLimitMS {
		return fmt.Errorf("retention window for class %s exceeds policy limit", eventabi.PayloadPHI)
	}

	return nil
}

func (p RetentionPolicy) retentionWindowFor(class eventabi.PayloadClass) int64 {
	if windowMS, ok := p.MaxRetentionByClassMS[class]; ok {
		return windowMS
	}
	return p.DefaultRetentionMS
}

// RetentionPolicyResolver resolves tenant-scoped retention policy.
type RetentionPolicyResolver interface {
	ResolveRetentionPolicy(tenantID string) (RetentionPolicy, error)
}

// StaticRetentionPolicyResolver resolves from in-memory defaults and overrides.
type StaticRetentionPolicyResolver struct {
	DefaultPolicy  RetentionPolicy
	TenantPolicies map[string]RetentionPolicy
}

// ResolveRetentionPolicy resolves and validates tenant policy.
func (r StaticRetentionPolicyResolver) ResolveRetentionPolicy(tenantID string) (RetentionPolicy, error) {
	if tenantID == "" {
		return RetentionPolicy{}, fmt.Errorf("tenant_id is required")
	}

	if policy, ok := r.TenantPolicies[tenantID]; ok {
		if policy.TenantID == "" {
			policy.TenantID = tenantID
		}
		if policy.TenantID != tenantID {
			return RetentionPolicy{}, fmt.Errorf("retention policy tenant mismatch: expected %s got %s", tenantID, policy.TenantID)
		}
		if err := policy.Validate(); err != nil {
			return RetentionPolicy{}, err
		}
		return policy, nil
	}

	policy := r.DefaultPolicy
	if policy.TenantID == "" {
		policy = DefaultRetentionPolicy(tenantID)
	}
	if policy.TenantID != tenantID {
		policy.TenantID = tenantID
	}
	if err := policy.Validate(); err != nil {
		return RetentionPolicy{}, err
	}
	return policy, nil
}

// ArtifactState defines replay artifact accessibility state.
type ArtifactState string

const (
	ArtifactStateActive                        ArtifactState = "active"
	ArtifactStateCryptographicallyInaccessible ArtifactState = "cryptographically_inaccessible"
)

func normalizeArtifactState(state ArtifactState) ArtifactState {
	if state == "" {
		return ArtifactStateActive
	}
	return state
}

// ReplayArtifactRecord is the baseline store record for replay artifacts.
type ReplayArtifactRecord struct {
	ArtifactID   string
	TenantID     string
	SessionID    string
	TurnID       string
	PayloadClass eventabi.PayloadClass
	RecordedAtMS int64
	State        ArtifactState
}

// Validate enforces baseline replay artifact contract requirements.
func (r ReplayArtifactRecord) Validate() error {
	if r.ArtifactID == "" {
		return fmt.Errorf("artifact_id is required")
	}
	if r.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if err := validatePayloadClass(r.PayloadClass); err != nil {
		return err
	}
	if r.RecordedAtMS < 0 {
		return fmt.Errorf("recorded_at_ms must be >=0")
	}
	switch normalizeArtifactState(r.State) {
	case ArtifactStateActive, ArtifactStateCryptographicallyInaccessible:
		return nil
	default:
		return fmt.Errorf("invalid artifact state: %q", r.State)
	}
}

func validatePayloadClass(class eventabi.PayloadClass) error {
	return (eventabi.RedactionDecision{PayloadClass: class, Action: eventabi.RedactionAllow}).Validate()
}

// RetentionSweepResult summarizes retention enforcement behavior.
type RetentionSweepResult struct {
	EvaluatedArtifacts int
	ExpiredArtifacts   int
	DeletedArtifacts   int
}

// InMemoryArtifactStore is a deterministic scaffold store for replay retention/deletion.
type InMemoryArtifactStore struct {
	mu        sync.Mutex
	artifacts []ReplayArtifactRecord
}

// NewInMemoryArtifactStore constructs an empty scaffold replay artifact store.
func NewInMemoryArtifactStore() *InMemoryArtifactStore {
	return &InMemoryArtifactStore{}
}

// Add stores a replay artifact record.
func (s *InMemoryArtifactStore) Add(record ReplayArtifactRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}

	record.State = normalizeArtifactState(record.State)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts = append(s.artifacts, record)
	return nil
}

// Snapshot returns a stable copy of replay artifact records.
func (s *InMemoryArtifactStore) Snapshot() []ReplayArtifactRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]ReplayArtifactRecord, len(s.artifacts))
	copy(copied, s.artifacts)
	return copied
}

// Read returns a replay artifact if still accessible.
func (s *InMemoryArtifactStore) Read(tenantID string, artifactID string) (ReplayArtifactRecord, error) {
	if tenantID == "" || artifactID == "" {
		return ReplayArtifactRecord{}, fmt.Errorf("tenant_id and artifact_id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.artifacts {
		if record.TenantID != tenantID || record.ArtifactID != artifactID {
			continue
		}
		if normalizeArtifactState(record.State) == ArtifactStateCryptographicallyInaccessible {
			return ReplayArtifactRecord{}, ErrReplayArtifactCryptographicallyInaccessible
		}
		return record, nil
	}
	return ReplayArtifactRecord{}, ErrReplayArtifactNotFound
}

// EnforceRetention deletes tenant artifacts that exceed class retention windows.
func (s *InMemoryArtifactStore) EnforceRetention(policy RetentionPolicy, nowMS int64) (RetentionSweepResult, error) {
	if err := policy.Validate(); err != nil {
		return RetentionSweepResult{}, err
	}
	if nowMS < 0 {
		return RetentionSweepResult{}, fmt.Errorf("now_ms must be >=0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := RetentionSweepResult{}
	retained := make([]ReplayArtifactRecord, 0, len(s.artifacts))

	for _, record := range s.artifacts {
		if record.TenantID != policy.TenantID {
			retained = append(retained, record)
			continue
		}
		result.EvaluatedArtifacts++
		if normalizeArtifactState(record.State) != ArtifactStateActive {
			retained = append(retained, record)
			continue
		}
		windowMS := policy.retentionWindowFor(record.PayloadClass)
		if isExpired(record.RecordedAtMS, nowMS, windowMS) {
			result.ExpiredArtifacts++
			result.DeletedArtifacts++
			continue
		}
		retained = append(retained, record)
	}

	s.artifacts = retained
	return result, nil
}

func isExpired(recordedAtMS int64, nowMS int64, windowMS int64) bool {
	ageMS := nowMS - recordedAtMS
	if ageMS < 0 {
		return false
	}
	return ageMS > windowMS
}

// Delete applies a tenant-scoped deletion request.
func (s *InMemoryArtifactStore) Delete(req DeletionRequest) (DeletionResult, error) {
	if err := req.Validate(); err != nil {
		return DeletionResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := DeletionResult{Scope: req.Scope, Mode: req.Mode}
	retained := make([]ReplayArtifactRecord, 0, len(s.artifacts))

	for _, record := range s.artifacts {
		if !req.Scope.matches(record) {
			retained = append(retained, record)
			continue
		}

		result.MatchedArtifacts++
		switch req.Mode {
		case DeletionModeHardDelete:
			result.DeletedArtifacts++
		case DeletionModeCryptoInaccessible:
			if normalizeArtifactState(record.State) != ArtifactStateCryptographicallyInaccessible {
				result.CryptographicallyInaccessibleArtifacts++
			}
			record.State = ArtifactStateCryptographicallyInaccessible
			retained = append(retained, record)
		default:
			return DeletionResult{}, fmt.Errorf("unsupported deletion mode: %q", req.Mode)
		}
	}

	s.artifacts = retained
	return result, nil
}
