package registry

import (
	"fmt"
	"sort"

	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

// Catalog stores deterministic provider adapters by modality.
type Catalog struct {
	adapters map[contracts.Modality]map[string]contracts.Adapter
	ordered  map[contracts.Modality][]string
}

// NewCatalog creates a deterministic adapter catalog.
func NewCatalog(adapters []contracts.Adapter) (Catalog, error) {
	catalog := Catalog{
		adapters: make(map[contracts.Modality]map[string]contracts.Adapter),
		ordered:  make(map[contracts.Modality][]string),
	}

	for _, modality := range []contracts.Modality{contracts.ModalitySTT, contracts.ModalityLLM, contracts.ModalityTTS} {
		catalog.adapters[modality] = make(map[string]contracts.Adapter)
	}

	for _, adapter := range adapters {
		if adapter == nil {
			return Catalog{}, fmt.Errorf("adapter cannot be nil")
		}

		modality := adapter.Modality()
		if err := modality.Validate(); err != nil {
			return Catalog{}, err
		}
		providerID := adapter.ProviderID()
		if providerID == "" {
			return Catalog{}, fmt.Errorf("provider_id is required")
		}
		if _, exists := catalog.adapters[modality][providerID]; exists {
			return Catalog{}, fmt.Errorf("duplicate provider_id %q for modality %q", providerID, modality)
		}
		catalog.adapters[modality][providerID] = adapter
	}

	for modality, providers := range catalog.adapters {
		ids := make([]string, 0, len(providers))
		for providerID := range providers {
			ids = append(ids, providerID)
		}
		sort.Strings(ids)
		catalog.ordered[modality] = ids
	}

	return catalog, nil
}

// Adapter returns a single adapter by modality/provider pair.
func (c Catalog) Adapter(modality contracts.Modality, providerID string) (contracts.Adapter, bool) {
	byProvider, ok := c.adapters[modality]
	if !ok {
		return nil, false
	}
	adapter, exists := byProvider[providerID]
	return adapter, exists
}

// ProviderIDs returns deterministic provider ids for a modality.
func (c Catalog) ProviderIDs(modality contracts.Modality) ([]string, error) {
	if err := modality.Validate(); err != nil {
		return nil, err
	}
	ids, ok := c.ordered[modality]
	if !ok || len(ids) == 0 {
		return nil, fmt.Errorf("no providers registered for modality %q", modality)
	}
	out := make([]string, len(ids))
	copy(out, ids)
	return out, nil
}

// Candidates returns deterministic adapters with preferred provider first when present.
func (c Catalog) Candidates(modality contracts.Modality, preferredProvider string, maxProviders int) ([]contracts.Adapter, error) {
	if err := modality.Validate(); err != nil {
		return nil, err
	}

	ids, ok := c.ordered[modality]
	if !ok || len(ids) == 0 {
		return nil, fmt.Errorf("no providers registered for modality %q", modality)
	}
	if maxProviders < 1 {
		maxProviders = 5
	}

	selected := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	if preferredProvider != "" {
		if _, exists := c.adapters[modality][preferredProvider]; !exists {
			return nil, fmt.Errorf("preferred provider %q is not registered for modality %q", preferredProvider, modality)
		}
		selected = append(selected, preferredProvider)
		seen[preferredProvider] = struct{}{}
	}

	for _, providerID := range ids {
		if _, exists := seen[providerID]; exists {
			continue
		}
		selected = append(selected, providerID)
		if len(selected) >= maxProviders {
			break
		}
	}

	adapters := make([]contracts.Adapter, 0, len(selected))
	for _, providerID := range selected {
		adapters = append(adapters, c.adapters[modality][providerID])
	}
	return adapters, nil
}

// ValidateCoverage enforces provider count bounds for each MVP modality.
func (c Catalog) ValidateCoverage(minPerModality, maxPerModality int) error {
	if minPerModality < 1 {
		return fmt.Errorf("min_per_modality must be >=1")
	}
	if maxPerModality < minPerModality {
		return fmt.Errorf("max_per_modality must be >= min_per_modality")
	}

	for _, modality := range []contracts.Modality{contracts.ModalitySTT, contracts.ModalityLLM, contracts.ModalityTTS} {
		count := len(c.adapters[modality])
		if count < minPerModality || count > maxPerModality {
			return fmt.Errorf("modality %q requires %d-%d providers, got %d", modality, minPerModality, maxPerModality, count)
		}
	}
	return nil
}
