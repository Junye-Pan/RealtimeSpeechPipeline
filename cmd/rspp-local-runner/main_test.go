package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	livekittransport "github.com/tiger/realtime-speech-pipeline/transports/livekit"
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

func TestRunLoopbackSyntheticWritesValidatedReport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "loopback-report.json")

	var stdout bytes.Buffer
	err := run(
		[]string{"loopback", "-synthetic-text", "hello offline world", "-report", reportPath},
		&stdout,
		&bytes.Buffer{},
		fixedNow,
	)
	if err != nil {
		t.Fatalf("unexpected loopback synthetic error: %v", err)
	}
	report := readLoopbackReport(t, reportPath)
	if report.ProcessedEvents == 0 || len(report.IngressRecords) == 0 || len(report.ControlSignals) == 0 {
		t.Fatalf("unexpected loopback synthetic report contents: %+v", report)
	}
	if report.IngressRecords[0].PayloadClass != eventabi.PayloadTextRaw {
		t.Fatalf("expected text_raw ingress payload class in synthetic loopback, got %+v", report.IngressRecords[0])
	}
	if !strings.Contains(stdout.String(), "loopback report validated against Event ABI") {
		t.Fatalf("expected loopback validation output, got %q", stdout.String())
	}
}

func TestRunLoopbackWAVWritesValidatedReport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "loopback-wav-report.json")
	wavPath := filepath.Join(tmp, "sample.wav")
	if err := os.WriteFile(wavPath, []byte("RIFF....WAVEfmt "), 0o644); err != nil {
		t.Fatalf("write wav fixture: %v", err)
	}

	var stdout bytes.Buffer
	err := run(
		[]string{"loopback", "-wav", wavPath, "-report", reportPath},
		&stdout,
		&bytes.Buffer{},
		fixedNow,
	)
	if err != nil {
		t.Fatalf("unexpected loopback wav error: %v", err)
	}
	report := readLoopbackReport(t, reportPath)
	foundAudioRaw := false
	for _, record := range report.IngressRecords {
		if record.PayloadClass == eventabi.PayloadAudioRaw {
			foundAudioRaw = true
			break
		}
	}
	if !foundAudioRaw {
		t.Fatalf("expected at least one audio_raw ingress record in wav loopback report: %+v", report.IngressRecords)
	}
}

func TestRunLoopbackRejectsConflictingInputs(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	wavPath := filepath.Join(tmp, "sample.wav")
	if err := os.WriteFile(wavPath, []byte("RIFF....WAVEfmt "), 0o644); err != nil {
		t.Fatalf("write wav fixture: %v", err)
	}

	err := run(
		[]string{"loopback", "-wav", wavPath, "-synthetic-text", "text"},
		&bytes.Buffer{},
		&bytes.Buffer{},
		fixedNow,
	)
	if err == nil {
		t.Fatalf("expected conflicting loopback input modes to fail")
	}
}

func readLoopbackReport(t *testing.T, path string) livekittransport.Report {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read loopback report %s: %v", path, err)
	}
	var report livekittransport.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("decode loopback report %s: %v", path, err)
	}
	return report
}

func fixedNow() time.Time {
	return time.Date(2026, time.February, 11, 12, 0, 0, 0, time.UTC)
}
