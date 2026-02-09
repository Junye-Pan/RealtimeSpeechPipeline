package transport

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestTagIngressEventRecordDefaults(t *testing.T) {
	t.Parallel()

	record, err := TagIngressEventRecord(eventabi.EventRecord{
		Lane: eventabi.LaneData,
	}, IngressClassificationConfig{})
	if err != nil {
		t.Fatalf("unexpected data-lane tag error: %v", err)
	}
	if record.PayloadClass != eventabi.PayloadTextRaw {
		t.Fatalf("expected default data class text_raw, got %s", record.PayloadClass)
	}

	telemetry, err := TagIngressEventRecord(eventabi.EventRecord{
		Lane: eventabi.LaneTelemetry,
	}, IngressClassificationConfig{})
	if err != nil {
		t.Fatalf("unexpected telemetry-lane tag error: %v", err)
	}
	if telemetry.PayloadClass != eventabi.PayloadMetadata {
		t.Fatalf("expected telemetry default class metadata, got %s", telemetry.PayloadClass)
	}
}

func TestTagIngressEventRecordCustomAndInvalid(t *testing.T) {
	t.Parallel()

	custom, err := TagIngressEventRecord(eventabi.EventRecord{
		Lane: eventabi.LaneData,
	}, IngressClassificationConfig{DefaultDataClass: eventabi.PayloadAudioRaw})
	if err != nil {
		t.Fatalf("unexpected custom class tag error: %v", err)
	}
	if custom.PayloadClass != eventabi.PayloadAudioRaw {
		t.Fatalf("expected custom default class audio_raw, got %s", custom.PayloadClass)
	}

	if _, err := TagIngressEventRecord(eventabi.EventRecord{
		Lane: eventabi.LaneData,
	}, IngressClassificationConfig{DefaultDataClass: "unknown"}); err == nil {
		t.Fatalf("expected invalid custom default class error")
	}

	preserved, err := TagIngressEventRecord(eventabi.EventRecord{
		Lane:         eventabi.LaneData,
		PayloadClass: eventabi.PayloadPII,
	}, IngressClassificationConfig{})
	if err != nil {
		t.Fatalf("unexpected preserved class error: %v", err)
	}
	if preserved.PayloadClass != eventabi.PayloadPII {
		t.Fatalf("expected payload class to remain PII, got %s", preserved.PayloadClass)
	}
}

func TestTagIngressEventRecordRejectsControlLane(t *testing.T) {
	t.Parallel()

	if _, err := TagIngressEventRecord(eventabi.EventRecord{Lane: eventabi.LaneControl}, IngressClassificationConfig{}); err == nil {
		t.Fatalf("expected control-lane ingress classification error")
	}
}
