package contract_test

import (
	"fmt"
	"testing"
)

type conformanceProfile struct {
	ID                  string
	MandatoryCategories []string
}

func TestProviderConformanceProfilePassesWithAllMandatoryCategories(t *testing.T) {
	t.Parallel()

	profile := conformanceProfile{
		ID: "provider-adapter-mvp-v1",
		MandatoryCategories: []string{
			"contract_shape",
			"cancel_path",
			"streaming_lifecycle",
			"outcome_normalization",
		},
	}

	results := map[string]bool{
		"contract_shape":        true,
		"cancel_path":           true,
		"streaming_lifecycle":   true,
		"outcome_normalization": true,
		"optional_load_smoke":   true,
	}

	if err := validateConformanceProfile(profile, results); err != nil {
		t.Fatalf("expected profile validation to pass, got %v", err)
	}
}

func TestProviderConformanceProfileRejectsMissingMandatoryCategory(t *testing.T) {
	t.Parallel()

	profile := conformanceProfile{
		ID: "provider-adapter-mvp-v1",
		MandatoryCategories: []string{
			"contract_shape",
			"cancel_path",
			"streaming_lifecycle",
			"outcome_normalization",
		},
	}

	results := map[string]bool{
		"contract_shape":      true,
		"cancel_path":         true,
		"streaming_lifecycle": true,
		// outcome_normalization omitted to prove mandatory-category rejection.
	}

	if err := validateConformanceProfile(profile, results); err == nil {
		t.Fatalf("expected missing mandatory category to fail profile validation")
	}
}

func validateConformanceProfile(profile conformanceProfile, results map[string]bool) error {
	if profile.ID == "" {
		return fmt.Errorf("profile id is required")
	}
	if len(profile.MandatoryCategories) == 0 {
		return fmt.Errorf("profile %s must define mandatory categories", profile.ID)
	}
	for _, category := range profile.MandatoryCategories {
		passed, ok := results[category]
		if !ok {
			return fmt.Errorf("profile %s missing mandatory category result: %s", profile.ID, category)
		}
		if !passed {
			return fmt.Errorf("profile %s failed mandatory category: %s", profile.ID, category)
		}
	}
	return nil
}
