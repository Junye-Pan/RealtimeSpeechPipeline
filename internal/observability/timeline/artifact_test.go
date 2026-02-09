package timeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadBaselineArtifactRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline.json")
	entries := []BaselineEvidence{minimalBaseline("turn-artifact-1")}
	if err := WriteBaselineArtifact(path, entries); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	artifact, err := ReadBaselineArtifact(path)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if artifact.SchemaVersion != baselineArtifactSchemaVersion {
		t.Fatalf("unexpected schema version: %s", artifact.SchemaVersion)
	}
	if len(artifact.Entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(artifact.Entries))
	}
	if artifact.Entries[0].TurnID != "turn-artifact-1" {
		t.Fatalf("unexpected artifact entry: %+v", artifact.Entries[0])
	}
}

func TestReadBaselineArtifactRejectsEmptyEntries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "empty.json")
	payload, err := json.Marshal(BaselineArtifact{SchemaVersion: baselineArtifactSchemaVersion, Entries: nil})
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	if _, err := ReadBaselineArtifact(path); err == nil {
		t.Fatalf("expected empty-entry artifact read to fail")
	}
}
