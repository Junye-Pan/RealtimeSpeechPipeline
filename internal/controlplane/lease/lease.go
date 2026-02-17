package lease

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const defaultLeaseResolutionSnapshot = "lease-resolution/v1"

const defaultLeaseTokenTTL = 5 * time.Minute

const (
	// ReasonLeaseAuthorized marks lease-authorized turn-start paths.
	ReasonLeaseAuthorized = "lease_authorized"
	// ReasonLeaseStaleEpoch marks lease epoch mismatch paths.
	ReasonLeaseStaleEpoch = "lease_stale_epoch"
	// ReasonLeaseDeauthorized marks lease deauthorization paths.
	ReasonLeaseDeauthorized = "lease_deauthorized"
	// ReasonLeaseInvalidInput marks lease input validation failures.
	ReasonLeaseInvalidInput = "lease_invalid_input"
)

// Input models CP-07 lease authority resolution context.
type Input struct {
	SessionID               string
	PipelineVersion         string
	RequestedAuthorityEpoch int64
}

// Output is the deterministic CP-07 lease authority artifact.
type Output struct {
	LeaseResolutionSnapshot string
	AuthorityEpoch          int64
	AuthorityEpochValid     *bool
	AuthorityAuthorized     *bool
	Reason                  string
	LeaseTokenID            string
	LeaseExpiresAtUTC       string
}

// Backend resolves lease authority output from a snapshot-fed control-plane source.
type Backend interface {
	Resolve(in Input) (Output, error)
}

// Service resolves deterministic CP-07 lease authority state.
type Service struct {
	DefaultLeaseResolutionSnapshot string
	DefaultLeaseTokenTTL           time.Duration
	Now                            func() time.Time
	Backend                        Backend
}

// NewService returns deterministic CP-07 baseline lease defaults.
func NewService() Service {
	return Service{
		DefaultLeaseResolutionSnapshot: defaultLeaseResolutionSnapshot,
		DefaultLeaseTokenTTL:           defaultLeaseTokenTTL,
		Now:                            time.Now,
	}
}

// Resolve evaluates lease authority state for pre-turn gating.
func (s Service) Resolve(in Input) (Output, error) {
	if in.SessionID == "" {
		return Output{}, fmt.Errorf("%s: session_id is required", ReasonLeaseInvalidInput)
	}
	if in.RequestedAuthorityEpoch < 0 {
		return Output{}, fmt.Errorf("%s: requested_authority_epoch must be >=0", ReasonLeaseInvalidInput)
	}

	if s.Backend != nil {
		out, err := s.Backend.Resolve(in)
		if err != nil {
			return Output{}, fmt.Errorf("resolve lease backend: %w", err)
		}
		return s.normalizeOutput(in, out), nil
	}

	return s.normalizeOutput(in, Output{}), nil
}

func (s Service) normalizeOutput(in Input, out Output) Output {
	snapshot := s.DefaultLeaseResolutionSnapshot
	if snapshot == "" {
		snapshot = defaultLeaseResolutionSnapshot
	}
	if out.LeaseResolutionSnapshot != "" {
		snapshot = out.LeaseResolutionSnapshot
	}

	epoch := out.AuthorityEpoch
	if epoch == 0 {
		epoch = in.RequestedAuthorityEpoch
	}
	if epoch < 0 {
		epoch = 0
	}

	authorityEpochValid := boolValue(out.AuthorityEpochValid, true)
	authorityAuthorized := boolValue(out.AuthorityAuthorized, true)
	now := s.now().UTC()

	leaseTokenID := strings.TrimSpace(out.LeaseTokenID)
	if leaseTokenID == "" {
		leaseTokenID = syntheticLeaseTokenID(in.SessionID, epoch)
	}

	leaseExpiresAtUTC := strings.TrimSpace(out.LeaseExpiresAtUTC)
	tokenExpired := false
	if leaseExpiresAtUTC != "" {
		expiresAt, err := time.Parse(time.RFC3339, leaseExpiresAtUTC)
		if err != nil {
			leaseExpiresAtUTC = ""
		} else if !expiresAt.After(now) {
			tokenExpired = true
		}
	}
	if leaseExpiresAtUTC == "" {
		leaseExpiresAtUTC = now.Add(s.tokenTTL()).Format(time.RFC3339)
		tokenExpired = false
	}
	if tokenExpired {
		authorityAuthorized = false
	}

	reason := out.Reason
	if reason == "" {
		switch {
		case !authorityEpochValid:
			reason = ReasonLeaseStaleEpoch
		case !authorityAuthorized:
			reason = ReasonLeaseDeauthorized
		default:
			reason = ReasonLeaseAuthorized
		}
	}

	return Output{
		LeaseResolutionSnapshot: snapshot,
		AuthorityEpoch:          epoch,
		AuthorityEpochValid:     boolPtr(authorityEpochValid),
		AuthorityAuthorized:     boolPtr(authorityAuthorized),
		Reason:                  reason,
		LeaseTokenID:            leaseTokenID,
		LeaseExpiresAtUTC:       leaseExpiresAtUTC,
	}
}

func boolValue(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}

func syntheticLeaseTokenID(sessionID string, authorityEpoch int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", strings.TrimSpace(sessionID), authorityEpoch)))
	return "lease-" + hex.EncodeToString(sum[:8])
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Service) tokenTTL() time.Duration {
	if s.DefaultLeaseTokenTTL > 0 {
		return s.DefaultLeaseTokenTTL
	}
	return defaultLeaseTokenTTL
}
