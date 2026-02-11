package livekit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ProbeStatusSkipped = "skipped"
	ProbeStatusPassed  = "passed"
	ProbeStatusFailed  = "failed"
)

// ProbeResult reports deterministic connectivity evidence for LiveKit room-service probing.
type ProbeResult struct {
	Status     string `json:"status"`
	URL        string `json:"url,omitempty"`
	Room       string `json:"room,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	RoomCount  int    `json:"room_count,omitempty"`
}

// HTTPClient is the transport seam used for room-service probing.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ProbeRoomService validates API auth and reachability against LiveKit RoomService ListRooms.
func ProbeRoomService(ctx context.Context, cfg Config, client HTTPClient, now func() time.Time) (ProbeResult, error) {
	result := ProbeResult{
		Status: ProbeStatusSkipped,
		URL:    cfg.URL,
		Room:   cfg.Room,
	}
	if !cfg.Probe {
		return result, nil
	}
	if now == nil {
		now = time.Now
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.RequestTimeout}
	}
	if cfg.URL == "" || cfg.APIKey == "" || cfg.APISecret == "" {
		result.Status = ProbeStatusFailed
		return result, fmt.Errorf("probe requires livekit url/api credentials")
	}

	token, err := buildAPIToken(cfg, now())
	if err != nil {
		result.Status = ProbeStatusFailed
		return result, err
	}

	requestBody := map[string][]string{"names": {cfg.Room}}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Status = ProbeStatusFailed
		return result, err
	}

	endpoint := strings.TrimRight(cfg.URL, "/") + "/twirp/livekit.RoomService/ListRooms"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		result.Status = ProbeStatusFailed
		return result, fmt.Errorf("build room-service request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		result.Status = ProbeStatusFailed
		return result, fmt.Errorf("probe room-service endpoint: %w", err)
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		result.Status = ProbeStatusFailed
		return result, fmt.Errorf("read probe response: %w", err)
	}
	if resp.StatusCode >= 300 {
		result.Status = ProbeStatusFailed
		return result, fmt.Errorf("livekit room-service probe failed status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed struct {
		Rooms []struct {
			Name string `json:"name"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		result.Status = ProbeStatusFailed
		return result, fmt.Errorf("decode probe response: %w", err)
	}
	result.Status = ProbeStatusPassed
	result.RoomCount = len(parsed.Rooms)
	return result, nil
}

func buildAPIToken(cfg Config, now time.Time) (string, error) {
	if cfg.APIKey == "" || cfg.APISecret == "" {
		return "", fmt.Errorf("missing livekit api credentials")
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	payload := map[string]any{
		"iss": cfg.APIKey,
		"sub": cfg.SessionID,
		"nbf": now.Add(-10 * time.Second).Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"video": map[string]any{
			"roomList":  true,
			"roomAdmin": true,
			"room":      cfg.Room,
		},
	}

	headerRaw, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(headerRaw) + "." + base64.RawURLEncoding.EncodeToString(payloadRaw)

	mac := hmac.New(sha256.New, []byte(cfg.APISecret))
	if _, err := mac.Write([]byte(unsigned)); err != nil {
		return "", err
	}
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}
