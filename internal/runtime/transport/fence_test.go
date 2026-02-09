package transport

import "testing"

func TestCF002LateOutputAfterCancelIsFenced(t *testing.T) {
	t.Parallel()

	fence := NewOutputFence()

	beforeCancel, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-cf-2",
		TurnID:               "turn-cf-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-before-cancel",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected pre-cancel output decision error: %v", err)
	}
	if !beforeCancel.Accepted || beforeCancel.Signal.Signal != "output_accepted" {
		t.Fatalf("expected output accepted before cancel, got %+v", beforeCancel)
	}

	cancelFence, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-cf-2",
		TurnID:               "turn-cf-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-cancel",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
		CancelAccepted:       true,
	})
	if err != nil {
		t.Fatalf("unexpected cancel fence error: %v", err)
	}
	if cancelFence.Accepted || cancelFence.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected cancel fence to reject output, got %+v", cancelFence)
	}

	lateProviderOutput, err := fence.EvaluateOutput(OutputAttempt{
		SessionID:            "sess-cf-2",
		TurnID:               "turn-cf-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-late-provider-output",
		TransportSequence:    3,
		RuntimeSequence:      3,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   102,
		WallClockTimestampMS: 102,
	})
	if err != nil {
		t.Fatalf("unexpected late-output fence error: %v", err)
	}
	if lateProviderOutput.Accepted || lateProviderOutput.Signal.Signal != "playback_cancelled" {
		t.Fatalf("expected deterministic late-output fence, got %+v", lateProviderOutput)
	}
}
