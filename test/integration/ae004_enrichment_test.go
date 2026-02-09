package integration_test

import (
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/guard"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

func TestAE004IngressAuthorityEnrichmentFallback(t *testing.T) {
	t.Parallel()

	guardEval := guard.Evaluator{}
	enriched, err := guardEval.EnrichIngressAuthority(guard.IngressAuthorityInput{
		SessionID:        "sess-ae-004",
		TurnID:           "turn-ae-004",
		RoutingViewEpoch: 14,
	})
	if err != nil {
		t.Fatalf("unexpected enrichment error: %v", err)
	}
	if !enriched.Enriched || enriched.AuthorityEpoch != 14 {
		t.Fatalf("expected deterministic routing-view enrichment, got %+v", enriched)
	}

	arbiter := turnarbiter.New()
	open, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-ae-004",
		TurnID:               "turn-ae-004",
		EventID:              "evt-ae-004",
		RuntimeTimestampMS:   120,
		WallClockTimestampMS: 120,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       enriched.AuthorityEpoch,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		t.Fatalf("unexpected turn open error after enrichment: %v", err)
	}
	if open.State == controlplane.TurnIdle {
		t.Fatalf("expected enriched authority to allow turn opening, got state=%s", open.State)
	}
}
