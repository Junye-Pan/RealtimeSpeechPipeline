package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	livekittransport "github.com/tiger/realtime-speech-pipeline/transports/livekit"
)

const (
	defaultLiveKitReportPath  = ".codex/transports/livekit-local-runner-report.json"
	defaultLoopbackReportPath = ".codex/transports/loopback-local-runner-report.json"
)

type loopbackInputMode string

const (
	loopbackInputSynthetic loopbackInputMode = "synthetic_text"
	loopbackInputWAV       loopbackInputMode = "wav"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now); err != nil {
		fmt.Fprintf(os.Stderr, "rspp-local-runner: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, _ io.Writer, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			printUsage(stdout)
			return nil
		case "livekit":
			args = args[1:]
		case "loopback":
			return runLoopback(args[1:], stdout, now)
		}
	}
	if len(args) == 0 {
		args = []string{
			"-dry-run=true",
			"-report=" + defaultLiveKitReportPath,
		}
	}

	_, _ = fmt.Fprintln(stdout, "rspp-local-runner: starting single-node runtime + minimal control-plane stub via livekit adapter")
	return livekittransport.RunCLI(args, stdout, now)
}

func runLoopback(args []string, stdout io.Writer, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}

	fs := flag.NewFlagSet("loopback", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	reportPath := fs.String("report", defaultLoopbackReportPath, "path to write loopback runner report json")
	wavPath := fs.String("wav", "", "optional wav input path for offline loopback audio simulation")
	syntheticText := fs.String("synthetic-text", "", "synthetic text source for offline loopback simulation")
	sessionID := fs.String("session-id", "sess-loopback-local-runner", "loopback session id")
	turnID := fs.String("turn-id", "turn-loopback-local-runner", "loopback turn id")
	pipelineVersion := fs.String("pipeline-version", "pipeline-v1", "loopback pipeline version")
	authorityEpoch := fs.Int64("authority-epoch", 1, "loopback authority epoch")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*reportPath) == "" {
		return fmt.Errorf("loopback report path is required")
	}
	if strings.TrimSpace(*wavPath) != "" && strings.TrimSpace(*syntheticText) != "" {
		return fmt.Errorf("loopback accepts either -wav or -synthetic-text, not both")
	}

	mode, defaultDataClass, events, err := buildLoopbackEvents(loopbackEventInput{
		wavPath:         strings.TrimSpace(*wavPath),
		syntheticText:   strings.TrimSpace(*syntheticText),
		sessionID:       strings.TrimSpace(*sessionID),
		turnID:          strings.TrimSpace(*turnID),
		pipelineVersion: strings.TrimSpace(*pipelineVersion),
	})
	if err != nil {
		return err
	}

	eventsPath, cleanup, err := writeLoopbackEventsFixture(events)
	if err != nil {
		return err
	}
	defer cleanup()

	livekitArgs := []string{
		"-dry-run=true",
		"-probe=false",
		"-default-data-class=" + string(defaultDataClass),
		"-report=" + *reportPath,
		"-events=" + eventsPath,
		"-session-id=" + *sessionID,
		"-turn-id=" + *turnID,
		"-pipeline-version=" + *pipelineVersion,
		"-authority-epoch=" + strconv.FormatInt(*authorityEpoch, 10),
	}

	_, _ = fmt.Fprintf(stdout, "rspp-local-runner: starting offline loopback simulation (mode=%s)\n", mode)
	if err := livekittransport.RunCLI(livekitArgs, stdout, now); err != nil {
		return err
	}
	if err := validateLoopbackReport(*reportPath, mode); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "rspp-local-runner: loopback report validated against Event ABI (%s)\n", *reportPath)
	return nil
}

type loopbackEventInput struct {
	wavPath         string
	syntheticText   string
	sessionID       string
	turnID          string
	pipelineVersion string
}

func buildLoopbackEvents(input loopbackEventInput) (loopbackInputMode, eventabi.PayloadClass, []livekittransport.Event, error) {
	if input.sessionID == "" || input.turnID == "" || input.pipelineVersion == "" {
		return "", "", nil, fmt.Errorf("loopback session_id, turn_id, and pipeline_version are required")
	}

	mode := loopbackInputSynthetic
	defaultDataClass := eventabi.PayloadTextRaw
	ingress := livekittransport.Event{
		Kind:         livekittransport.EventIngressData,
		PayloadClass: eventabi.PayloadTextRaw,
		Reason:       "loopback_ingress_synthetic_text",
	}

	if input.wavPath != "" {
		if err := validateLoopbackWAVPath(input.wavPath); err != nil {
			return "", "", nil, err
		}
		mode = loopbackInputWAV
		defaultDataClass = eventabi.PayloadAudioRaw
		ingress = livekittransport.Event{
			Kind:         livekittransport.EventIngressAudio,
			PayloadClass: eventabi.PayloadAudioRaw,
			Reason:       "loopback_ingress_wav",
		}
	}

	events := []livekittransport.Event{
		{Kind: livekittransport.EventConnected, Reason: "loopback_connected"},
		{Kind: livekittransport.EventTurnOpenProposed, Reason: "loopback_turn_open_proposed"},
		ingress,
		{Kind: livekittransport.EventOutput, Reason: "loopback_output"},
		{Kind: livekittransport.EventTerminalSuccess, Reason: "loopback_terminal_success"},
		{Kind: livekittransport.EventEnded, Reason: "loopback_session_ended"},
	}

	return mode, defaultDataClass, events, nil
}

func validateLoopbackWAVPath(path string) error {
	if !strings.EqualFold(filepath.Ext(path), ".wav") {
		return fmt.Errorf("loopback wav input must use .wav extension: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read loopback wav input %s: %w", path, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("loopback wav input %s must be non-empty", path)
	}
	return nil
}

type loopbackFixtureArtifact struct {
	Events []livekittransport.Event `json:"events"`
}

func writeLoopbackEventsFixture(events []livekittransport.Event) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "rspp-loopback-events-*")
	if err != nil {
		return "", nil, fmt.Errorf("create loopback events temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	path := filepath.Join(tmpDir, "events.json")
	payload, err := json.MarshalIndent(loopbackFixtureArtifact{Events: events}, "", "  ")
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("encode loopback events fixture: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write loopback events fixture: %w", err)
	}
	return path, cleanup, nil
}

func validateLoopbackReport(path string, mode loopbackInputMode) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read loopback report %s: %w", path, err)
	}
	var report livekittransport.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return fmt.Errorf("decode loopback report %s: %w", path, err)
	}
	if len(report.IngressRecords) == 0 {
		return fmt.Errorf("loopback report must include at least one ingress record")
	}
	if len(report.ControlSignals) == 0 {
		return fmt.Errorf("loopback report must include at least one control signal")
	}
	for idx, record := range report.IngressRecords {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("loopback ingress record[%d] invalid: %w", idx, err)
		}
	}
	for idx, signal := range report.ControlSignals {
		if err := signal.Validate(); err != nil {
			return fmt.Errorf("loopback control signal[%d] invalid: %w", idx, err)
		}
	}

	requiredClass := eventabi.PayloadTextRaw
	if mode == loopbackInputWAV {
		requiredClass = eventabi.PayloadAudioRaw
	}
	hasRequiredClass := false
	for _, record := range report.IngressRecords {
		if record.PayloadClass == requiredClass {
			hasRequiredClass = true
			break
		}
	}
	if !hasRequiredClass {
		return fmt.Errorf("loopback report missing ingress payload_class %s", requiredClass)
	}
	return nil
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "rspp-local-runner usage:")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner [livekit] [livekit-flags]")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner loopback [-wav input.wav | -synthetic-text \"hello\"] [loopback-flags]")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner livekit -dry-run=false -probe=true -url https://livekit.example.com -api-key <key> -api-secret <secret> -room rspp-demo")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner loopback -synthetic-text \"test utterance\" -report .codex/transports/loopback-local-runner-report.json")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner loopback -wav ./fixtures/sample.wav -report .codex/transports/loopback-local-runner-report.json")
}
