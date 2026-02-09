package lanes

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestDefaultRouterResolveDeterministicTargets(t *testing.T) {
	t.Parallel()

	router := NewDefaultRouter()
	cases := []struct {
		name         string
		nodeType     string
		lane         eventabi.Lane
		expectedLane eventabi.Lane
		expectedKey  string
	}{
		{
			name:         "data lane",
			nodeType:     "provider",
			lane:         eventabi.LaneData,
			expectedLane: eventabi.LaneData,
			expectedKey:  "runtime/data/provider",
		},
		{
			name:         "control lane",
			nodeType:     "admission",
			lane:         eventabi.LaneControl,
			expectedLane: eventabi.LaneControl,
			expectedKey:  "runtime/control/admission",
		},
		{
			name:         "telemetry lane",
			nodeType:     "metrics",
			lane:         eventabi.LaneTelemetry,
			expectedLane: eventabi.LaneTelemetry,
			expectedKey:  "runtime/telemetry/metrics",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			target, err := router.Resolve(tc.nodeType, tc.lane)
			if err != nil {
				t.Fatalf("unexpected resolve error: %v", err)
			}
			if target.Lane != tc.expectedLane {
				t.Fatalf("expected lane %s, got %s", tc.expectedLane, target.Lane)
			}
			if target.QueueKey != tc.expectedKey {
				t.Fatalf("expected queue key %s, got %s", tc.expectedKey, target.QueueKey)
			}
		})
	}
}

func TestDefaultRouterResolveOverride(t *testing.T) {
	t.Parallel()

	router, err := NewDefaultRouterWithOverrides(map[string]DispatchTarget{
		routeKey("provider", eventabi.LaneData): {
			Lane:     eventabi.LaneData,
			QueueKey: "runtime/custom/provider-data",
		},
	})
	if err != nil {
		t.Fatalf("unexpected router override error: %v", err)
	}

	target, err := router.Resolve("provider", eventabi.LaneData)
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if target.QueueKey != "runtime/custom/provider-data" {
		t.Fatalf("expected override queue key, got %s", target.QueueKey)
	}
}

func TestDefaultRouterResolveValidation(t *testing.T) {
	t.Parallel()

	router := NewDefaultRouter()

	if _, err := router.Resolve("", eventabi.LaneData); err == nil {
		t.Fatalf("expected empty node type validation error")
	}
	if _, err := router.Resolve("provider", eventabi.Lane("UnknownLane")); err == nil {
		t.Fatalf("expected unknown lane validation error")
	}
}

func TestNewDefaultRouterWithOverridesValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewDefaultRouterWithOverrides(map[string]DispatchTarget{
		"provider|DataLane": {Lane: eventabi.LaneData},
	}); err == nil {
		t.Fatalf("expected missing queue key validation error")
	}

	if _, err := NewDefaultRouterWithOverrides(map[string]DispatchTarget{
		"provider|DataLane": {Lane: eventabi.Lane("BadLane"), QueueKey: "x"},
	}); err == nil {
		t.Fatalf("expected invalid lane validation error")
	}
}
