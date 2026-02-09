package timeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BaselineArtifact is the runtime-produced OR-02 Stage-A baseline artifact.
type BaselineArtifact struct {
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at_utc"`
	Entries       []BaselineEvidence `json:"entries"`
}

const baselineArtifactSchemaVersion = "v1"

// WriteBaselineArtifact writes OR-02 baseline entries to a machine-readable file.
func WriteBaselineArtifact(path string, entries []BaselineEvidence) error {
	if path == "" {
		return fmt.Errorf("artifact path is required")
	}
	artifact := BaselineArtifact{
		SchemaVersion: baselineArtifactSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Entries:       entries,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

// ReadBaselineArtifact loads OR-02 baseline entries from artifact file.
func ReadBaselineArtifact(path string) (BaselineArtifact, error) {
	if path == "" {
		return BaselineArtifact{}, fmt.Errorf("artifact path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return BaselineArtifact{}, err
	}
	var artifact BaselineArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return BaselineArtifact{}, err
	}
	if artifact.SchemaVersion != baselineArtifactSchemaVersion {
		return BaselineArtifact{}, fmt.Errorf("unsupported baseline artifact schema_version: %s", artifact.SchemaVersion)
	}
	if len(artifact.Entries) == 0 {
		return BaselineArtifact{}, fmt.Errorf("baseline artifact contains no entries")
	}
	return artifact, nil
}
