package nodehost

import "testing"

func TestHandleFailureF2TerminalWhenNoFallback(t *testing.T) {
	t.Parallel()

	result, err := HandleFailure(NodeFailureInput{
		SessionID:            "sess-f2-1",
		TurnID:               "turn-f2-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f2-1",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Terminal || result.TerminalReason != "node_timeout_or_failure" {
		t.Fatalf("expected terminal node timeout/failure, got %+v", result)
	}
	if len(result.Signals) < 2 {
		t.Fatalf("expected budget warning/exhausted signals, got %+v", result.Signals)
	}
}

func TestHandleFailureF2DegradeAvoidsTerminal(t *testing.T) {
	t.Parallel()

	result, err := HandleFailure(NodeFailureInput{
		SessionID:            "sess-f2-2",
		TurnID:               "turn-f2-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f2-2",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		AllowDegrade:         true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Terminal {
		t.Fatalf("expected non-terminal when degrade is available")
	}
	found := false
	for _, sig := range result.Signals {
		if sig.Signal == "degrade" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected degrade signal in non-terminal path")
	}
}
