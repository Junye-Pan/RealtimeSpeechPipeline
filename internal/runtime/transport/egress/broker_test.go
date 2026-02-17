package egress

import (
	"testing"

	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
)

func TestBrokerDeliverCancelFence(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	base := baseAttempt()

	first, err := broker.Deliver(Delivery{Attempt: base, PayloadBytes: 10})
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if !first.Accepted || first.Signal.Signal != "output_accepted" {
		t.Fatalf("expected first output accepted, got %+v", first)
	}

	cancelAttempt := base
	cancelAttempt.EventID = "evt-egress-cancel"
	cancelAttempt.CancelAccepted = true
	cancelDecision, err := broker.Deliver(Delivery{Attempt: cancelAttempt, PayloadBytes: 10})
	if err != nil {
		t.Fatalf("cancel delivery: %v", err)
	}
	if cancelDecision.Accepted || cancelDecision.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected cancel-fenced output, got %+v", cancelDecision)
	}
}

func TestBrokerDeliverRejectsInvalidPayloadBytes(t *testing.T) {
	t.Parallel()

	broker := NewBroker(nil)
	if _, err := broker.Deliver(Delivery{Attempt: baseAttempt(), PayloadBytes: -1}); err == nil {
		t.Fatalf("expected negative payload bytes to fail")
	}
}

func baseAttempt() runtimetransport.OutputAttempt {
	return runtimetransport.OutputAttempt{
		SessionID:            "sess-egress-1",
		TurnID:               "turn-egress-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-egress-1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       2,
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
	}
}
