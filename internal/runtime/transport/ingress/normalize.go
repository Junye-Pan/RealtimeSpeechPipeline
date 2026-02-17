package ingress

import (
	"fmt"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
)

// NormalizeInput captures ingress normalization inputs.
type NormalizeInput struct {
	Record           eventabi.EventRecord
	DefaultDataClass eventabi.PayloadClass
	SourceCodec      string
}

// Normalize converts transport ingress records into ABI-compliant runtime events.
func Normalize(in NormalizeInput) (eventabi.EventRecord, error) {
	if err := validateCodec(in.Record.Lane, in.SourceCodec); err != nil {
		return eventabi.EventRecord{}, err
	}
	tagged, err := runtimetransport.TagIngressEventRecord(in.Record, runtimetransport.IngressClassificationConfig{
		DefaultDataClass: in.DefaultDataClass,
	})
	if err != nil {
		return eventabi.EventRecord{}, err
	}
	if tagged.Lane == eventabi.LaneData && tagged.PayloadClass == eventabi.PayloadAudioRaw && tagged.MediaTime == nil {
		pts := tagged.RuntimeTimestampMS
		tagged.MediaTime = &eventabi.MediaTime{PTSMS: &pts}
	}
	normalized, err := runtimeeventabi.ValidateAndNormalizeEventRecords([]eventabi.EventRecord{tagged})
	if err != nil {
		return eventabi.EventRecord{}, err
	}
	return normalized[0], nil
}

func validateCodec(lane eventabi.Lane, rawCodec string) error {
	codec := strings.ToLower(strings.TrimSpace(rawCodec))
	if codec == "" || lane != eventabi.LaneData {
		return nil
	}
	switch codec {
	case "pcm16", "opus", "pcmu", "mulaw":
		return nil
	default:
		return fmt.Errorf("unsupported ingress codec %q", rawCodec)
	}
}
