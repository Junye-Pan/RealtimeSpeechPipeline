package prelude

import "testing"

func TestProposeTurnGeneratesSignal(t *testing.T) {
	t.Parallel()

	engine := NewEngine()
	proposal, err := engine.ProposeTurn(Input{
		SessionID:            "sess-prelude-1",
		TurnID:               "turn-prelude-1",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected proposal error: %v", err)
	}
	if proposal.Signal.Signal != "turn_open_proposed" || proposal.Signal.EmittedBy != "RK-02" {
		t.Fatalf("expected turn_open_proposed RK-02 signal, got %+v", proposal.Signal)
	}
}

func TestProposeTurnRequiresSessionTurn(t *testing.T) {
	t.Parallel()

	engine := NewEngine()
	if _, err := engine.ProposeTurn(Input{}); err == nil {
		t.Fatalf("expected missing session/turn to fail")
	}
}
