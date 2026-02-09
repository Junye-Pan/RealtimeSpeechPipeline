package validation

import (
	"path/filepath"
	"testing"
)

func TestValidateContractFixtures(t *testing.T) {
	t.Parallel()

	fixtureRoot := filepath.Join("..", "..", "..", "test", "contract", "fixtures")
	schemaPath := filepath.Join("..", "..", "..", "docs", "ContractArtifacts.schema.json")
	summary, err := ValidateContractFixturesWithSchema(schemaPath, fixtureRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Total == 0 {
		t.Fatalf("expected non-zero fixture count")
	}
	if summary.Failed != 0 {
		t.Fatalf("expected zero failures, got %d\n%s", summary.Failed, RenderSummary(summary))
	}
}
