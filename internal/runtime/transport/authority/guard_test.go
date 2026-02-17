package authority

import (
	"context"
	"errors"
	"testing"

	apitransport "github.com/tiger/realtime-speech-pipeline/api/transport"
)

type tokenValidatorStub struct {
	err error
}

func (s tokenValidatorStub) ValidateSessionToken(ctx context.Context, tokenRef string, sessionID string, tenantID string, pipelineVersion string) error {
	return s.err
}

func TestValidateBootstrapWithTokenValidator(t *testing.T) {
	t.Parallel()

	bootstrap := apitransport.SessionBootstrap{
		SchemaVersion:   "v1.0",
		TransportKind:   apitransport.TransportLiveKit,
		TenantID:        "tenant-a",
		SessionID:       "sess-auth-1",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  3,
		LeaseTokenRef:   "token-ref",
		RouteRef:        "route-ref",
		RequestedAtMS:   10,
	}

	guard := NewGuard(tokenValidatorStub{})
	if err := guard.ValidateBootstrap(context.Background(), bootstrap); err != nil {
		t.Fatalf("expected bootstrap validation to pass, got %v", err)
	}

	guard = NewGuard(tokenValidatorStub{err: errors.New("signature mismatch")})
	if err := guard.ValidateBootstrap(context.Background(), bootstrap); !errors.Is(err, ErrUnauthorizedToken) {
		t.Fatalf("expected unauthorized token error, got %v", err)
	}
}

func TestValidateIngressEgressStaleEpoch(t *testing.T) {
	t.Parallel()

	guard := NewGuard(nil)
	if err := guard.ValidateIngress(5, 4); !errors.Is(err, ErrStaleAuthorityEpoch) {
		t.Fatalf("expected stale ingress epoch error, got %v", err)
	}
	if err := guard.ValidateEgress(5, 4); !errors.Is(err, ErrStaleAuthorityEpoch) {
		t.Fatalf("expected stale egress epoch error, got %v", err)
	}
	if err := guard.ValidateIngress(5, 5); err != nil {
		t.Fatalf("expected equal ingress epoch to pass, got %v", err)
	}
	if err := guard.ValidateEgress(5, 6); err != nil {
		t.Fatalf("expected newer egress epoch to pass, got %v", err)
	}
}
