package main

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
)

func TestRunPublishListGetRollbackAndAudit(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "cp-state.json")
	env := func(key string) string {
		if key == envControlPlaneStatePath {
			return statePath
		}
		if key == envControlPlaneActor {
			return "test-actor"
		}
		return ""
	}

	now := time.Date(2026, time.February, 16, 23, 35, 0, 0, time.UTC)
	clock := func() time.Time {
		now = now.Add(time.Second)
		return now
	}

	if err := run(
		[]string{"publish", "pipeline-v1", "graph/default", "simple"},
		io.Discard,
		io.Discard,
		clock,
		env,
	); err != nil {
		t.Fatalf("publish v1 failed: %v", err)
	}
	if err := run(
		[]string{"publish", "pipeline-v2", "graph/v2", "simple"},
		io.Discard,
		io.Discard,
		clock,
		env,
	); err != nil {
		t.Fatalf("publish v2 failed: %v", err)
	}

	resolveRouteOut := new(strings.Builder)
	if err := run(
		[]string{"resolve-session-route", "tenant-a", "sess-a", "pipeline-v1", "livekit", "7"},
		resolveRouteOut,
		io.Discard,
		clock,
		env,
	); err != nil {
		t.Fatalf("resolve-session-route failed: %v", err)
	}
	var route controlplane.SessionRoute
	if err := json.Unmarshal([]byte(resolveRouteOut.String()), &route); err != nil {
		t.Fatalf("decode route output: %v", err)
	}
	if err := route.Validate(); err != nil {
		t.Fatalf("expected valid route output, got %v", err)
	}
	if route.PipelineVersion != "pipeline-v1" {
		t.Fatalf("expected requested pipeline version in route output, got %+v", route)
	}

	issueTokenOut := new(strings.Builder)
	if err := run(
		[]string{"issue-session-token", "tenant-a", "sess-a", "pipeline-v1", "livekit", "7", "120"},
		issueTokenOut,
		io.Discard,
		clock,
		env,
	); err != nil {
		t.Fatalf("issue-session-token failed: %v", err)
	}
	var token controlplane.SignedSessionToken
	if err := json.Unmarshal([]byte(issueTokenOut.String()), &token); err != nil {
		t.Fatalf("decode token output: %v", err)
	}
	if err := token.Validate(); err != nil {
		t.Fatalf("expected valid token output, got %v", err)
	}

	if err := run(
		[]string{"set-session-status", "tenant-a", "sess-a", "running", "pipeline-v1", "7", "integration_test"},
		io.Discard,
		io.Discard,
		clock,
		env,
	); err != nil {
		t.Fatalf("set-session-status failed: %v", err)
	}

	listOut := new(strings.Builder)
	if err := run([]string{"list"}, listOut, io.Discard, clock, env); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var listed []struct {
		PipelineVersion string `json:"pipeline_version"`
		CreatedAtUTC    string `json:"created_at_utc"`
		Author          string `json:"author"`
		SpecHash        string `json:"spec_hash"`
		Active          bool   `json:"active"`
	}
	if err := json.Unmarshal([]byte(listOut.String()), &listed); err != nil {
		t.Fatalf("decode list output: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 listed versions, got %d", len(listed))
	}
	if listed[0].PipelineVersion != "pipeline-v1" || listed[1].PipelineVersion != "pipeline-v2" {
		t.Fatalf("unexpected list order/content: %+v", listed)
	}
	if listed[0].Active || !listed[1].Active {
		t.Fatalf("expected pipeline-v2 active after second publish, got %+v", listed)
	}
	if listed[0].CreatedAtUTC == "" || listed[0].Author != "test-actor" || listed[0].SpecHash == "" {
		t.Fatalf("expected metadata fields in list output, got %+v", listed[0])
	}

	getOut := new(strings.Builder)
	if err := run([]string{"get", "pipeline-v1"}, getOut, io.Discard, clock, env); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	var got struct {
		PipelineVersion string `json:"pipeline_version"`
		CreatedAtUTC    string `json:"created_at_utc"`
		Author          string `json:"author"`
		SpecHash        string `json:"spec_hash"`
		Active          bool   `json:"active"`
	}
	if err := json.Unmarshal([]byte(getOut.String()), &got); err != nil {
		t.Fatalf("decode get output: %v", err)
	}
	if got.PipelineVersion != "pipeline-v1" || got.Active {
		t.Fatalf("unexpected get output: %+v", got)
	}
	if got.CreatedAtUTC == "" || got.Author != "test-actor" || got.SpecHash == "" {
		t.Fatalf("expected metadata fields in get output, got %+v", got)
	}

	if err := run([]string{"rollback", "pipeline-v1"}, io.Discard, io.Discard, clock, env); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	statusOut := new(strings.Builder)
	if err := run([]string{"status"}, statusOut, io.Discard, clock, env); err != nil {
		t.Fatalf("status failed: %v", err)
	}
	var status struct {
		ActiveVersion string `json:"active_version"`
	}
	if err := json.Unmarshal([]byte(statusOut.String()), &status); err != nil {
		t.Fatalf("decode status output: %v", err)
	}
	if status.ActiveVersion != "pipeline-v1" {
		t.Fatalf("expected active version pipeline-v1 after rollback, got %q", status.ActiveVersion)
	}

	sessionStatusOut := new(strings.Builder)
	if err := run([]string{"session-status", "tenant-a", "sess-a"}, sessionStatusOut, io.Discard, clock, env); err != nil {
		t.Fatalf("session-status failed: %v", err)
	}
	var sessionStatus controlplane.SessionStatusView
	if err := json.Unmarshal([]byte(sessionStatusOut.String()), &sessionStatus); err != nil {
		t.Fatalf("decode session-status output: %v", err)
	}
	if sessionStatus.Status != controlplane.SessionStatusRunning || sessionStatus.LastReason != "integration_test" {
		t.Fatalf("expected running session status from persisted state, got %+v", sessionStatus)
	}

	auditOut := new(strings.Builder)
	if err := run([]string{"audit"}, auditOut, io.Discard, clock, env); err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	var audit []auditEntry
	if err := json.Unmarshal([]byte(auditOut.String()), &audit); err != nil {
		t.Fatalf("decode audit output: %v", err)
	}
	if len(audit) != 6 {
		t.Fatalf("expected 6 audit entries, got %d", len(audit))
	}
	if audit[0].Action != "publish" ||
		audit[1].Action != "publish" ||
		audit[2].Action != "resolve_session_route" ||
		audit[3].Action != "issue_session_token" ||
		audit[4].Action != "set_session_status" ||
		audit[5].Action != "rollback" {
		t.Fatalf("unexpected audit actions: %+v", audit)
	}
	if audit[0].Actor != "test-actor" || audit[0].ChangeHash == "" {
		t.Fatalf("expected actor/hash metadata in publish audit entry, got %+v", audit[0])
	}
	if audit[5].Actor != "test-actor" || audit[5].ChangeHash == "" {
		t.Fatalf("expected actor/hash metadata in rollback audit entry, got %+v", audit[5])
	}
}

func TestRunRejectsDuplicatePublish(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "cp-state.json")
	env := func(key string) string {
		if key == envControlPlaneStatePath {
			return statePath
		}
		return ""
	}

	if err := run(
		[]string{"publish", "pipeline-v1", "graph/default", "simple"},
		io.Discard,
		io.Discard,
		time.Now,
		env,
	); err != nil {
		t.Fatalf("initial publish failed: %v", err)
	}

	err := run(
		[]string{"publish", "pipeline-v1", "graph/other", "simple"},
		io.Discard,
		io.Discard,
		time.Now,
		env,
	)
	if err == nil {
		t.Fatalf("expected duplicate publish to fail")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestRunUnknownCommandFails(t *testing.T) {
	t.Parallel()

	err := run([]string{"nope"}, io.Discard, io.Discard, time.Now, func(string) string { return "" })
	if err == nil {
		t.Fatalf("expected unknown command error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected unknown command error: %v", err)
	}
}

func TestRunSessionStatusDefaultsToEnded(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "cp-state.json")
	env := func(key string) string {
		if key == envControlPlaneStatePath {
			return statePath
		}
		return ""
	}

	statusOut := new(strings.Builder)
	if err := run([]string{"session-status", "tenant-x", "sess-x"}, statusOut, io.Discard, time.Now, env); err != nil {
		t.Fatalf("session-status failed: %v", err)
	}
	var status controlplane.SessionStatusView
	if err := json.Unmarshal([]byte(statusOut.String()), &status); err != nil {
		t.Fatalf("decode session-status output: %v", err)
	}
	if status.Status != controlplane.SessionStatusEnded || status.LastReason != "session_not_found" {
		t.Fatalf("expected default ended status for unknown session, got %+v", status)
	}
}
