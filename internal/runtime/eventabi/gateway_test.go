package eventabi

import (
	"strings"
	"testing"

	apieventabi "github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestValidateAndNormalizeControlSignalsDefaults(t *testing.T) {
	t.Parallel()

	in := []apieventabi.ControlSignal{
		{
			EventScope:         apieventabi.ScopeSession,
			SessionID:          "sess-1",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-1",
			Lane:               apieventabi.LaneControl,
			RuntimeSequence:    5,
			AuthorityEpoch:     1,
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			PayloadClass:       apieventabi.PayloadMetadata,
			Signal:             "shed",
			EmittedBy:          "RK-25",
			Reason:             "scheduling_point_shed",
		},
	}

	out, err := ValidateAndNormalizeControlSignals(in)
	if err != nil {
		t.Fatalf("unexpected normalize/validate error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected single signal, got %d", len(out))
	}
	if out[0].SchemaVersion != "v1.0" {
		t.Fatalf("expected default schema version v1.0, got %q", out[0].SchemaVersion)
	}
	if out[0].Scope != "session" {
		t.Fatalf("expected default scope=session, got %q", out[0].Scope)
	}
	if out[0].TransportSequence == nil || *out[0].TransportSequence != 0 {
		t.Fatalf("expected default transport sequence 0, got %v", out[0].TransportSequence)
	}
}

func TestValidateAndNormalizeControlSignalsSequenceRegression(t *testing.T) {
	t.Parallel()

	seq1 := int64(10)
	seq2 := int64(9)
	_, err := ValidateAndNormalizeControlSignals([]apieventabi.ControlSignal{
		{
			SchemaVersion:      "v1.0",
			EventScope:         apieventabi.ScopeSession,
			SessionID:          "sess-1",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-1",
			Lane:               apieventabi.LaneControl,
			TransportSequence:  &seq1,
			RuntimeSequence:    10,
			AuthorityEpoch:     1,
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			PayloadClass:       apieventabi.PayloadMetadata,
			Signal:             "shed",
			EmittedBy:          "RK-25",
			Reason:             "scheduling_point_shed",
		},
		{
			SchemaVersion:      "v1.0",
			EventScope:         apieventabi.ScopeSession,
			SessionID:          "sess-1",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-2",
			Lane:               apieventabi.LaneControl,
			TransportSequence:  &seq2,
			RuntimeSequence:    9,
			AuthorityEpoch:     1,
			RuntimeTimestampMS: 101,
			WallClockMS:        101,
			PayloadClass:       apieventabi.PayloadMetadata,
			Signal:             "shed",
			EmittedBy:          "RK-25",
			Reason:             "scheduling_point_shed",
		},
	})
	if err == nil {
		t.Fatalf("expected sequence regression error")
	}
	if !strings.Contains(err.Error(), "regression") {
		t.Fatalf("expected regression error, got %v", err)
	}
}

func TestValidateAndNormalizeControlSignalsTurnScopeRequiresTurnID(t *testing.T) {
	t.Parallel()

	_, err := ValidateAndNormalizeControlSignals([]apieventabi.ControlSignal{
		{
			EventScope:         apieventabi.ScopeTurn,
			SessionID:          "sess-1",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-1",
			Lane:               apieventabi.LaneControl,
			RuntimeSequence:    1,
			AuthorityEpoch:     1,
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			PayloadClass:       apieventabi.PayloadMetadata,
			Signal:             "turn_open",
			EmittedBy:          "RK-03",
		},
	})
	if err == nil {
		t.Fatalf("expected turn scope validation error")
	}
	if !strings.Contains(err.Error(), "turn_id") {
		t.Fatalf("expected turn_id-related error, got %v", err)
	}
}

func TestValidateAndNormalizeEventRecordsDefaults(t *testing.T) {
	t.Parallel()

	in := []apieventabi.EventRecord{
		{
			EventScope:         apieventabi.ScopeSession,
			SessionID:          "sess-ev-1",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-ev-1",
			Lane:               apieventabi.LaneData,
			RuntimeSequence:    3,
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			PayloadClass:       apieventabi.PayloadTextRaw,
		},
	}
	out, err := ValidateAndNormalizeEventRecords(in)
	if err != nil {
		t.Fatalf("unexpected event record normalize/validate error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected single event record, got %d", len(out))
	}
	if out[0].SchemaVersion != "v1.0" {
		t.Fatalf("expected default schema version v1.0, got %q", out[0].SchemaVersion)
	}
	if out[0].TransportSequence == nil || *out[0].TransportSequence != 0 {
		t.Fatalf("expected default transport sequence 0, got %v", out[0].TransportSequence)
	}
	if out[0].AuthorityEpoch == nil || *out[0].AuthorityEpoch != 0 {
		t.Fatalf("expected default authority epoch 0, got %v", out[0].AuthorityEpoch)
	}
}

func TestValidateAndNormalizeEventRecordsSequenceRegression(t *testing.T) {
	t.Parallel()

	seq1 := int64(2)
	seq2 := int64(1)
	_, err := ValidateAndNormalizeEventRecords([]apieventabi.EventRecord{
		{
			SchemaVersion:      "v1.0",
			EventScope:         apieventabi.ScopeSession,
			SessionID:          "sess-ev-2",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-ev-1",
			Lane:               apieventabi.LaneData,
			TransportSequence:  &seq1,
			RuntimeSequence:    2,
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			PayloadClass:       apieventabi.PayloadTextRaw,
		},
		{
			SchemaVersion:      "v1.0",
			EventScope:         apieventabi.ScopeSession,
			SessionID:          "sess-ev-2",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-ev-2",
			Lane:               apieventabi.LaneData,
			TransportSequence:  &seq2,
			RuntimeSequence:    1,
			RuntimeTimestampMS: 101,
			WallClockMS:        101,
			PayloadClass:       apieventabi.PayloadTextRaw,
		},
	})
	if err == nil {
		t.Fatalf("expected event record sequence regression error")
	}
	if !strings.Contains(err.Error(), "regression") {
		t.Fatalf("expected regression error, got %v", err)
	}
}

func TestValidateAndNormalizeEventRecordsTurnScopeRequiresTurnID(t *testing.T) {
	t.Parallel()

	_, err := ValidateAndNormalizeEventRecords([]apieventabi.EventRecord{
		{
			EventScope:         apieventabi.ScopeTurn,
			SessionID:          "sess-ev-3",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-ev-3",
			Lane:               apieventabi.LaneData,
			RuntimeSequence:    1,
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			PayloadClass:       apieventabi.PayloadTextRaw,
		},
	})
	if err == nil {
		t.Fatalf("expected turn scope validation error for event record")
	}
	if !strings.Contains(err.Error(), "turn_id") {
		t.Fatalf("expected turn_id-related error, got %v", err)
	}
}
