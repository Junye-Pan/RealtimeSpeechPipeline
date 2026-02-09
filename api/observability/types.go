package observability

// DivergenceClass is a minimal shared type for replay/reporting artifacts.
type DivergenceClass string

const (
	PlanDivergence      DivergenceClass = "PLAN_DIVERGENCE"
	OutcomeDivergence   DivergenceClass = "OUTCOME_DIVERGENCE"
	OrderingDivergence  DivergenceClass = "ORDERING_DIVERGENCE"
	TimingDivergence    DivergenceClass = "TIMING_DIVERGENCE"
	AuthorityDivergence DivergenceClass = "AUTHORITY_DIVERGENCE"
)

// ReplayDivergence captures a classified replay mismatch entry.
type ReplayDivergence struct {
	Class    DivergenceClass `json:"class"`
	Scope    string          `json:"scope"`
	Message  string          `json:"message"`
	DiffMS   *int64          `json:"diff_ms,omitempty"`
	Expected bool            `json:"expected,omitempty"`
}
