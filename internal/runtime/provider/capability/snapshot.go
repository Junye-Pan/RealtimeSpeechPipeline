package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

const defaultSnapshotRef = "provider-capability/default"

// ProviderState captures turn-freeze capability inputs for one provider.
type ProviderState struct {
	Healthy           bool
	AvailabilityScore int
	PriceMicros       int64
}

// Snapshot freezes provider capability signals for deterministic turn execution.
type Snapshot struct {
	SnapshotRef  string
	CapturedAtMS int64
	Providers    map[string]ProviderState
}

// FreezeInput captures source capability state before turn open.
type FreezeInput struct {
	SnapshotRef  string
	CapturedAtMS int64
	Providers    map[string]ProviderState
}

// Freeze creates a validated immutable capability snapshot.
func Freeze(in FreezeInput) (Snapshot, error) {
	snapshot := Snapshot{
		SnapshotRef:  in.SnapshotRef,
		CapturedAtMS: in.CapturedAtMS,
		Providers:    cloneProviders(in.Providers),
	}
	if snapshot.SnapshotRef == "" {
		snapshot.SnapshotRef = defaultSnapshotRef
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// Validate enforces snapshot shape invariants.
func (s Snapshot) Validate() error {
	if s.SnapshotRef == "" {
		return fmt.Errorf("snapshot_ref is required")
	}
	if s.CapturedAtMS < 0 {
		return fmt.Errorf("captured_at_ms must be >=0")
	}
	for providerID, state := range s.Providers {
		if providerID == "" {
			return fmt.Errorf("provider id cannot be empty")
		}
		if state.AvailabilityScore < 0 || state.AvailabilityScore > 100 {
			return fmt.Errorf("availability_score must be in [0,100] for provider %s", providerID)
		}
		if state.PriceMicros < 0 {
			return fmt.Errorf("price_micros must be >=0 for provider %s", providerID)
		}
	}
	return nil
}

// Fingerprint returns deterministic digest for replay/debug correlation.
func (s Snapshot) Fingerprint() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	providers := make([]providerFingerprintEntry, 0, len(s.Providers))
	for providerID, state := range s.Providers {
		providers = append(providers, providerFingerprintEntry{ProviderID: providerID, State: state})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ProviderID < providers[j].ProviderID
	})
	payload := struct {
		SnapshotRef  string
		CapturedAtMS int64
		Providers    []providerFingerprintEntry
	}{
		SnapshotRef:  s.SnapshotRef,
		CapturedAtMS: s.CapturedAtMS,
		Providers:    providers,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Healthy returns whether provider is known and healthy in this snapshot.
func (s Snapshot) Healthy(providerID string) (bool, bool) {
	state, ok := s.Providers[providerID]
	if !ok {
		return false, false
	}
	return state.Healthy, true
}

// Price returns provider price in micros when present.
func (s Snapshot) Price(providerID string) (int64, bool) {
	state, ok := s.Providers[providerID]
	if !ok {
		return 0, false
	}
	return state.PriceMicros, true
}

func cloneProviders(in map[string]ProviderState) map[string]ProviderState {
	if len(in) == 0 {
		return map[string]ProviderState{}
	}
	out := make(map[string]ProviderState, len(in))
	for providerID, state := range in {
		out[providerID] = state
	}
	return out
}

type providerFingerprintEntry struct {
	ProviderID string
	State      ProviderState
}
