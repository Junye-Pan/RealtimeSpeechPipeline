package transport

import "testing"

func TestBuildConnectionSignalF6DisconnectAndStall(t *testing.T) {
	t.Parallel()

	disconnected, err := BuildConnectionSignal(ConnectionSignalInput{
		SessionID:            "sess-f6-1",
		TurnID:               "turn-f6-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f6-1",
		Signal:               "disconnected",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
		Reason:               "transport_disconnect_or_stall",
	})
	if err != nil {
		t.Fatalf("unexpected disconnected signal error: %v", err)
	}
	if disconnected.Signal != "disconnected" || disconnected.EmittedBy != "RK-23" {
		t.Fatalf("unexpected disconnected signal: %+v", disconnected)
	}

	stall, err := BuildConnectionSignal(ConnectionSignalInput{
		SessionID:            "sess-f6-1",
		TurnID:               "turn-f6-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-f6-2",
		Signal:               "stall",
		TransportSequence:    2,
		RuntimeSequence:      3,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   110,
		WallClockTimestampMS: 110,
		Reason:               "transport_disconnect_or_stall",
	})
	if err != nil {
		t.Fatalf("unexpected stall signal error: %v", err)
	}
	if stall.Signal != "stall" || stall.EmittedBy != "RK-23" {
		t.Fatalf("unexpected stall signal: %+v", stall)
	}
}
