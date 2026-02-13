package livekit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunCLIWritesReportFromDefaultScenario(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "livekit-report.json")

	var stdout bytes.Buffer
	if err := RunCLI([]string{
		"-dry-run=true",
		"-probe=false",
		"-report", reportPath,
		"-session-id", "sess-cmd-1",
		"-turn-id", "turn-cmd-1",
		"-pipeline-version", "pipeline-v1",
		"-authority-epoch", "4",
	}, &stdout, fixedCommandNow); err != nil {
		t.Fatalf("run livekit cli: %v", err)
	}

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected non-empty report")
	}
	if stdout.Len() == 0 {
		t.Fatalf("expected summary output")
	}
}

func TestLoadEventsAcceptsArrayAndObject(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	arrayPath := filepath.Join(tmp, "array.json")
	objectPath := filepath.Join(tmp, "object.json")

	if err := os.WriteFile(arrayPath, []byte(`[{"kind":"connected"},{"kind":"turn_open_proposed"}]`), 0o644); err != nil {
		t.Fatalf("write array fixture: %v", err)
	}
	if err := os.WriteFile(objectPath, []byte(`{"events":[{"kind":"connected"},{"kind":"turn_open_proposed"}]}`), 0o644); err != nil {
		t.Fatalf("write object fixture: %v", err)
	}

	if _, err := LoadEvents(arrayPath); err != nil {
		t.Fatalf("load array events: %v", err)
	}
	if _, err := LoadEvents(objectPath); err != nil {
		t.Fatalf("load object events: %v", err)
	}
}

func fixedCommandNow() time.Time {
	return time.Date(2026, time.February, 11, 11, 0, 0, 0, time.UTC)
}
