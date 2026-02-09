package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

// Config configures a generic JSON-over-HTTP provider adapter.
type Config struct {
	ProviderID       string
	Modality         contracts.Modality
	Endpoint         string
	Method           string
	APIKey           string
	APIKeyHeader     string
	APIKeyPrefix     string
	QueryAPIKeyParam string
	StaticHeaders    map[string]string
	Timeout          time.Duration
	BuildBody        func(req contracts.InvocationRequest) any
}

// Adapter implements contracts.Adapter against a JSON-over-HTTP endpoint.
type Adapter struct {
	cfg    Config
	client *http.Client
}

// New constructs a generic HTTP adapter.
func New(cfg Config) (*Adapter, error) {
	if cfg.ProviderID == "" {
		return nil, fmt.Errorf("provider_id is required")
	}
	if err := cfg.Modality.Validate(); err != nil {
		return nil, err
	}
	if cfg.Method == "" {
		cfg.Method = http.MethodPost
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.BuildBody == nil {
		cfg.BuildBody = func(req contracts.InvocationRequest) any {
			return map[string]any{
				"provider_invocation_id": req.ProviderInvocationID,
				"event_id":               req.EventID,
			}
		}
	}
	if cfg.StaticHeaders == nil {
		cfg.StaticHeaders = map[string]string{}
	}
	return &Adapter{cfg: cfg, client: &http.Client{}}, nil
}

// ProviderID returns provider identity.
func (a *Adapter) ProviderID() string {
	return a.cfg.ProviderID
}

// Modality returns provider modality.
func (a *Adapter) Modality() contracts.Modality {
	return a.cfg.Modality
}

// Invoke executes one provider attempt and normalizes the outcome.
func (a *Adapter) Invoke(req contracts.InvocationRequest) (contracts.Outcome, error) {
	if err := req.Validate(); err != nil {
		return contracts.Outcome{}, err
	}
	if req.CancelRequested {
		return contracts.Outcome{Class: contracts.OutcomeCancelled, Retryable: false, Reason: "provider_cancelled"}, nil
	}
	if a.cfg.Endpoint == "" {
		return contracts.Outcome{Class: contracts.OutcomeBlocked, Retryable: false, Reason: "provider_endpoint_missing"}, nil
	}

	body, err := json.Marshal(a.cfg.BuildBody(req))
	if err != nil {
		return contracts.Outcome{}, err
	}

	endpoint := a.cfg.Endpoint
	if a.cfg.QueryAPIKeyParam != "" && a.cfg.APIKey != "" {
		endpoint, err = withQuery(endpoint, a.cfg.QueryAPIKeyParam, a.cfg.APIKey)
		if err != nil {
			return contracts.Outcome{}, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, a.cfg.Method, endpoint, bytes.NewReader(body))
	if err != nil {
		return contracts.Outcome{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.cfg.APIKeyHeader != "" && a.cfg.APIKey != "" {
		httpReq.Header.Set(a.cfg.APIKeyHeader, a.cfg.APIKeyPrefix+a.cfg.APIKey)
	}
	for key, value := range a.cfg.StaticHeaders {
		httpReq.Header.Set(key, value)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return normalizeNetworkError(err), nil
	}
	defer resp.Body.Close()

	return normalizeStatus(resp.StatusCode, resp.Header.Get("Retry-After")), nil
}

func withQuery(rawEndpoint string, key string, value string) (string, error) {
	u, err := url.Parse(rawEndpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func normalizeNetworkError(err error) contracts.Outcome {
	if errors.Is(err, context.Canceled) {
		return contracts.Outcome{Class: contracts.OutcomeCancelled, Retryable: false, Reason: "provider_cancelled"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return contracts.Outcome{Class: contracts.OutcomeTimeout, Retryable: true, Reason: "provider_timeout"}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return contracts.Outcome{Class: contracts.OutcomeTimeout, Retryable: true, Reason: "provider_timeout"}
	}
	return contracts.Outcome{Class: contracts.OutcomeInfrastructureFailure, Retryable: true, Reason: "provider_transport_error"}
}

func normalizeStatus(status int, retryAfter string) contracts.Outcome {
	switch {
	case status >= 200 && status <= 299:
		return contracts.Outcome{Class: contracts.OutcomeSuccess}
	case status == http.StatusTooManyRequests:
		return contracts.Outcome{
			Class:       contracts.OutcomeOverload,
			Retryable:   true,
			Reason:      "provider_overload",
			BackoffMS:   retryAfterToMS(retryAfter),
			CircuitOpen: true,
		}
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return contracts.Outcome{Class: contracts.OutcomeTimeout, Retryable: true, Reason: "provider_timeout"}
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return contracts.Outcome{Class: contracts.OutcomeBlocked, Retryable: false, Reason: "provider_auth_or_policy_block"}
	case status >= 400 && status <= 499:
		return contracts.Outcome{Class: contracts.OutcomeBlocked, Retryable: false, Reason: "provider_client_error"}
	default:
		return contracts.Outcome{Class: contracts.OutcomeInfrastructureFailure, Retryable: true, Reason: "provider_server_error", CircuitOpen: status >= 500}
	}
}

func retryAfterToMS(retryAfter string) int64 {
	if strings.TrimSpace(retryAfter) == "" {
		return 500
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter))
	if err != nil || seconds < 1 {
		return 500
	}
	return int64(seconds) * 1000
}
