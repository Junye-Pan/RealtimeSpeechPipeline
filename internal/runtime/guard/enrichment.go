package guard

import "fmt"

// IngressAuthorityInput captures authority metadata available at ingress.
type IngressAuthorityInput struct {
	SessionID             string
	TurnID                string
	CarrierAuthorityEpoch *int64
	RoutingViewEpoch      int64
}

// IngressAuthorityResult is the normalized authority context after enrichment.
type IngressAuthorityResult struct {
	AuthorityEpoch int64
	Enriched       bool
	Source         string
}

// EnrichIngressAuthority deterministically fills missing carrier authority metadata.
func (Evaluator) EnrichIngressAuthority(in IngressAuthorityInput) (IngressAuthorityResult, error) {
	if in.SessionID == "" {
		return IngressAuthorityResult{}, fmt.Errorf("session_id is required")
	}
	if in.RoutingViewEpoch < 0 {
		return IngressAuthorityResult{}, fmt.Errorf("routing view epoch must be >= 0")
	}

	if in.CarrierAuthorityEpoch != nil {
		if *in.CarrierAuthorityEpoch < 0 {
			return IngressAuthorityResult{}, fmt.Errorf("carrier authority epoch must be >= 0")
		}
		return IngressAuthorityResult{AuthorityEpoch: *in.CarrierAuthorityEpoch, Enriched: false, Source: "carrier"}, nil
	}

	return IngressAuthorityResult{AuthorityEpoch: in.RoutingViewEpoch, Enriched: true, Source: "routing_view"}, nil
}
