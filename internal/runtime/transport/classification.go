package transport

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// IngressClassificationConfig controls deterministic RK-22 ingress tagging defaults.
type IngressClassificationConfig struct {
	DefaultDataClass eventabi.PayloadClass
}

// TagIngressEventRecord applies required RK-22 payload classification tags before RK-05 validation.
func TagIngressEventRecord(record eventabi.EventRecord, cfg IngressClassificationConfig) (eventabi.EventRecord, error) {
	if record.Lane == eventabi.LaneControl {
		return eventabi.EventRecord{}, fmt.Errorf("ingress event records cannot use control lane")
	}

	if record.PayloadClass != "" {
		if !isPayloadClass(record.PayloadClass) {
			return eventabi.EventRecord{}, fmt.Errorf("invalid ingress payload class: %s", record.PayloadClass)
		}
		return record, nil
	}

	switch record.Lane {
	case eventabi.LaneTelemetry:
		record.PayloadClass = eventabi.PayloadMetadata
	case eventabi.LaneData:
		record.PayloadClass = cfg.DefaultDataClass
		if record.PayloadClass == "" {
			record.PayloadClass = eventabi.PayloadTextRaw
		}
		if !isPayloadClass(record.PayloadClass) {
			return eventabi.EventRecord{}, fmt.Errorf("invalid ingress default data class: %s", record.PayloadClass)
		}
	default:
		return eventabi.EventRecord{}, fmt.Errorf("invalid ingress lane: %s", record.Lane)
	}

	return record, nil
}

func isPayloadClass(class eventabi.PayloadClass) bool {
	switch class {
	case eventabi.PayloadPII, eventabi.PayloadPHI, eventabi.PayloadAudioRaw, eventabi.PayloadTextRaw, eventabi.PayloadDerivedSummary, eventabi.PayloadMetadata:
		return true
	default:
		return false
	}
}
