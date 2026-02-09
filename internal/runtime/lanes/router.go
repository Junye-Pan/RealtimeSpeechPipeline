package lanes

import (
	"fmt"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// LaneID is the deterministic scheduler-visible lane identifier.
type LaneID string

const (
	LaneIDData      LaneID = "data"
	LaneIDControl   LaneID = "control"
	LaneIDTelemetry LaneID = "telemetry"
)

// DispatchTarget defines where a node dispatch is routed.
type DispatchTarget struct {
	Lane     eventabi.Lane
	QueueKey string
}

// Router resolves runtime dispatch targets for lane/node pairs.
type Router interface {
	Resolve(nodeType string, lane eventabi.Lane) (DispatchTarget, error)
}

// DefaultRouter performs deterministic lane routing with optional overrides.
type DefaultRouter struct {
	overrides map[string]DispatchTarget
}

// NewDefaultRouter returns a deterministic lane router.
func NewDefaultRouter() DefaultRouter {
	return DefaultRouter{
		overrides: map[string]DispatchTarget{},
	}
}

// NewDefaultRouterWithOverrides returns a default router with explicit route overrides.
func NewDefaultRouterWithOverrides(overrides map[string]DispatchTarget) (DefaultRouter, error) {
	r := NewDefaultRouter()
	for key, target := range overrides {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			return DefaultRouter{}, fmt.Errorf("override key is required")
		}
		if err := validateLane(target.Lane); err != nil {
			return DefaultRouter{}, err
		}
		if strings.TrimSpace(target.QueueKey) == "" {
			return DefaultRouter{}, fmt.Errorf("override queue key is required for %s", normalizedKey)
		}
		r.overrides[normalizedKey] = target
	}
	return r, nil
}

// Resolve returns deterministic routing for a node type and lane.
func (r DefaultRouter) Resolve(nodeType string, lane eventabi.Lane) (DispatchTarget, error) {
	nodeType = strings.TrimSpace(nodeType)
	if nodeType == "" {
		return DispatchTarget{}, fmt.Errorf("node_type is required")
	}
	if err := validateLane(lane); err != nil {
		return DispatchTarget{}, err
	}

	key := routeKey(nodeType, lane)
	if target, ok := r.overrides[key]; ok {
		return target, nil
	}
	return DispatchTarget{
		Lane:     lane,
		QueueKey: fmt.Sprintf("runtime/%s/%s", laneIDFor(lane), strings.ToLower(nodeType)),
	}, nil
}

func routeKey(nodeType string, lane eventabi.Lane) string {
	return strings.ToLower(strings.TrimSpace(nodeType)) + "|" + string(lane)
}

func validateLane(lane eventabi.Lane) error {
	switch lane {
	case eventabi.LaneData, eventabi.LaneControl, eventabi.LaneTelemetry:
		return nil
	default:
		return fmt.Errorf("unsupported lane: %q", lane)
	}
}

func laneIDFor(lane eventabi.Lane) LaneID {
	switch lane {
	case eventabi.LaneData:
		return LaneIDData
	case eventabi.LaneControl:
		return LaneIDControl
	default:
		return LaneIDTelemetry
	}
}
