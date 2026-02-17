package authority

import (
	"context"
	"errors"
	"fmt"

	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
)

var (
	// ErrUnauthorizedToken indicates token validation failure at transport boundary.
	ErrUnauthorizedToken = errors.New("transport token unauthorized")
	// ErrStaleAuthorityEpoch indicates stale epoch at ingress/egress boundary.
	ErrStaleAuthorityEpoch = errors.New("stale authority epoch")
)

// TokenValidator validates transport session tokens issued by control plane.
type TokenValidator interface {
	ValidateSessionToken(ctx context.Context, tokenRef string, sessionID string, tenantID string, pipelineVersion string) error
}

// Guard validates bootstrap token and authority epoch boundaries.
type Guard struct {
	validator TokenValidator
}

// NewGuard creates an authority guard.
func NewGuard(validator TokenValidator) Guard {
	return Guard{validator: validator}
}

// ValidateBootstrap validates shared bootstrap contract and token authority.
func (g Guard) ValidateBootstrap(ctx context.Context, bootstrap apitransport.SessionBootstrap) error {
	if err := bootstrap.Validate(); err != nil {
		return err
	}
	if g.validator == nil {
		return nil
	}
	if err := g.validator.ValidateSessionToken(ctx, bootstrap.LeaseTokenRef, bootstrap.SessionID, bootstrap.TenantID, bootstrap.PipelineVersion); err != nil {
		return fmt.Errorf("%w: %v", ErrUnauthorizedToken, err)
	}
	return nil
}

// ValidateIngress enforces stale-epoch rejection for ingress events.
func (g Guard) ValidateIngress(expectedEpoch int64, observedEpoch int64) error {
	if observedEpoch < expectedEpoch {
		return fmt.Errorf("%w: ingress epoch %d < expected %d", ErrStaleAuthorityEpoch, observedEpoch, expectedEpoch)
	}
	return nil
}

// ValidateEgress enforces stale-epoch rejection for egress events.
func (g Guard) ValidateEgress(expectedEpoch int64, observedEpoch int64) error {
	if observedEpoch < expectedEpoch {
		return fmt.Errorf("%w: egress epoch %d < expected %d", ErrStaleAuthorityEpoch, observedEpoch, expectedEpoch)
	}
	return nil
}
