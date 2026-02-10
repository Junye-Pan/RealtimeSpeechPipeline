package replay

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
)

func TestJSONLFileAuditBackendResolverRequiresRoot(t *testing.T) {
	t.Parallel()

	resolver := JSONLFileAuditBackendResolver{}
	_, err := resolver.ResolveReplayAuditBackend("tenant-a")
	if !errors.Is(err, ErrReplayAuditLogRootRequired) {
		t.Fatalf("expected missing root error, got %v", err)
	}
}

func TestJSONLFileAuditBackendResolverAndBackendAppendPerTenant(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "audit")
	resolver := JSONLFileAuditBackendResolver{RootDir: root}

	tenantA, err := resolver.ResolveReplayAuditBackend("tenant-a")
	if err != nil {
		t.Fatalf("unexpected tenant-a resolver error: %v", err)
	}
	tenantB, err := resolver.ResolveReplayAuditBackend("tenant-b")
	if err != nil {
		t.Fatalf("unexpected tenant-b resolver error: %v", err)
	}

	eventA := replayAuditEventFixture("tenant-a", 100)
	if err := tenantA.AppendReplayAuditEvent(eventA); err != nil {
		t.Fatalf("unexpected tenant-a append error: %v", err)
	}

	eventB := replayAuditEventFixture("tenant-b", 200)
	if err := tenantB.AppendReplayAuditEvent(eventB); err != nil {
		t.Fatalf("unexpected tenant-b append error: %v", err)
	}

	pathA, err := resolver.TenantLogPath("tenant-a")
	if err != nil {
		t.Fatalf("unexpected tenant-a path error: %v", err)
	}
	pathB, err := resolver.TenantLogPath("tenant-b")
	if err != nil {
		t.Fatalf("unexpected tenant-b path error: %v", err)
	}
	if pathA == pathB {
		t.Fatalf("expected distinct tenant audit log paths, got %s", pathA)
	}

	rawA, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("unexpected tenant-a log read error: %v", err)
	}
	rawB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatalf("unexpected tenant-b log read error: %v", err)
	}

	linesA := strings.Split(strings.TrimSpace(string(rawA)), "\n")
	linesB := strings.Split(strings.TrimSpace(string(rawB)), "\n")
	if len(linesA) != 1 || len(linesB) != 1 {
		t.Fatalf("expected one jsonl line per tenant, got tenant-a=%d tenant-b=%d", len(linesA), len(linesB))
	}

	var decodedA obs.ReplayAuditEvent
	if err := json.Unmarshal([]byte(linesA[0]), &decodedA); err != nil {
		t.Fatalf("unexpected tenant-a decode error: %v", err)
	}
	var decodedB obs.ReplayAuditEvent
	if err := json.Unmarshal([]byte(linesB[0]), &decodedB); err != nil {
		t.Fatalf("unexpected tenant-b decode error: %v", err)
	}
	if decodedA.Request.TenantID != "tenant-a" || decodedB.Request.TenantID != "tenant-b" {
		t.Fatalf("unexpected tenant routing in audit logs: a=%+v b=%+v", decodedA, decodedB)
	}
}

func TestJSONLFileAuditBackendRejectsInvalidEvent(t *testing.T) {
	t.Parallel()

	backend := &JSONLFileAuditBackend{Path: filepath.Join(t.TempDir(), "audit.jsonl")}
	err := backend.AppendReplayAuditEvent(obs.ReplayAuditEvent{
		TimestampMS: -1,
		Request: obs.ReplayAccessRequest{
			TenantID:          "tenant-a",
			PrincipalID:       "user-1",
			Role:              "oncall",
			Purpose:           "incident",
			RequestedScope:    "turn:1",
			RequestedFidelity: obs.ReplayFidelityL0,
		},
		Decision: obs.ReplayAccessDecision{Allowed: true},
	})
	if err == nil {
		t.Fatalf("expected invalid audit event append to fail")
	}
}

func TestDefaultReplayAuditTenantFilenameSanitizesUnsafeCharacters(t *testing.T) {
	t.Parallel()

	name, err := defaultReplayAuditTenantFilename("tenant/../a")
	if err != nil {
		t.Fatalf("unexpected sanitize error: %v", err)
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		t.Fatalf("expected sanitized tenant filename, got %q", name)
	}
}

func replayAuditEventFixture(tenantID string, ts int64) obs.ReplayAuditEvent {
	return obs.ReplayAuditEvent{
		TimestampMS: ts,
		Request: obs.ReplayAccessRequest{
			TenantID:          tenantID,
			PrincipalID:       "user-1",
			Role:              "oncall",
			Purpose:           "incident",
			RequestedScope:    "turn:1",
			RequestedFidelity: obs.ReplayFidelityL0,
		},
		Decision: obs.ReplayAccessDecision{Allowed: true},
	}
}
