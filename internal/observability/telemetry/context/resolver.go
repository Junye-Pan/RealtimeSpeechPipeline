package telemetrycontext

import (
	"fmt"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/telemetry"
)

const (
	defaultPipelineVersion = "pipeline-v1"
	defaultEmitter         = "OR-01"
)

// ResolveInput defines canonical correlation resolver inputs.
type ResolveInput struct {
	SessionID            string
	TurnID               string
	EventID              string
	PipelineVersion      string
	NodeID               string
	EdgeID               string
	AuthorityEpoch       int64
	Lane                 eventabi.Lane
	EmittedBy            string
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
}

// Resolver normalizes correlation IDs and default values for runtime telemetry.
type Resolver struct {
	DefaultPipelineVersion string
	DefaultLane            eventabi.Lane
	DefaultEmitter         string
}

// NewResolver returns canonical correlation resolver defaults.
func NewResolver() Resolver {
	return Resolver{
		DefaultPipelineVersion: defaultPipelineVersion,
		DefaultLane:            eventabi.LaneTelemetry,
		DefaultEmitter:         defaultEmitter,
	}
}

// Resolve returns normalized telemetry correlation values.
func Resolve(in ResolveInput) (telemetry.Correlation, error) {
	return NewResolver().Resolve(in)
}

// Resolve returns normalized telemetry correlation values.
func (r Resolver) Resolve(in ResolveInput) (telemetry.Correlation, error) {
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return telemetry.Correlation{}, fmt.Errorf("session_id is required")
	}
	eventID := strings.TrimSpace(in.EventID)
	if eventID == "" {
		return telemetry.Correlation{}, fmt.Errorf("event_id is required")
	}

	lane := in.Lane
	if lane == "" {
		lane = r.defaultLane()
	}
	if !isLane(lane) {
		return telemetry.Correlation{}, fmt.Errorf("invalid lane: %q", lane)
	}

	return telemetry.Correlation{
		SessionID:            sessionID,
		TurnID:               strings.TrimSpace(in.TurnID),
		EventID:              eventID,
		PipelineVersion:      firstNonEmpty(strings.TrimSpace(in.PipelineVersion), strings.TrimSpace(r.DefaultPipelineVersion), defaultPipelineVersion),
		NodeID:               strings.TrimSpace(in.NodeID),
		EdgeID:               strings.TrimSpace(in.EdgeID),
		AuthorityEpoch:       nonNegative(in.AuthorityEpoch),
		Lane:                 string(lane),
		EmittedBy:            firstNonEmpty(strings.TrimSpace(in.EmittedBy), strings.TrimSpace(r.DefaultEmitter), defaultEmitter),
		RuntimeTimestampMS:   nonNegative(in.RuntimeTimestampMS),
		WallClockTimestampMS: nonNegative(in.WallClockTimestampMS),
	}, nil
}

func (r Resolver) defaultLane() eventabi.Lane {
	if isLane(r.DefaultLane) {
		return r.DefaultLane
	}
	return eventabi.LaneTelemetry
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func isLane(lane eventabi.Lane) bool {
	switch lane {
	case eventabi.LaneControl, eventabi.LaneData, eventabi.LaneTelemetry:
		return true
	default:
		return false
	}
}
