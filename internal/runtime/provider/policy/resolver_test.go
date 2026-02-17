package policy

import (
	"reflect"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/capability"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

func TestResolveUsesTenantRuleAndCapabilityOrdering(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(RuleSet{
		ByTenant: map[string]map[contracts.Modality][]string{
			"tenant-a": {
				contracts.ModalitySTT: {"stt-b", "stt-a", "stt-c"},
			},
		},
		AllowedActions:    []string{"retry", "fallback"},
		Budget:            Budget{MaxTotalAttempts: 4, MaxTotalLatencyMS: 2500},
		PolicySnapshotRef: "policy-resolution/v2",
	})

	capSnapshot, err := capability.Freeze(capability.FreezeInput{
		SnapshotRef:  "provider-health/v2",
		CapturedAtMS: 100,
		Providers: map[string]capability.ProviderState{
			"stt-a": {Healthy: true, AvailabilityScore: 95, PriceMicros: 30},
			"stt-b": {Healthy: false, AvailabilityScore: 10, PriceMicros: 10},
			"stt-c": {Healthy: true, AvailabilityScore: 90, PriceMicros: 20},
		},
	})
	if err != nil {
		t.Fatalf("freeze capability snapshot: %v", err)
	}

	plan, err := resolver.Resolve(Input{
		Modality:           contracts.ModalitySTT,
		CatalogProviderIDs: []string{"stt-a", "stt-b", "stt-c"},
		TenantID:           "tenant-a",
		CapabilitySnapshot: capSnapshot,
	})
	if err != nil {
		t.Fatalf("resolve provider plan: %v", err)
	}

	expectedCandidates := []string{"stt-c", "stt-a", "stt-b"}
	if !reflect.DeepEqual(plan.OrderedCandidates, expectedCandidates) {
		t.Fatalf("expected ordered candidates %v, got %v", expectedCandidates, plan.OrderedCandidates)
	}
	if plan.RoutingReason != "rule:tenant" {
		t.Fatalf("expected tenant routing reason, got %q", plan.RoutingReason)
	}
	if plan.SignalSource != "capability_snapshot" {
		t.Fatalf("expected capability signal source, got %q", plan.SignalSource)
	}
	if plan.PolicySnapshotRef != "policy-resolution/v2" || plan.CapabilitySnapshotRef != "provider-health/v2" {
		t.Fatalf("expected snapshot refs to be preserved, got policy=%q capability=%q", plan.PolicySnapshotRef, plan.CapabilitySnapshotRef)
	}
}

func TestResolvePreferredProviderPinnedFirst(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(RuleSet{
		Default: map[contracts.Modality][]string{
			contracts.ModalityLLM: {"llm-a", "llm-b", "llm-c"},
		},
	})

	capSnapshot, err := capability.Freeze(capability.FreezeInput{
		SnapshotRef: "provider-health/v1",
		Providers: map[string]capability.ProviderState{
			"llm-a": {Healthy: true, PriceMicros: 10},
			"llm-b": {Healthy: false, PriceMicros: 1},
			"llm-c": {Healthy: true, PriceMicros: 20},
		},
	})
	if err != nil {
		t.Fatalf("freeze capability snapshot: %v", err)
	}

	plan, err := resolver.Resolve(Input{
		Modality:           contracts.ModalityLLM,
		CatalogProviderIDs: []string{"llm-a", "llm-b", "llm-c"},
		PreferredProvider:  "llm-b",
		CapabilitySnapshot: capSnapshot,
	})
	if err != nil {
		t.Fatalf("resolve provider plan: %v", err)
	}
	if len(plan.OrderedCandidates) == 0 || plan.OrderedCandidates[0] != "llm-b" {
		t.Fatalf("expected preferred provider llm-b pinned first, got %+v", plan.OrderedCandidates)
	}
}

func TestResolveFallbackToCatalogDefaults(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(RuleSet{})
	plan, err := resolver.Resolve(Input{
		Modality:           contracts.ModalityTTS,
		CatalogProviderIDs: []string{"tts-b", "tts-a"},
	})
	if err != nil {
		t.Fatalf("resolve provider plan: %v", err)
	}

	expected := []string{"tts-b", "tts-a"}
	if !reflect.DeepEqual(plan.OrderedCandidates, expected) {
		t.Fatalf("expected catalog fallback ordering %v, got %v", expected, plan.OrderedCandidates)
	}
	if plan.RoutingReason != "fallback:catalog_default" {
		t.Fatalf("expected fallback routing reason, got %q", plan.RoutingReason)
	}
	if !reflect.DeepEqual(plan.AllowedActions, []string{"fallback", "provider_switch", "retry"}) {
		t.Fatalf("expected default normalized actions, got %v", plan.AllowedActions)
	}
}

func TestResolveRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(RuleSet{AllowedActions: []string{"unsupported"}})
	_, err := resolver.Resolve(Input{
		Modality:           contracts.ModalitySTT,
		CatalogProviderIDs: []string{"stt-a"},
	})
	if err == nil {
		t.Fatalf("expected invalid allowed actions to fail")
	}

	_, err = NewResolver(RuleSet{}).Resolve(Input{Modality: contracts.ModalitySTT})
	if err == nil {
		t.Fatalf("expected empty catalog provider IDs to fail")
	}
}
