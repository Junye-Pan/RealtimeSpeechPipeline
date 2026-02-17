package telemetrycontext

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestResolveNormalizesAndDefaults(t *testing.T) {
	t.Parallel()

	correlation, err := Resolve(ResolveInput{
		SessionID:            " sess-1 ",
		TurnID:               " turn-1 ",
		EventID:              " evt-1 ",
		PipelineVersion:      " pipeline-custom ",
		NodeID:               " node-a ",
		EdgeID:               " edge-a ",
		AuthorityEpoch:       7,
		Lane:                 eventabi.LaneData,
		EmittedBy:            " OR-99 ",
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 201,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if correlation.SessionID != "sess-1" || correlation.TurnID != "turn-1" || correlation.EventID != "evt-1" {
		t.Fatalf("expected trimmed identity fields, got %+v", correlation)
	}
	if correlation.PipelineVersion != "pipeline-custom" || correlation.NodeID != "node-a" || correlation.EdgeID != "edge-a" {
		t.Fatalf("expected trimmed pipeline/node/edge fields, got %+v", correlation)
	}
	if correlation.Lane != string(eventabi.LaneData) || correlation.EmittedBy != "OR-99" {
		t.Fatalf("expected lane/emitter values to be preserved, got %+v", correlation)
	}
}

func TestResolveAppliesDefaultPipelineLaneAndEmitter(t *testing.T) {
	t.Parallel()

	correlation, err := Resolve(ResolveInput{
		SessionID: "sess-1",
		EventID:   "evt-1",
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if correlation.PipelineVersion != defaultPipelineVersion {
		t.Fatalf("expected default pipeline version %q, got %+v", defaultPipelineVersion, correlation)
	}
	if correlation.Lane != string(eventabi.LaneTelemetry) {
		t.Fatalf("expected default telemetry lane, got %+v", correlation)
	}
	if correlation.EmittedBy != defaultEmitter {
		t.Fatalf("expected default emitter %q, got %+v", defaultEmitter, correlation)
	}
}

func TestResolveRejectsMissingRequiredIDs(t *testing.T) {
	t.Parallel()

	if _, err := Resolve(ResolveInput{EventID: "evt-1"}); err == nil {
		t.Fatalf("expected missing session_id to fail")
	}
	if _, err := Resolve(ResolveInput{SessionID: "sess-1"}); err == nil {
		t.Fatalf("expected missing event_id to fail")
	}
}

func TestResolveRejectsInvalidLane(t *testing.T) {
	t.Parallel()

	if _, err := Resolve(ResolveInput{
		SessionID: "sess-1",
		EventID:   "evt-1",
		Lane:      eventabi.Lane("sideband"),
	}); err == nil {
		t.Fatalf("expected invalid lane to fail")
	}
}

func TestResolveNormalizesNegativeNumericFields(t *testing.T) {
	t.Parallel()

	correlation, err := Resolve(ResolveInput{
		SessionID:            "sess-1",
		EventID:              "evt-1",
		AuthorityEpoch:       -1,
		RuntimeTimestampMS:   -5,
		WallClockTimestampMS: -9,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if correlation.AuthorityEpoch != 0 || correlation.RuntimeTimestampMS != 0 || correlation.WallClockTimestampMS != 0 {
		t.Fatalf("expected negative numeric fields to normalize to zero, got %+v", correlation)
	}
}
