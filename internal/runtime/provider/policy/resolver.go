package policy

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/capability"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

const defaultPolicySnapshotRef = "policy/default"

// Budget defines deterministic invocation-budget limits resolved at turn start.
type Budget struct {
	MaxTotalAttempts  int
	MaxTotalLatencyMS int64
}

// Validate enforces budget invariants.
func (b Budget) Validate() error {
	if b.MaxTotalAttempts < 0 {
		return fmt.Errorf("max_total_attempts must be >=0")
	}
	if b.MaxTotalLatencyMS < 0 {
		return fmt.Errorf("max_total_latency_ms must be >=0")
	}
	return nil
}

// RuleSet defines deterministic provider candidate ordering rules.
type RuleSet struct {
	Default                map[contracts.Modality][]string
	ByTenant               map[string]map[contracts.Modality][]string
	ByLanguage             map[string]map[contracts.Modality][]string
	ByRegion               map[string]map[contracts.Modality][]string
	ByCostTier             map[string]map[contracts.Modality][]string
	AllowedActions         []string
	MaxAttemptsPerProvider int
	Budget                 Budget
	PolicySnapshotRef      string
}

// Input captures provider-selection context at turn-start freeze.
type Input struct {
	Modality           contracts.Modality
	CatalogProviderIDs []string
	PreferredProvider  string
	TenantID           string
	Language           string
	Region             string
	CostTier           string
	CapabilitySnapshot capability.Snapshot
}

// ResolvedProviderPlan contains frozen provider-selection behavior for invocation.
type ResolvedProviderPlan struct {
	OrderedCandidates      []string
	MaxAttemptsPerProvider int
	AllowedActions         []string
	Budget                 Budget
	PolicySnapshotRef      string
	CapabilitySnapshotRef  string
	RoutingReason          string
	SignalSource           string
}

// Resolver produces deterministic provider plans from policy and capability snapshots.
type Resolver struct {
	rules RuleSet
}

// NewResolver returns a deterministic policy resolver.
func NewResolver(rules RuleSet) Resolver {
	if rules.MaxAttemptsPerProvider < 1 {
		rules.MaxAttemptsPerProvider = 2
	}
	if rules.PolicySnapshotRef == "" {
		rules.PolicySnapshotRef = defaultPolicySnapshotRef
	}
	return Resolver{rules: rules}
}

// Resolve produces the turn-frozen provider plan.
func (r Resolver) Resolve(in Input) (ResolvedProviderPlan, error) {
	if err := in.Modality.Validate(); err != nil {
		return ResolvedProviderPlan{}, err
	}
	catalogSet, catalogOrder, err := validateCatalogProviderIDs(in.CatalogProviderIDs)
	if err != nil {
		return ResolvedProviderPlan{}, err
	}
	if err := r.rules.Budget.Validate(); err != nil {
		return ResolvedProviderPlan{}, err
	}

	candidates, reason := r.resolveRuleCandidates(in)
	candidates = filterCandidates(candidates, catalogSet)
	if len(candidates) == 0 {
		candidates = append([]string(nil), catalogOrder...)
		reason = "fallback:catalog_default"
	}

	signalSource := "fallback_default"
	capabilityRef := ""
	if len(in.CapabilitySnapshot.Providers) > 0 {
		candidates = applyCapabilityOrdering(candidates, in.CapabilitySnapshot)
		signalSource = "capability_snapshot"
		capabilityRef = in.CapabilitySnapshot.SnapshotRef
		if capabilityRef == "" {
			capabilityRef = "provider-capability/default"
		}
	}
	candidates = pinPreferred(candidates, in.PreferredProvider, catalogSet)
	if len(candidates) == 0 {
		return ResolvedProviderPlan{}, fmt.Errorf("no candidates available after applying preferred provider")
	}

	actions, err := resolveAllowedActions(r.rules.AllowedActions)
	if err != nil {
		return ResolvedProviderPlan{}, err
	}

	return ResolvedProviderPlan{
		OrderedCandidates:      candidates,
		MaxAttemptsPerProvider: r.rules.MaxAttemptsPerProvider,
		AllowedActions:         actions,
		Budget:                 r.rules.Budget,
		PolicySnapshotRef:      r.rules.PolicySnapshotRef,
		CapabilitySnapshotRef:  capabilityRef,
		RoutingReason:          reason,
		SignalSource:           signalSource,
	}, nil
}

func (r Resolver) resolveRuleCandidates(in Input) ([]string, string) {
	modality := in.Modality
	if values := resolveScopedRule(r.rules.ByTenant, strings.TrimSpace(in.TenantID), modality); len(values) > 0 {
		return dedupe(values), "rule:tenant"
	}
	if values := resolveScopedRule(r.rules.ByLanguage, strings.TrimSpace(in.Language), modality); len(values) > 0 {
		return dedupe(values), "rule:language"
	}
	if values := resolveScopedRule(r.rules.ByRegion, strings.TrimSpace(in.Region), modality); len(values) > 0 {
		return dedupe(values), "rule:region"
	}
	if values := resolveScopedRule(r.rules.ByCostTier, strings.TrimSpace(in.CostTier), modality); len(values) > 0 {
		return dedupe(values), "rule:cost_tier"
	}
	if values := r.rules.Default[modality]; len(values) > 0 {
		return dedupe(values), "rule:default"
	}
	return nil, "fallback:catalog_default"
}

func resolveScopedRule(scope map[string]map[contracts.Modality][]string, key string, modality contracts.Modality) []string {
	if key == "" || len(scope) == 0 {
		return nil
	}
	byModality, ok := scope[key]
	if !ok {
		return nil
	}
	return byModality[modality]
}

func resolveAllowedActions(configured []string) ([]string, error) {
	source := configured
	if len(source) == 0 {
		source = []string{"retry", "provider_switch", "fallback"}
	}
	actions, err := contracts.NormalizeAdaptiveActions(source)
	if err != nil {
		return nil, err
	}
	return actions, nil
}

func validateCatalogProviderIDs(in []string) (map[string]struct{}, []string, error) {
	if len(in) == 0 {
		return nil, nil, fmt.Errorf("catalog_provider_ids must be non-empty")
	}
	set := make(map[string]struct{}, len(in))
	ordered := make([]string, 0, len(in))
	for _, providerID := range in {
		trimmed := strings.TrimSpace(providerID)
		if trimmed == "" {
			return nil, nil, fmt.Errorf("catalog_provider_ids cannot contain empty values")
		}
		if _, exists := set[trimmed]; exists {
			continue
		}
		set[trimmed] = struct{}{}
		ordered = append(ordered, trimmed)
	}
	if len(ordered) == 0 {
		return nil, nil, fmt.Errorf("catalog_provider_ids must resolve to at least one provider")
	}
	return set, ordered, nil
}

func filterCandidates(candidates []string, allowed map[string]struct{}) []string {
	filtered := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, providerID := range candidates {
		trimmed := strings.TrimSpace(providerID)
		if trimmed == "" {
			continue
		}
		if _, ok := allowed[trimmed]; !ok {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func pinPreferred(candidates []string, preferredProvider string, allowed map[string]struct{}) []string {
	preferred := strings.TrimSpace(preferredProvider)
	if preferred == "" {
		return candidates
	}
	if _, ok := allowed[preferred]; !ok {
		return candidates
	}
	out := make([]string, 0, len(candidates)+1)
	out = append(out, preferred)
	for _, providerID := range candidates {
		if providerID == preferred {
			continue
		}
		out = append(out, providerID)
	}
	return out
}

func applyCapabilityOrdering(candidates []string, snapshot capability.Snapshot) []string {
	ordered := append([]string(nil), candidates...)
	index := make(map[string]int, len(ordered))
	for i, providerID := range ordered {
		index[providerID] = i
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		left := rankProvider(snapshot, ordered[i], index[ordered[i]])
		right := rankProvider(snapshot, ordered[j], index[ordered[j]])
		if left.healthRank != right.healthRank {
			return left.healthRank < right.healthRank
		}
		if left.priceMicros != right.priceMicros {
			return left.priceMicros < right.priceMicros
		}
		return left.originalIndex < right.originalIndex
	})
	return ordered
}

type providerRank struct {
	healthRank    int
	priceMicros   int64
	originalIndex int
}

func rankProvider(snapshot capability.Snapshot, providerID string, originalIndex int) providerRank {
	state, ok := snapshot.Providers[providerID]
	if !ok {
		return providerRank{healthRank: 1, priceMicros: math.MaxInt64, originalIndex: originalIndex}
	}
	healthRank := 2
	if state.Healthy {
		healthRank = 0
	}
	price := state.PriceMicros
	if price < 0 {
		price = math.MaxInt64
	}
	return providerRank{healthRank: healthRank, priceMicros: price, originalIndex: originalIndex}
}

func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
