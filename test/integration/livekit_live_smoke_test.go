//go:build livekitproviders

package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	livekittransport "github.com/tiger/realtime-speech-pipeline/transports/livekit"
)

func TestLiveKitSmoke(t *testing.T) {
	t.Parallel()

	if os.Getenv("RSPP_LIVEKIT_SMOKE") != "1" {
		t.Skip("livekit smoke disabled (set RSPP_LIVEKIT_SMOKE=1)")
	}
	if os.Getenv(livekittransport.EnvURL) == "" || os.Getenv(livekittransport.EnvAPIKey) == "" || os.Getenv(livekittransport.EnvAPISecret) == "" {
		t.Skip("livekit smoke missing required credentials/env")
	}

	cfg := livekittransport.Config{
		URL:              os.Getenv(livekittransport.EnvURL),
		APIKey:           os.Getenv(livekittransport.EnvAPIKey),
		APISecret:        os.Getenv(livekittransport.EnvAPISecret),
		Room:             defaultString(os.Getenv(livekittransport.EnvRoom), "rspp-smoke-room"),
		SessionID:        "sess-livekit-smoke",
		TurnID:           "turn-livekit-smoke",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           false,
		Probe:            true,
		RequestTimeout:   8 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("livekit smoke config invalid: %v", err)
	}

	probeResult, err := livekittransport.ProbeRoomService(context.Background(), cfg, nil, time.Now)
	if err != nil {
		t.Fatalf("livekit room-service probe failed: %v", err)
	}
	if probeResult.Status != livekittransport.ProbeStatusPassed {
		t.Fatalf("unexpected probe status: %+v", probeResult)
	}

	adapter, err := livekittransport.NewAdapter(cfg, livekittransport.Dependencies{Now: time.Now})
	if err != nil {
		t.Fatalf("new livekit adapter: %v", err)
	}
	report, err := adapter.Process(context.Background(), livekittransport.DefaultScenario(cfg))
	if err != nil {
		t.Fatalf("process deterministic adapter flow: %v", err)
	}
	report.Probe = probeResult

	reportPath := defaultString(os.Getenv("RSPP_LIVEKIT_SMOKE_REPORT_PATH"), ".codex/providers/livekit-smoke-report.json")
	if err := writeSmokeReport(reportPath, report); err != nil {
		t.Fatalf("write livekit smoke report: %v", err)
	}
}

func writeSmokeReport(path string, report livekittransport.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func defaultString(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
