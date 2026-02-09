package guard

import "testing"

func TestEnrichIngressAuthorityFallbackAE004(t *testing.T) {
	t.Parallel()

	eval := Evaluator{}
	result, err := eval.EnrichIngressAuthority(IngressAuthorityInput{
		SessionID:        "sess-ae4-1",
		TurnID:           "turn-ae4-1",
		RoutingViewEpoch: 12,
	})
	if err != nil {
		t.Fatalf("unexpected enrichment error: %v", err)
	}
	if !result.Enriched || result.Source != "routing_view" || result.AuthorityEpoch != 12 {
		t.Fatalf("unexpected enrichment result: %+v", result)
	}

	repeat, err := eval.EnrichIngressAuthority(IngressAuthorityInput{
		SessionID:        "sess-ae4-1",
		TurnID:           "turn-ae4-1",
		RoutingViewEpoch: 12,
	})
	if err != nil {
		t.Fatalf("unexpected repeat enrichment error: %v", err)
	}
	if repeat != result {
		t.Fatalf("expected deterministic enrichment result, got first=%+v second=%+v", result, repeat)
	}
}

func TestEnrichIngressAuthorityUsesCarrierWhenPresent(t *testing.T) {
	t.Parallel()

	epoch := int64(22)
	eval := Evaluator{}
	result, err := eval.EnrichIngressAuthority(IngressAuthorityInput{
		SessionID:             "sess-ae4-2",
		CarrierAuthorityEpoch: &epoch,
		RoutingViewEpoch:      12,
	})
	if err != nil {
		t.Fatalf("unexpected enrichment error: %v", err)
	}
	if result.Enriched || result.Source != "carrier" || result.AuthorityEpoch != 22 {
		t.Fatalf("unexpected carrier result: %+v", result)
	}
}
