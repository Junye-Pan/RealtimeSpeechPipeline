package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/registry"
	"github.com/tiger/realtime-speech-pipeline/internal/controlplane/sessionroute"
)

const (
	envControlPlaneStatePath = "RSPP_CP_STATE_PATH"
	envControlPlaneActor     = "RSPP_CP_ACTOR"
	envSessionTokenSecret    = "RSPP_CP_TOKEN_SECRET"
	envSessionTokenTTL       = "RSPP_CP_TOKEN_TTL_SECONDS"
	defaultControlPlaneState = ".codex/controlplane/process-state.json"
)

type pipelineRecord struct {
	PipelineVersion    string `json:"pipeline_version"`
	GraphDefinitionRef string `json:"graph_definition_ref"`
	ExecutionProfile   string `json:"execution_profile"`
	CreatedAtUTC       string `json:"created_at_utc,omitempty"`
	PublishedAtUTC     string `json:"published_at_utc,omitempty"` // legacy alias
	Author             string `json:"author,omitempty"`
	SpecHash           string `json:"spec_hash,omitempty"`
}

type auditEntry struct {
	Seq         int    `json:"seq"`
	AtUTC       string `json:"at_utc"`
	Actor       string `json:"actor,omitempty"`
	Action      string `json:"action"`
	Version     string `json:"version,omitempty"`
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
	ChangeHash  string `json:"change_hash,omitempty"`
}

type processState struct {
	ActiveVersion string                                    `json:"active_version,omitempty"`
	Pipelines     map[string]pipelineRecord                 `json:"pipelines,omitempty"`
	Sessions      map[string]controlplane.SessionStatusView `json:"sessions,omitempty"`
	Audit         []auditEntry                              `json:"audit,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now, os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "rspp-control-plane: %v\n", err)
		os.Exit(1)
	}
}

func run(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	now func() time.Time,
	getenv func(string) string,
) error {
	if len(args) == 0 || isHelpFlag(args[0]) {
		printUsage(stdout)
		return nil
	}

	statePath := strings.TrimSpace(getenv(envControlPlaneStatePath))
	if statePath == "" {
		statePath = defaultControlPlaneState
	}

	switch args[0] {
	case "publish":
		if len(args) < 3 {
			return fmt.Errorf("publish requires <pipeline_version> <graph_definition_ref> [execution_profile]")
		}
		executionProfile := "simple"
		if len(args) >= 4 {
			executionProfile = strings.TrimSpace(args[3])
		}
		record := pipelineRecord{
			PipelineVersion:    strings.TrimSpace(args[1]),
			GraphDefinitionRef: strings.TrimSpace(args[2]),
			ExecutionProfile:   executionProfile,
			CreatedAtUTC:       now().UTC().Format(time.RFC3339),
			PublishedAtUTC:     now().UTC().Format(time.RFC3339),
			Author:             controlPlaneActor(getenv),
		}
		record.SpecHash = pipelineRecordHash(record)
		if err := validateRecord(record); err != nil {
			return err
		}
		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		if _, exists := state.Pipelines[record.PipelineVersion]; exists {
			return fmt.Errorf("pipeline version %q already exists", record.PipelineVersion)
		}
		fromVersion := state.ActiveVersion
		state.Pipelines[record.PipelineVersion] = record
		state.ActiveVersion = record.PipelineVersion
		appendAudit(&state, now, auditEntry{
			Action:      "publish",
			Version:     record.PipelineVersion,
			FromVersion: fromVersion,
			ToVersion:   record.PipelineVersion,
			Actor:       controlPlaneActor(getenv),
			ChangeHash:  record.SpecHash,
		})
		if err := saveState(statePath, state); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "published %s (active=%s)\n", record.PipelineVersion, state.ActiveVersion)
		return nil
	case "list":
		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		type listItem struct {
			PipelineVersion    string `json:"pipeline_version"`
			GraphDefinitionRef string `json:"graph_definition_ref"`
			ExecutionProfile   string `json:"execution_profile"`
			CreatedAtUTC       string `json:"created_at_utc"`
			Author             string `json:"author"`
			SpecHash           string `json:"spec_hash"`
			Active             bool   `json:"active"`
		}
		versions := make([]string, 0, len(state.Pipelines))
		for version := range state.Pipelines {
			versions = append(versions, version)
		}
		sort.Strings(versions)
		items := make([]listItem, 0, len(versions))
		for _, version := range versions {
			record := state.Pipelines[version]
			items = append(items, listItem{
				PipelineVersion:    version,
				GraphDefinitionRef: record.GraphDefinitionRef,
				ExecutionProfile:   record.ExecutionProfile,
				CreatedAtUTC:       record.CreatedAtUTC,
				Author:             record.Author,
				SpecHash:           record.SpecHash,
				Active:             version == state.ActiveVersion,
			})
		}
		return writeJSON(stdout, items)
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("get requires <pipeline_version>")
		}
		version := strings.TrimSpace(args[1])
		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		record, ok := state.Pipelines[version]
		if !ok {
			return fmt.Errorf("unknown pipeline version %q", version)
		}
		out := struct {
			PipelineVersion    string `json:"pipeline_version"`
			GraphDefinitionRef string `json:"graph_definition_ref"`
			ExecutionProfile   string `json:"execution_profile"`
			CreatedAtUTC       string `json:"created_at_utc"`
			Author             string `json:"author"`
			SpecHash           string `json:"spec_hash"`
			Active             bool   `json:"active"`
		}{
			PipelineVersion:    record.PipelineVersion,
			GraphDefinitionRef: record.GraphDefinitionRef,
			ExecutionProfile:   record.ExecutionProfile,
			CreatedAtUTC:       record.CreatedAtUTC,
			Author:             record.Author,
			SpecHash:           record.SpecHash,
			Active:             version == state.ActiveVersion,
		}
		return writeJSON(stdout, out)
	case "rollback":
		if len(args) < 2 {
			return fmt.Errorf("rollback requires <pipeline_version>")
		}
		version := strings.TrimSpace(args[1])
		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		if _, ok := state.Pipelines[version]; !ok {
			return fmt.Errorf("unknown pipeline version %q", version)
		}
		fromVersion := state.ActiveVersion
		state.ActiveVersion = version
		appendAudit(&state, now, auditEntry{
			Action:      "rollback",
			Version:     version,
			FromVersion: fromVersion,
			ToVersion:   version,
			Actor:       controlPlaneActor(getenv),
			ChangeHash:  rolloutChangeHash(fromVersion, version),
		})
		if err := saveState(statePath, state); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "rolled back active version to %s\n", version)
		return nil
	case "status":
		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		out := struct {
			ActiveVersion  string `json:"active_version,omitempty"`
			TotalPipelines int    `json:"total_pipelines"`
			AuditEntries   int    `json:"audit_entries"`
			StatePath      string `json:"state_path"`
		}{
			ActiveVersion:  state.ActiveVersion,
			TotalPipelines: len(state.Pipelines),
			AuditEntries:   len(state.Audit),
			StatePath:      statePath,
		}
		return writeJSON(stdout, out)
	case "audit":
		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		return writeJSON(stdout, state.Audit)
	case "resolve-session-route":
		if len(args) < 3 {
			return fmt.Errorf("resolve-session-route requires <tenant_id> <session_id> [requested_pipeline_version] [transport_kind] [authority_epoch]")
		}
		tenantID := strings.TrimSpace(args[1])
		sessionID := strings.TrimSpace(args[2])
		requestedPipelineVersion := ""
		if len(args) >= 4 {
			requestedPipelineVersion = strings.TrimSpace(args[3])
		}
		transportKind := ""
		if len(args) >= 5 {
			transportKind = strings.TrimSpace(args[4])
		}
		authorityEpoch := int64(0)
		if len(args) >= 6 {
			parsedEpoch, err := strconv.ParseInt(strings.TrimSpace(args[5]), 10, 64)
			if err != nil {
				return fmt.Errorf("parse authority_epoch: %w", err)
			}
			authorityEpoch = parsedEpoch
		}

		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		service := newSessionRouteService(state, now, getenv)
		route, err := service.ResolveSessionRoute(controlplane.SessionRouteRequest{
			TenantID:                 tenantID,
			SessionID:                sessionID,
			RequestedPipelineVersion: requestedPipelineVersion,
			TransportKind:            transportKind,
			RequestedAuthorityEpoch:  authorityEpoch,
		})
		if err != nil {
			return err
		}
		status := controlplane.SessionStatusView{
			TenantID:        route.TenantID,
			SessionID:       route.SessionID,
			Status:          controlplane.SessionStatusConnected,
			PipelineVersion: route.PipelineVersion,
			AuthorityEpoch:  route.Lease.AuthorityEpoch,
			UpdatedAtUTC:    now().UTC().Format(time.RFC3339),
			LastReason:      "session_route_resolved",
		}
		if err := upsertSessionStatus(&state, status); err != nil {
			return err
		}
		appendAudit(&state, now, auditEntry{
			Action:     "resolve_session_route",
			Version:    route.PipelineVersion,
			Actor:      controlPlaneActor(getenv),
			ChangeHash: sessionStatusChangeHash(status),
		})
		if err := saveState(statePath, state); err != nil {
			return err
		}
		return writeJSON(stdout, route)
	case "issue-session-token":
		if len(args) < 3 {
			return fmt.Errorf("issue-session-token requires <tenant_id> <session_id> [requested_pipeline_version] [transport_kind] [authority_epoch] [ttl_seconds]")
		}
		tenantID := strings.TrimSpace(args[1])
		sessionID := strings.TrimSpace(args[2])
		requestedPipelineVersion := ""
		if len(args) >= 4 {
			requestedPipelineVersion = strings.TrimSpace(args[3])
		}
		transportKind := ""
		if len(args) >= 5 {
			transportKind = strings.TrimSpace(args[4])
		}
		authorityEpoch := int64(0)
		if len(args) >= 6 {
			parsedEpoch, err := strconv.ParseInt(strings.TrimSpace(args[5]), 10, 64)
			if err != nil {
				return fmt.Errorf("parse authority_epoch: %w", err)
			}
			authorityEpoch = parsedEpoch
		}
		ttlSeconds := 0
		if len(args) >= 7 {
			parsedTTL, err := strconv.Atoi(strings.TrimSpace(args[6]))
			if err != nil {
				return fmt.Errorf("parse ttl_seconds: %w", err)
			}
			ttlSeconds = parsedTTL
		}

		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		service := newSessionRouteService(state, now, getenv)
		token, err := service.IssueSessionToken(controlplane.SessionTokenRequest{
			TenantID:                 tenantID,
			SessionID:                sessionID,
			RequestedPipelineVersion: requestedPipelineVersion,
			TransportKind:            transportKind,
			RequestedAuthorityEpoch:  authorityEpoch,
			TTLSeconds:               ttlSeconds,
		})
		if err != nil {
			return err
		}
		status := controlplane.SessionStatusView{
			TenantID:        token.Claims.TenantID,
			SessionID:       token.Claims.SessionID,
			Status:          controlplane.SessionStatusConnected,
			PipelineVersion: token.Claims.PipelineVersion,
			AuthorityEpoch:  token.Claims.AuthorityEpoch,
			UpdatedAtUTC:    now().UTC().Format(time.RFC3339),
			LastReason:      "session_token_issued",
		}
		if err := upsertSessionStatus(&state, status); err != nil {
			return err
		}
		appendAudit(&state, now, auditEntry{
			Action:     "issue_session_token",
			Version:    token.Claims.PipelineVersion,
			Actor:      controlPlaneActor(getenv),
			ChangeHash: sessionTokenChangeHash(token),
		})
		if err := saveState(statePath, state); err != nil {
			return err
		}
		return writeJSON(stdout, token)
	case "session-status":
		if len(args) < 3 {
			return fmt.Errorf("session-status requires <tenant_id> <session_id>")
		}
		tenantID := strings.TrimSpace(args[1])
		sessionID := strings.TrimSpace(args[2])
		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		status := sessionStatusOrDefault(state, tenantID, sessionID, now)
		return writeJSON(stdout, status)
	case "set-session-status":
		if len(args) < 4 {
			return fmt.Errorf("set-session-status requires <tenant_id> <session_id> <status> [pipeline_version] [authority_epoch] [reason]")
		}
		tenantID := strings.TrimSpace(args[1])
		sessionID := strings.TrimSpace(args[2])
		statusKind := controlplane.SessionStatus(strings.TrimSpace(args[3]))
		pipelineVersion := ""
		if len(args) >= 5 {
			pipelineVersion = strings.TrimSpace(args[4])
		}
		authorityEpoch := int64(0)
		if len(args) >= 6 {
			parsedEpoch, err := strconv.ParseInt(strings.TrimSpace(args[5]), 10, 64)
			if err != nil {
				return fmt.Errorf("parse authority_epoch: %w", err)
			}
			authorityEpoch = parsedEpoch
		}
		reason := "operator_status_update"
		if len(args) >= 7 {
			reason = strings.TrimSpace(args[6])
		}

		state, err := loadState(statePath)
		if err != nil {
			return err
		}
		status := controlplane.SessionStatusView{
			TenantID:        tenantID,
			SessionID:       sessionID,
			Status:          statusKind,
			PipelineVersion: pipelineVersion,
			AuthorityEpoch:  authorityEpoch,
			UpdatedAtUTC:    now().UTC().Format(time.RFC3339),
			LastReason:      reason,
		}
		if err := upsertSessionStatus(&state, status); err != nil {
			return err
		}
		appendAudit(&state, now, auditEntry{
			Action:     "set_session_status",
			Version:    pipelineVersion,
			Actor:      controlPlaneActor(getenv),
			ChangeHash: sessionStatusChangeHash(status),
		})
		if err := saveState(statePath, state); err != nil {
			return err
		}
		return writeJSON(stdout, status)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func isHelpFlag(arg string) bool {
	switch arg {
	case "help", "-h", "--help":
		return true
	default:
		return false
	}
}

func validateRecord(record pipelineRecord) error {
	if record.PipelineVersion == "" {
		return fmt.Errorf("pipeline_version is required")
	}
	if record.GraphDefinitionRef == "" {
		return fmt.Errorf("graph_definition_ref is required")
	}
	if record.ExecutionProfile == "" {
		return fmt.Errorf("execution_profile is required")
	}
	if strings.TrimSpace(record.CreatedAtUTC) == "" {
		return fmt.Errorf("created_at_utc is required")
	}
	if strings.TrimSpace(record.Author) == "" {
		return fmt.Errorf("author is required")
	}
	if strings.TrimSpace(record.SpecHash) == "" {
		return fmt.Errorf("spec_hash is required")
	}
	return nil
}

func loadState(path string) (processState, error) {
	if strings.TrimSpace(path) == "" {
		return processState{}, fmt.Errorf("state path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return processState{
				Pipelines: map[string]pipelineRecord{},
				Audit:     []auditEntry{},
			}, nil
		}
		return processState{}, fmt.Errorf("read state: %w", err)
	}
	var state processState
	if err := json.Unmarshal(raw, &state); err != nil {
		return processState{}, fmt.Errorf("decode state: %w", err)
	}
	if state.Pipelines == nil {
		state.Pipelines = map[string]pipelineRecord{}
	}
	if state.Sessions == nil {
		state.Sessions = map[string]controlplane.SessionStatusView{}
	}
	if state.Audit == nil {
		state.Audit = []auditEntry{}
	}
	for version, record := range state.Pipelines {
		state.Pipelines[version] = normalizePipelineRecord(record)
	}
	return state, nil
}

func saveState(path string, state processState) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("state path is required")
	}
	if state.Pipelines == nil {
		state.Pipelines = map[string]pipelineRecord{}
	}
	if state.Sessions == nil {
		state.Sessions = map[string]controlplane.SessionStatusView{}
	}
	if state.Audit == nil {
		state.Audit = []auditEntry{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func appendAudit(state *processState, now func() time.Time, entry auditEntry) {
	if state == nil {
		return
	}
	entry.Seq = len(state.Audit) + 1
	entry.AtUTC = now().UTC().Format(time.RFC3339)
	state.Audit = append(state.Audit, entry)
}

func controlPlaneActor(getenv func(string) string) string {
	if getenv == nil {
		return "system"
	}
	actor := strings.TrimSpace(getenv(envControlPlaneActor))
	if actor != "" {
		return actor
	}
	return "system"
}

func pipelineRecordHash(record pipelineRecord) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		record.PipelineVersion,
		record.GraphDefinitionRef,
		record.ExecutionProfile,
		record.CreatedAtUTC,
		record.Author,
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func rolloutChangeHash(fromVersion string, toVersion string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(fromVersion),
		strings.TrimSpace(toVersion),
	}, "->")))
	return hex.EncodeToString(sum[:])
}

func sessionTokenChangeHash(token controlplane.SignedSessionToken) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(token.TokenID),
		strings.TrimSpace(token.ExpiresAtUTC),
		strings.TrimSpace(token.Claims.TenantID),
		strings.TrimSpace(token.Claims.SessionID),
		strings.TrimSpace(token.Claims.PipelineVersion),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func sessionStatusChangeHash(status controlplane.SessionStatusView) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(status.TenantID),
		strings.TrimSpace(status.SessionID),
		string(status.Status),
		strings.TrimSpace(status.PipelineVersion),
		fmt.Sprintf("%d", status.AuthorityEpoch),
		strings.TrimSpace(status.UpdatedAtUTC),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func upsertSessionStatus(state *processState, status controlplane.SessionStatusView) error {
	if state == nil {
		return fmt.Errorf("state is required")
	}
	if err := status.Validate(); err != nil {
		return err
	}
	if state.Sessions == nil {
		state.Sessions = map[string]controlplane.SessionStatusView{}
	}
	state.Sessions[sessionStateKey(status.TenantID, status.SessionID)] = status
	return nil
}

func sessionStatusOrDefault(state processState, tenantID string, sessionID string, now func() time.Time) controlplane.SessionStatusView {
	key := sessionStateKey(tenantID, sessionID)
	if view, ok := state.Sessions[key]; ok {
		if err := view.Validate(); err == nil {
			return view
		}
	}
	return controlplane.SessionStatusView{
		TenantID:     strings.TrimSpace(tenantID),
		SessionID:    strings.TrimSpace(sessionID),
		Status:       controlplane.SessionStatusEnded,
		UpdatedAtUTC: now().UTC().Format(time.RFC3339),
		LastReason:   "session_not_found",
	}
}

func sessionStateKey(tenantID string, sessionID string) string {
	return strings.TrimSpace(tenantID) + "/" + strings.TrimSpace(sessionID)
}

func newSessionRouteService(state processState, now func() time.Time, getenv func(string) string) *sessionroute.Service {
	service := sessionroute.NewService()
	service.Now = now
	service.Registry.Backend = processStateRegistryBackend{state: state}
	if getenv != nil {
		if secret := strings.TrimSpace(getenv(envSessionTokenSecret)); secret != "" {
			service.TokenSecret = secret
		}
		if ttlRaw := strings.TrimSpace(getenv(envSessionTokenTTL)); ttlRaw != "" {
			if ttlSeconds, err := strconv.Atoi(ttlRaw); err == nil && ttlSeconds > 0 {
				service.DefaultTokenTTL = time.Duration(ttlSeconds) * time.Second
			}
		}
	}
	return &service
}

type processStateRegistryBackend struct {
	state processState
}

func (b processStateRegistryBackend) ResolvePipelineRecord(version string) (registry.PipelineRecord, error) {
	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = strings.TrimSpace(b.state.ActiveVersion)
	}
	if resolvedVersion == "" {
		return registry.PipelineRecord{}, fmt.Errorf("pipeline version is required")
	}

	record, ok := b.state.Pipelines[resolvedVersion]
	if !ok {
		return registry.PipelineRecord{}, fmt.Errorf("unknown pipeline version %q", resolvedVersion)
	}

	out := registry.PipelineRecord{
		PipelineVersion:    resolvedVersion,
		GraphDefinitionRef: strings.TrimSpace(record.GraphDefinitionRef),
		ExecutionProfile:   strings.TrimSpace(record.ExecutionProfile),
	}
	if out.ExecutionProfile == "" {
		out.ExecutionProfile = "simple"
	}
	if err := out.Validate(); err != nil {
		return registry.PipelineRecord{}, err
	}
	return out, nil
}

func normalizePipelineRecord(record pipelineRecord) pipelineRecord {
	if record.CreatedAtUTC == "" {
		record.CreatedAtUTC = record.PublishedAtUTC
	}
	if record.Author == "" {
		record.Author = "system"
	}
	if record.SpecHash == "" {
		record.SpecHash = pipelineRecordHash(record)
	}
	return record
}

func writeJSON(w io.Writer, v any) error {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, string(payload))
	return nil
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "rspp-control-plane usage:")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane publish <pipeline_version> <graph_definition_ref> [execution_profile]")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane list")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane get <pipeline_version>")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane rollback <pipeline_version>")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane status")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane audit")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane resolve-session-route <tenant_id> <session_id> [requested_pipeline_version] [transport_kind] [authority_epoch]")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane issue-session-token <tenant_id> <session_id> [requested_pipeline_version] [transport_kind] [authority_epoch] [ttl_seconds]")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane session-status <tenant_id> <session_id>")
	_, _ = fmt.Fprintln(w, "  rspp-control-plane set-session-status <tenant_id> <session_id> <status> [pipeline_version] [authority_epoch] [reason]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Env:")
	_, _ = fmt.Fprintf(w, "  %s (default: %s)\n", envControlPlaneStatePath, defaultControlPlaneState)
	_, _ = fmt.Fprintf(w, "  %s (default: system)\n", envControlPlaneActor)
	_, _ = fmt.Fprintf(w, "  %s (optional token signing secret override)\n", envSessionTokenSecret)
	_, _ = fmt.Fprintf(w, "  %s (optional default token ttl seconds)\n", envSessionTokenTTL)
}
