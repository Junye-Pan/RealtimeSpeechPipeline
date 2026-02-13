package httpadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

type providerIOCaptureMode string

const (
	captureModeRedacted providerIOCaptureMode = "redacted"
	captureModeFull     providerIOCaptureMode = "full"
	captureModeHash     providerIOCaptureMode = "hash"

	envProviderIOCaptureMode     = "RSPP_PROVIDER_IO_CAPTURE_MODE"
	envProviderIOCaptureMaxBytes = "RSPP_PROVIDER_IO_CAPTURE_MAX_BYTES"

	defaultProviderIOCaptureMode     = captureModeRedacted
	defaultProviderIOCaptureMaxBytes = 8192
	minProviderIOCaptureMaxBytes     = 256
)

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
	captureMode := resolveProviderIOCaptureMode()
	captureMaxBytes := resolveProviderIOCaptureMaxBytes()
	capturedInput, inputTruncated := capturePayload(body, captureMode, captureMaxBytes, false)

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
		outcome := normalizeNetworkError(err)
		outcome.InputPayload = capturedInput
		output, outputTruncated := capturePayload([]byte(fmt.Sprintf("network_error=%v", err)), captureMode, captureMaxBytes, false)
		outcome.OutputPayload = output
		outcome.PayloadTruncated = inputTruncated || outputTruncated
		return outcome, nil
	}
	defer resp.Body.Close()

	responseBody, responseTruncated, readErr := readBodySample(resp.Body, captureMaxBytes)
	outcome := normalizeStatus(resp.StatusCode, resp.Header.Get("Retry-After"))
	outcome.InputPayload = capturedInput
	if readErr != nil {
		responseBody = []byte(fmt.Sprintf("response_read_error=%v", readErr))
	}
	capturedOutput, outputTruncated := capturePayload(responseBody, captureMode, captureMaxBytes, responseTruncated)
	outcome.OutputPayload = capturedOutput
	outcome.PayloadTruncated = inputTruncated || outputTruncated
	return outcome, nil
}

// InvokeStream provides a compatibility streaming path for HTTP request/response adapters.
// It emits start/final (or error) events while preserving the unary invoke behavior.
func (a *Adapter) InvokeStream(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
	if observer == nil {
		observer = contracts.NoopStreamObserver{}
	}
	if err := req.Validate(); err != nil {
		return contracts.Outcome{}, err
	}

	start := streamChunkFromRequest(req, contracts.StreamChunkStart, 0)
	if err := observer.OnStart(start); err != nil {
		return contracts.Outcome{}, err
	}

	outcome, invokeErr := a.Invoke(req)
	if invokeErr != nil {
		errorChunk := streamChunkFromRequest(req, contracts.StreamChunkError, 1)
		errorChunk.ErrorReason = invokeErr.Error()
		if emitErr := observer.OnError(errorChunk); emitErr != nil {
			return contracts.Outcome{}, emitErr
		}
		return contracts.Outcome{}, invokeErr
	}

	if outcome.Class != contracts.OutcomeSuccess {
		errorChunk := streamChunkFromRequest(req, contracts.StreamChunkError, 1)
		errorChunk.ErrorReason = outcome.Reason
		if errorChunk.ErrorReason == "" {
			errorChunk.ErrorReason = "provider_stream_error"
		}
		if emitErr := observer.OnError(errorChunk); emitErr != nil {
			return contracts.Outcome{}, emitErr
		}
		return outcome, nil
	}

	final := streamChunkFromRequest(req, contracts.StreamChunkFinal, 1)
	final.TextFinal = outcome.OutputPayload
	final.Metadata = map[string]string{
		"outcome_class":       string(outcome.Class),
		"output_status_code":  strconv.Itoa(outcome.OutputStatusCode),
		"payload_truncated":   strconv.FormatBool(outcome.PayloadTruncated),
		"capture_mode":        string(resolveProviderIOCaptureMode()),
		"capture_max_bytes":   strconv.Itoa(resolveProviderIOCaptureMaxBytes()),
		"provider_invocation": req.ProviderInvocationID,
	}
	if err := observer.OnComplete(final); err != nil {
		return contracts.Outcome{}, err
	}
	return outcome, nil
}

func streamChunkFromRequest(req contracts.InvocationRequest, kind contracts.StreamChunkKind, sequence int) contracts.StreamChunk {
	return contracts.StreamChunk{
		SessionID:            req.SessionID,
		TurnID:               req.TurnID,
		PipelineVersion:      req.PipelineVersion,
		EventID:              req.EventID,
		ProviderInvocationID: req.ProviderInvocationID,
		ProviderID:           req.ProviderID,
		Modality:             req.Modality,
		Attempt:              req.Attempt,
		Sequence:             sequence,
		RuntimeTimestampMS:   req.RuntimeTimestampMS,
		WallClockTimestampMS: req.WallClockTimestampMS,
		Kind:                 kind,
	}
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
	outcome := contracts.Outcome{OutputStatusCode: status}
	switch {
	case status >= 200 && status <= 299:
		outcome.Class = contracts.OutcomeSuccess
		return outcome
	case status == http.StatusTooManyRequests:
		outcome.Class = contracts.OutcomeOverload
		outcome.Retryable = true
		outcome.Reason = "provider_overload"
		outcome.BackoffMS = retryAfterToMS(retryAfter)
		outcome.CircuitOpen = true
		return outcome
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		outcome.Class = contracts.OutcomeTimeout
		outcome.Retryable = true
		outcome.Reason = "provider_timeout"
		return outcome
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		outcome.Class = contracts.OutcomeBlocked
		outcome.Reason = "provider_auth_or_policy_block"
		return outcome
	case status >= 400 && status <= 499:
		outcome.Class = contracts.OutcomeBlocked
		outcome.Reason = "provider_client_error"
		return outcome
	default:
		outcome.Class = contracts.OutcomeInfrastructureFailure
		outcome.Retryable = true
		outcome.Reason = "provider_server_error"
		outcome.CircuitOpen = status >= 500
		return outcome
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

func resolveProviderIOCaptureMode() providerIOCaptureMode {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(envProviderIOCaptureMode)))
	switch providerIOCaptureMode(raw) {
	case captureModeFull, captureModeHash, captureModeRedacted:
		return providerIOCaptureMode(raw)
	default:
		return defaultProviderIOCaptureMode
	}
}

func resolveProviderIOCaptureMaxBytes() int {
	raw := strings.TrimSpace(os.Getenv(envProviderIOCaptureMaxBytes))
	if raw == "" {
		return defaultProviderIOCaptureMaxBytes
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minProviderIOCaptureMaxBytes {
		return defaultProviderIOCaptureMaxBytes
	}
	return value
}

func capturePayload(raw []byte, mode providerIOCaptureMode, maxBytes int, preTruncated bool) (string, bool) {
	if maxBytes < 1 {
		maxBytes = defaultProviderIOCaptureMaxBytes
	}
	truncated := preTruncated
	sample := raw
	if len(sample) > maxBytes {
		sample = sample[:maxBytes]
		truncated = true
	}
	switch mode {
	case captureModeFull:
		if len(sample) == 0 {
			return "", truncated
		}
		if utf8.Valid(sample) {
			return string(sample), truncated
		}
		return "base64:" + base64.StdEncoding.EncodeToString(sample), truncated
	case captureModeHash:
		return fmt.Sprintf("sha256=%s bytes=%d", hashBytes(sample), len(sample)), truncated
	default:
		return fmt.Sprintf("redacted sha256=%s bytes=%d", hashBytes(sample), len(sample)), truncated
	}
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func readBodySample(reader io.Reader, maxBytes int) ([]byte, bool, error) {
	if maxBytes < 1 {
		maxBytes = defaultProviderIOCaptureMaxBytes
	}
	payload, err := io.ReadAll(io.LimitReader(reader, int64(maxBytes+1)))
	if err != nil {
		return nil, false, err
	}
	if len(payload) > maxBytes {
		return payload[:maxBytes], true, nil
	}
	return payload, false, nil
}

// WithQuery appends/overrides a query key on an endpoint URL.
func WithQuery(rawEndpoint string, key string, value string) (string, error) {
	return withQuery(rawEndpoint, key, value)
}

// NormalizeNetworkError maps transport-level errors to normalized outcomes.
func NormalizeNetworkError(err error) contracts.Outcome {
	return normalizeNetworkError(err)
}

// NormalizeStatus maps HTTP status and retry-after headers to normalized outcomes.
func NormalizeStatus(status int, retryAfter string) contracts.Outcome {
	return normalizeStatus(status, retryAfter)
}

// ResolveProviderIOCaptureMode returns the active capture mode name.
func ResolveProviderIOCaptureMode() string {
	return string(resolveProviderIOCaptureMode())
}

// ResolveProviderIOCaptureMaxBytes returns the active capture byte limit.
func ResolveProviderIOCaptureMaxBytes() int {
	return resolveProviderIOCaptureMaxBytes()
}

// CapturePayload captures payload bytes using current env-based capture settings.
func CapturePayload(raw []byte, preTruncated bool) (string, bool) {
	return capturePayload(raw, resolveProviderIOCaptureMode(), resolveProviderIOCaptureMaxBytes(), preTruncated)
}

// CapturePayloadWithMode captures payload bytes using explicit mode/limit settings.
func CapturePayloadWithMode(raw []byte, mode string, maxBytes int, preTruncated bool) (string, bool) {
	normalized := providerIOCaptureMode(strings.ToLower(strings.TrimSpace(mode)))
	switch normalized {
	case captureModeFull, captureModeHash, captureModeRedacted:
	default:
		normalized = defaultProviderIOCaptureMode
	}
	if maxBytes < minProviderIOCaptureMaxBytes {
		maxBytes = defaultProviderIOCaptureMaxBytes
	}
	return capturePayload(raw, normalized, maxBytes, preTruncated)
}

// ReadBodySample reads at most maxBytes + 1 bytes and reports truncation.
func ReadBodySample(reader io.Reader, maxBytes int) ([]byte, bool, error) {
	return readBodySample(reader, maxBytes)
}
