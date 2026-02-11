package livekit

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

const (
	EnvURL              = "RSPP_LIVEKIT_URL"
	EnvAPIKey           = "RSPP_LIVEKIT_API_KEY"
	EnvAPISecret        = "RSPP_LIVEKIT_API_SECRET"
	EnvRoom             = "RSPP_LIVEKIT_ROOM"
	EnvSessionID        = "RSPP_LIVEKIT_SESSION_ID"
	EnvTurnID           = "RSPP_LIVEKIT_TURN_ID"
	EnvPipelineVersion  = "RSPP_LIVEKIT_PIPELINE_VERSION"
	EnvAuthorityEpoch   = "RSPP_LIVEKIT_AUTHORITY_EPOCH"
	EnvDefaultDataClass = "RSPP_LIVEKIT_DEFAULT_DATA_CLASS"
	EnvDryRun           = "RSPP_LIVEKIT_DRY_RUN"
	EnvProbe            = "RSPP_LIVEKIT_PROBE"
	EnvRequestTimeoutMS = "RSPP_LIVEKIT_REQUEST_TIMEOUT_MS"
)

// Config controls deterministic LiveKit adapter processing and optional room-service probing.
type Config struct {
	URL              string
	APIKey           string
	APISecret        string
	Room             string
	SessionID        string
	TurnID           string
	PipelineVersion  string
	AuthorityEpoch   int64
	DefaultDataClass eventabi.PayloadClass
	DryRun           bool
	Probe            bool
	RequestTimeout   time.Duration
}

// ConfigFromEnv loads runtime/operator LiveKit settings from RSPP_LIVEKIT_* variables.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		URL:              strings.TrimSpace(os.Getenv(EnvURL)),
		APIKey:           strings.TrimSpace(os.Getenv(EnvAPIKey)),
		APISecret:        strings.TrimSpace(os.Getenv(EnvAPISecret)),
		Room:             defaultString(strings.TrimSpace(os.Getenv(EnvRoom)), "rspp-default-room"),
		SessionID:        defaultString(strings.TrimSpace(os.Getenv(EnvSessionID)), "sess-livekit-runtime"),
		TurnID:           defaultString(strings.TrimSpace(os.Getenv(EnvTurnID)), "turn-livekit-runtime"),
		PipelineVersion:  defaultString(strings.TrimSpace(os.Getenv(EnvPipelineVersion)), "pipeline-v1"),
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           true,
		Probe:            false,
		RequestTimeout:   5 * time.Second,
	}

	if raw := strings.TrimSpace(os.Getenv(EnvAuthorityEpoch)); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", EnvAuthorityEpoch, err)
		}
		cfg.AuthorityEpoch = v
	}
	if raw := strings.TrimSpace(os.Getenv(EnvDefaultDataClass)); raw != "" {
		cfg.DefaultDataClass = eventabi.PayloadClass(raw)
	}
	if raw := strings.TrimSpace(os.Getenv(EnvDryRun)); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", EnvDryRun, err)
		}
		cfg.DryRun = v
	}
	if raw := strings.TrimSpace(os.Getenv(EnvProbe)); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", EnvProbe, err)
		}
		cfg.Probe = v
	}
	if raw := strings.TrimSpace(os.Getenv(EnvRequestTimeoutMS)); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", EnvRequestTimeoutMS, err)
		}
		cfg.RequestTimeout = time.Duration(v) * time.Millisecond
	}

	return cfg, cfg.Validate()
}

// Validate enforces deterministic and operator-safe config invariants.
func (c Config) Validate() error {
	if c.SessionID == "" || c.TurnID == "" || c.PipelineVersion == "" {
		return fmt.Errorf("session_id, turn_id, and pipeline_version are required")
	}
	if c.AuthorityEpoch < 0 {
		return fmt.Errorf("authority_epoch must be >=0")
	}
	switch c.DefaultDataClass {
	case eventabi.PayloadAudioRaw, eventabi.PayloadTextRaw, eventabi.PayloadDerivedSummary:
	default:
		return fmt.Errorf("default_data_class must be one of audio_raw, text_raw, derived_summary")
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be >0")
	}

	needsCredentials := c.Probe || !c.DryRun
	if needsCredentials {
		if c.URL == "" || c.APIKey == "" || c.APISecret == "" {
			return fmt.Errorf("livekit url/api credentials are required when probe enabled or dry_run=false")
		}
	}
	return nil
}

func defaultString(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
