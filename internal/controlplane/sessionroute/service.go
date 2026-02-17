package sessionroute

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/lease"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/rollout"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/routingview"
)

const (
	defaultTransportKind = "livekit"
	defaultRegion        = "global"
	defaultRuntimeHost   = "runtime.rspp.local"
	defaultTokenTTL      = 5 * time.Minute
	defaultTokenSecret   = "rspp-session-token-dev-secret"
)

// Service provides session route, token, and status APIs for CP-03.
type Service struct {
	Registry    registry.Service
	Rollout     rollout.Service
	RoutingView routingview.Service
	Lease       lease.Service

	DefaultTransportKind string
	DefaultRegion        string
	RuntimeHost          string
	DefaultTokenTTL      time.Duration
	TokenSecret          string
	Now                  func() time.Time

	mu       sync.RWMutex
	statuses map[string]controlplane.SessionStatusView
}

// NewService returns a deterministic CP-03 session route/token/status service.
func NewService() Service {
	return Service{
		Registry:             registry.NewService(),
		Rollout:              rollout.NewService(),
		RoutingView:          routingview.NewService(),
		Lease:                lease.NewService(),
		DefaultTransportKind: defaultTransportKind,
		DefaultRegion:        defaultRegion,
		RuntimeHost:          defaultRuntimeHost,
		DefaultTokenTTL:      defaultTokenTTL,
		TokenSecret:          defaultTokenSecret,
		Now:                  time.Now,
		statuses:             map[string]controlplane.SessionStatusView{},
	}
}

// ResolveSessionRoute resolves deterministic route metadata for a session bootstrap.
func (s *Service) ResolveSessionRoute(req controlplane.SessionRouteRequest) (controlplane.SessionRoute, error) {
	if err := req.Validate(); err != nil {
		return controlplane.SessionRoute{}, err
	}

	pipelineVersion, err := s.resolvePipelineVersion(req)
	if err != nil {
		return controlplane.SessionRoute{}, err
	}

	routingSnapshot, err := s.routingViewService().GetSnapshot(routingview.Input{
		SessionID:       req.SessionID,
		PipelineVersion: pipelineVersion,
		AuthorityEpoch:  req.RequestedAuthorityEpoch,
		TenantID:        req.TenantID,
		TransportKind:   req.TransportKind,
	})
	if err != nil {
		return controlplane.SessionRoute{}, fmt.Errorf("resolve routing snapshot: %w", err)
	}

	leaseOutput, err := s.leaseService().Resolve(lease.Input{
		SessionID:               req.SessionID,
		PipelineVersion:         pipelineVersion,
		RequestedAuthorityEpoch: req.RequestedAuthorityEpoch,
	})
	if err != nil {
		return controlplane.SessionRoute{}, fmt.Errorf("resolve lease authority: %w", err)
	}

	authorityValid := true
	if leaseOutput.AuthorityEpochValid != nil {
		authorityValid = *leaseOutput.AuthorityEpochValid
	}
	authorityGranted := true
	if leaseOutput.AuthorityAuthorized != nil {
		authorityGranted = *leaseOutput.AuthorityAuthorized
	}

	transportKind := s.transportKind(req.TransportKind)
	if strings.TrimSpace(req.TransportKind) == "" && strings.TrimSpace(routingSnapshot.TransportKind) != "" {
		transportKind = strings.TrimSpace(routingSnapshot.TransportKind)
	}
	endpoint := strings.TrimSpace(routingSnapshot.TransportEndpoint)
	if endpoint == "" {
		endpoint = s.runtimeEndpoint(transportKind, pipelineVersion)
	}
	runtimeID := strings.TrimSpace(routingSnapshot.RuntimeID)
	if runtimeID == "" {
		runtimeID = s.runtimeID(req.TenantID, req.SessionID)
	}

	route := controlplane.SessionRoute{
		TenantID:                req.TenantID,
		SessionID:               req.SessionID,
		PipelineVersion:         pipelineVersion,
		RoutingViewSnapshot:     routingSnapshot.RoutingViewSnapshot,
		AdmissionPolicySnapshot: routingSnapshot.AdmissionPolicySnapshot,
		Endpoint: controlplane.TransportEndpointRef{
			TransportKind: transportKind,
			Endpoint:      endpoint,
			Region:        s.region(),
			RuntimeID:     runtimeID,
		},
		Lease: controlplane.PlacementLease{
			AuthorityEpoch: leaseOutput.AuthorityEpoch,
			Granted:        authorityGranted,
			Valid:          authorityValid,
			TokenRef: controlplane.LeaseTokenRef{
				TokenID:      leaseOutput.LeaseTokenID,
				ExpiresAtUTC: leaseOutput.LeaseExpiresAtUTC,
			},
		},
	}
	if err := route.Validate(); err != nil {
		return controlplane.SessionRoute{}, err
	}

	_ = s.SetSessionStatus(controlplane.SessionStatusView{
		TenantID:        route.TenantID,
		SessionID:       route.SessionID,
		Status:          controlplane.SessionStatusConnected,
		PipelineVersion: route.PipelineVersion,
		AuthorityEpoch:  route.Lease.AuthorityEpoch,
		UpdatedAtUTC:    s.now().UTC().Format(time.RFC3339),
		LastReason:      "session_route_resolved",
	})

	return route, nil
}

// IssueSessionToken issues a signed, short-lived token for session bootstrap.
func (s *Service) IssueSessionToken(req controlplane.SessionTokenRequest) (controlplane.SignedSessionToken, error) {
	if err := req.Validate(); err != nil {
		return controlplane.SignedSessionToken{}, err
	}

	route, err := s.ResolveSessionRoute(controlplane.SessionRouteRequest{
		TenantID:                 req.TenantID,
		SessionID:                req.SessionID,
		RequestedPipelineVersion: req.RequestedPipelineVersion,
		TransportKind:            req.TransportKind,
		RequestedAuthorityEpoch:  req.RequestedAuthorityEpoch,
	})
	if err != nil {
		return controlplane.SignedSessionToken{}, err
	}

	now := s.now().UTC()
	ttl := s.tokenTTL(req.TTLSeconds)
	expiresAt := now.Add(ttl).UTC()
	tokenID := strings.TrimSpace(route.Lease.TokenRef.TokenID)
	if tokenID == "" {
		tokenID = s.syntheticTokenID(route.TenantID, route.SessionID, route.PipelineVersion, now)
	}

	claims := controlplane.SessionTokenClaims{
		TenantID:        route.TenantID,
		SessionID:       route.SessionID,
		PipelineVersion: route.PipelineVersion,
		TransportKind:   route.Endpoint.TransportKind,
		AuthorityEpoch:  route.Lease.AuthorityEpoch,
		IssuedAtUTC:     now.Format(time.RFC3339),
		ExpiresAtUTC:    expiresAt.Format(time.RFC3339),
	}

	token, err := s.signToken(tokenID, claims)
	if err != nil {
		return controlplane.SignedSessionToken{}, err
	}
	out := controlplane.SignedSessionToken{
		Token:        token,
		TokenID:      tokenID,
		ExpiresAtUTC: claims.ExpiresAtUTC,
		Claims:       claims,
	}
	if err := out.Validate(); err != nil {
		return controlplane.SignedSessionToken{}, err
	}

	_ = s.SetSessionStatus(controlplane.SessionStatusView{
		TenantID:        route.TenantID,
		SessionID:       route.SessionID,
		Status:          controlplane.SessionStatusConnected,
		PipelineVersion: route.PipelineVersion,
		AuthorityEpoch:  route.Lease.AuthorityEpoch,
		UpdatedAtUTC:    now.Format(time.RFC3339),
		LastReason:      "session_token_issued",
	})

	return out, nil
}

// GetSessionStatus returns the latest known status for a tenant/session.
func (s *Service) GetSessionStatus(req controlplane.SessionStatusRequest) (controlplane.SessionStatusView, error) {
	if err := req.Validate(); err != nil {
		return controlplane.SessionStatusView{}, err
	}
	key := sessionKey(req.TenantID, req.SessionID)

	s.mu.RLock()
	view, ok := s.statuses[key]
	s.mu.RUnlock()
	if ok {
		return view, nil
	}

	out := controlplane.SessionStatusView{
		TenantID:     req.TenantID,
		SessionID:    req.SessionID,
		Status:       controlplane.SessionStatusEnded,
		UpdatedAtUTC: s.now().UTC().Format(time.RFC3339),
		LastReason:   "session_not_found",
	}
	if err := out.Validate(); err != nil {
		return controlplane.SessionStatusView{}, err
	}
	return out, nil
}

// SetSessionStatus writes a status value for subsequent lookups.
func (s *Service) SetSessionStatus(status controlplane.SessionStatusView) error {
	if strings.TrimSpace(status.UpdatedAtUTC) == "" {
		status.UpdatedAtUTC = s.now().UTC().Format(time.RFC3339)
	}
	if err := status.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = map[string]controlplane.SessionStatusView{}
	}
	s.statuses[sessionKey(status.TenantID, status.SessionID)] = status
	return nil
}

func (s *Service) resolvePipelineVersion(req controlplane.SessionRouteRequest) (string, error) {
	record, err := s.registryService().ResolvePipelineRecord(req.RequestedPipelineVersion)
	if err != nil {
		return "", fmt.Errorf("resolve pipeline record: %w", err)
	}
	out, err := s.rolloutService().ResolvePipelineVersion(rollout.ResolveVersionInput{
		TenantID:                 req.TenantID,
		SessionID:                req.SessionID,
		RequestedPipelineVersion: req.RequestedPipelineVersion,
		RegistryPipelineVersion:  record.PipelineVersion,
	})
	if err != nil {
		return "", fmt.Errorf("resolve rollout pipeline version: %w", err)
	}
	if strings.TrimSpace(out.PipelineVersion) == "" {
		return "", fmt.Errorf("pipeline_version is required")
	}
	return out.PipelineVersion, nil
}

func (s *Service) signToken(tokenID string, claims controlplane.SessionTokenClaims) (string, error) {
	payload, err := json.Marshal(struct {
		TokenID string                          `json:"token_id"`
		Claims  controlplane.SessionTokenClaims `json:"claims"`
	}{
		TokenID: tokenID,
		Claims:  claims,
	})
	if err != nil {
		return "", fmt.Errorf("encode session token payload: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(s.tokenSecret()))
	_, _ = mac.Write(payload)
	signature := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *Service) syntheticTokenID(tenantID string, sessionID string, pipelineVersion string, now time.Time) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(sessionID),
		strings.TrimSpace(pipelineVersion),
		now.UTC().Format(time.RFC3339Nano),
	}, "|")))
	return hex.EncodeToString(sum[:8])
}

func (s *Service) runtimeEndpoint(transportKind string, pipelineVersion string) string {
	host := strings.TrimSpace(s.RuntimeHost)
	if host == "" {
		host = defaultRuntimeHost
	}
	return fmt.Sprintf("wss://%s/%s/%s", host, strings.TrimSpace(transportKind), strings.TrimSpace(pipelineVersion))
}

func (s *Service) runtimeID(tenantID string, sessionID string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(sessionID),
	}, "|")))
	return "runtime-" + hex.EncodeToString(sum[:4])
}

func (s *Service) transportKind(requested string) string {
	kind := strings.TrimSpace(requested)
	if kind != "" {
		return kind
	}
	kind = strings.TrimSpace(s.DefaultTransportKind)
	if kind != "" {
		return kind
	}
	return defaultTransportKind
}

func (s *Service) tokenTTL(requestedSeconds int) time.Duration {
	if requestedSeconds > 0 {
		return time.Duration(requestedSeconds) * time.Second
	}
	if s.DefaultTokenTTL > 0 {
		return s.DefaultTokenTTL
	}
	return defaultTokenTTL
}

func (s *Service) tokenSecret() string {
	secret := strings.TrimSpace(s.TokenSecret)
	if secret != "" {
		return secret
	}
	return defaultTokenSecret
}

func (s *Service) region() string {
	region := strings.TrimSpace(s.DefaultRegion)
	if region != "" {
		return region
	}
	return defaultRegion
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) registryService() registry.Service {
	if s.Registry.DefaultRecord == (registry.PipelineRecord{}) &&
		len(s.Registry.Records) == 0 &&
		s.Registry.Backend == nil {
		return registry.NewService()
	}
	return s.Registry
}

func (s *Service) rolloutService() rollout.Service {
	if s.Rollout.DefaultVersionResolutionSnapshot == "" && s.Rollout.Backend == nil &&
		isZeroRolloutPolicy(s.Rollout.Policy) {
		return rollout.NewService()
	}
	return s.Rollout
}

func (s *Service) routingViewService() routingview.Service {
	if s.RoutingView.DefaultSnapshot == (routingview.Snapshot{}) && s.RoutingView.Backend == nil {
		return routingview.NewService()
	}
	return s.RoutingView
}

func (s *Service) leaseService() lease.Service {
	if s.Lease.DefaultLeaseResolutionSnapshot == "" && s.Lease.Backend == nil {
		return lease.NewService()
	}
	return s.Lease
}

func sessionKey(tenantID string, sessionID string) string {
	return strings.TrimSpace(tenantID) + "/" + strings.TrimSpace(sessionID)
}

func isZeroRolloutPolicy(in rollout.Policy) bool {
	return in.BasePipelineVersion == "" &&
		in.CanaryPipelineVersion == "" &&
		in.CanaryPercentage == 0 &&
		len(in.TenantAllowlist) == 0
}
