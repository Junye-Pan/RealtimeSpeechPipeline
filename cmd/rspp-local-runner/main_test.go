package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunDefaultsToLiveKitDryRun(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "local-runner-report.json")

	var stdout bytes.Buffer
	err := run([]string{"-dry-run=true", "-probe=false", "-report", reportPath}, &stdout, &bytes.Buffer{}, fixedNow)
	if err != nil {
		t.Fatalf("unexpected local-runner error: %v", err)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected local-runner report at %s: %v", reportPath, err)
	}
	if !strings.Contains(stdout.String(), "single-node runtime + minimal control-plane stub") {
		t.Fatalf("expected startup summary output, got %q", stdout.String())
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &bytes.Buffer{}, fixedNow); err != nil {
		t.Fatalf("unexpected help error: %v", err)
	}
	if !strings.Contains(stdout.String(), "rspp-local-runner usage") {
		t.Fatalf("expected usage output, got %q", stdout.String())
	}
}

func fixedNow() time.Time {
	return time.Date(2026, time.February, 11, 12, 0, 0, 0, time.UTC)
}
