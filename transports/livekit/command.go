package livekit

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

// RunCLI executes the LiveKit transport command workflow used by runtime and local-runner entrypoints.
func RunCLI(args []string, stdout io.Writer, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}

	baseCfg, err := ConfigFromEnv()
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("livekit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	reportPath := fs.String("report", filepath.Join(".codex", "transports", "livekit-report.json"), "path to write livekit transport report json")
	eventsPath := fs.String("events", "", "optional path to deterministic livekit event fixture json")
	dryRun := fs.Bool("dry-run", baseCfg.DryRun, "run deterministic fixture processing without remote probe requirement")
	probe := fs.Bool("probe", baseCfg.Probe, "probe LiveKit RoomService list endpoint before processing")
	url := fs.String("url", baseCfg.URL, "LiveKit URL, for example https://livekit.example.com")
	apiKey := fs.String("api-key", baseCfg.APIKey, "LiveKit API key")
	apiSecret := fs.String("api-secret", baseCfg.APISecret, "LiveKit API secret")
	room := fs.String("room", baseCfg.Room, "LiveKit room name")
	sessionID := fs.String("session-id", baseCfg.SessionID, "runtime session id")
	turnID := fs.String("turn-id", baseCfg.TurnID, "runtime turn id")
	pipelineVersion := fs.String("pipeline-version", baseCfg.PipelineVersion, "pipeline version")
	authorityEpoch := fs.Int64("authority-epoch", baseCfg.AuthorityEpoch, "authority epoch")
	defaultDataClass := fs.String("default-data-class", string(baseCfg.DefaultDataClass), "default ingress data payload class")
	requestTimeoutMS := fs.Int64("request-timeout-ms", baseCfg.RequestTimeout.Milliseconds(), "room service probe timeout in milliseconds")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := Config{
		URL:              strings.TrimSpace(*url),
		APIKey:           strings.TrimSpace(*apiKey),
		APISecret:        strings.TrimSpace(*apiSecret),
		Room:             strings.TrimSpace(*room),
		SessionID:        strings.TrimSpace(*sessionID),
		TurnID:           strings.TrimSpace(*turnID),
		PipelineVersion:  strings.TrimSpace(*pipelineVersion),
		AuthorityEpoch:   *authorityEpoch,
		DefaultDataClass: eventabi.PayloadClass(strings.TrimSpace(*defaultDataClass)),
		DryRun:           *dryRun,
		Probe:            *probe,
		RequestTimeout:   time.Duration(*requestTimeoutMS) * time.Millisecond,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	var events []Event
	if strings.TrimSpace(*eventsPath) == "" {
		events = DefaultScenario(cfg)
	} else {
		events, err = LoadEvents(*eventsPath)
		if err != nil {
			return err
		}
	}

	adapter, err := NewAdapter(cfg, Dependencies{Now: now})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
	defer cancel()

	report, err := adapter.Process(ctx, events)
	if err != nil {
		return err
	}

	if cfg.Probe {
		probeResult, probeErr := ProbeRoomService(ctx, cfg, nil, now)
		report.Probe = probeResult
		if probeErr != nil {
			if err := writeJSONArtifact(*reportPath, report); err != nil {
				return err
			}
			return probeErr
		}
	}

	if err := writeJSONArtifact(*reportPath, report); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(
		stdout,
		"livekit transport: report=%s events=%d ingress=%d control=%d lifecycle=%d probe=%s\n",
		*reportPath,
		report.ProcessedEvents,
		len(report.IngressRecords),
		len(report.ControlSignals),
		len(report.LifecycleEvents),
		report.Probe.Status,
	)
	return nil
}

func writeJSONArtifact(path string, payload any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("report path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory for %s: %w", path, err)
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report %s: %w", path, err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write report %s: %w", path, err)
	}
	return nil
}

type eventArtifact struct {
	Events []Event `json:"events"`
}

// LoadEvents loads deterministic event fixtures from either a JSON array or {"events":[...]} object.
func LoadEvents(path string) ([]Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read livekit events fixture %s: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("livekit events fixture %s is empty", path)
	}

	if strings.HasPrefix(trimmed, "[") {
		var events []Event
		if err := json.Unmarshal(raw, &events); err != nil {
			return nil, fmt.Errorf("decode livekit events fixture %s: %w", path, err)
		}
		if len(events) == 0 {
			return nil, fmt.Errorf("livekit events fixture %s must contain at least one event", path)
		}
		return events, nil
	}

	var artifact eventArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil, fmt.Errorf("decode livekit event artifact %s: %w", path, err)
	}
	if len(artifact.Events) == 0 {
		return nil, fmt.Errorf("livekit event artifact %s must contain events", path)
	}
	return artifact.Events, nil
}
