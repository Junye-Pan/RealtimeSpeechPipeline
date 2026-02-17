package ingress

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestNormalizeDefaultsPayloadClassAndValidatesABI(t *testing.T) {
	t.Parallel()

	transportSequence := int64(1)
	authorityEpoch := int64(2)
	record := eventabi.EventRecord{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeTurn,
		SessionID:          "sess-ingress-1",
		TurnID:             "turn-ingress-1",
		PipelineVersion:    "pipeline-v1",
		EventID:            "evt-ingress-1",
		Lane:               eventabi.LaneData,
		TransportSequence:  &transportSequence,
		RuntimeSequence:    1,
		AuthorityEpoch:     &authorityEpoch,
		RuntimeTimestampMS: 10,
		WallClockMS:        10,
	}

	normalized, err := Normalize(NormalizeInput{
		Record:           record,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		SourceCodec:      "opus",
	})
	if err != nil {
		t.Fatalf("normalize ingress: %v", err)
	}
	if normalized.PayloadClass != eventabi.PayloadAudioRaw {
		t.Fatalf("expected default data class audio_raw, got %s", normalized.PayloadClass)
	}
}

func TestNormalizeRejectsUnsupportedCodec(t *testing.T) {
	t.Parallel()

	transportSequence := int64(1)
	authorityEpoch := int64(2)
	record := eventabi.EventRecord{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeTurn,
		SessionID:          "sess-ingress-2",
		TurnID:             "turn-ingress-2",
		PipelineVersion:    "pipeline-v1",
		EventID:            "evt-ingress-2",
		Lane:               eventabi.LaneData,
		TransportSequence:  &transportSequence,
		RuntimeSequence:    1,
		AuthorityEpoch:     &authorityEpoch,
		RuntimeTimestampMS: 10,
		WallClockMS:        10,
	}

	_, err := Normalize(NormalizeInput{Record: record, SourceCodec: "flac"})
	if err == nil {
		t.Fatalf("expected unsupported codec to fail normalization")
	}
}
