package integration_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	replay "github.com/tiger/realtime-speech-pipeline/internal/observability/replay"
)

func TestReplayAuditResolverFallsBackToJSONLWhenHTTPUnavailable(t *testing.T) {
	t.Parallel()

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	downURL := down.URL
	down.Close()

	root := filepath.Join(t.TempDir(), "audit")
	resolver := replay.FallbackAuditBackendResolver{
		Primary: replay.HTTPAuditBackendResolver{
			Config: replay.HTTPAuditBackendConfig{
				URLs:             []string{downURL},
				RetryMaxAttempts: 1,
			},
		},
		Fallback: replay.JSONLFileAuditBackendResolver{RootDir: root},
	}

	service := replay.AccessService{
		Policy: replay.DefaultAccessPolicy("tenant-a"),
		AuditSink: replay.ResolverBackedAuditSink{
			Resolver: resolver,
		},
		NowMS: func() int64 { return 777 },
	}

	decision, err := service.AuthorizeReplayAccess(obs.ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:1",
		RequestedFidelity: obs.ReplayFidelityL0,
	})
	if err != nil {
		t.Fatalf("expected fallback audit append success, got %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected access decision to remain allowed when fallback sink succeeds, got %+v", decision)
	}

	path, err := (replay.JSONLFileAuditBackendResolver{RootDir: root}).TenantLogPath("tenant-a")
	if err != nil {
		t.Fatalf("resolve tenant fallback log path: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fallback tenant log: %v", err)
	}
	if !strings.Contains(string(raw), "\"timestamp_ms\":777") {
		t.Fatalf("expected fallback jsonl replay-audit entry, got %q", string(raw))
	}
}
