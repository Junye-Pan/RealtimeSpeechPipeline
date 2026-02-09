package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildReplaySmokeDivergencesIsZero(t *testing.T) {
	t.Parallel()

	divergences := buildReplaySmokeDivergences()
	if len(divergences) != 0 {
		t.Fatalf("expected zero replay smoke divergences, got %+v", divergences)
	}
}

func TestWriteReplaySmokeReport(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "replay", "smoke-report.json")
	if err := writeReplaySmokeReport(out); err != nil {
		t.Fatalf("unexpected report write error: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	var report replaySmokeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unexpected json error: %v", err)
	}
	if report.TotalDivergences != 0 {
		t.Fatalf("expected zero divergences, got %d", report.TotalDivergences)
	}
}
