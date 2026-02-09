package contract_test

import (
	"path/filepath"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/internal/tooling/validation"
)

func TestContractFixturesMatchSchema(t *testing.T) {
	t.Parallel()

	fixtureRoot := filepath.Join("fixtures")
	schemaPath := filepath.Join("..", "..", "docs", "ContractArtifacts.schema.json")

	summary, err := validation.ValidateContractFixturesWithSchema(schemaPath, fixtureRoot)
	if err != nil {
		t.Fatalf("schema validation execution failed: %v", err)
	}
	if summary.Total == 0 {
		t.Fatalf("expected non-zero fixture count")
	}
	if summary.Failed != 0 {
		t.Fatalf("expected zero schema mismatches, got %d\n%s", summary.Failed, validation.RenderSummary(summary))
	}
}
