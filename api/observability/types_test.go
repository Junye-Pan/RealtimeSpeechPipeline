package observability

import (
	"strings"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestReplayAccessRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*ReplayAccessRequest)
		wantError string
	}{
		{
			name: "valid request",
		},
		{
			name: "invalid requested fidelity",
			mutate: func(req *ReplayAccessRequest) {
				req.RequestedFidelity = "L9"
			},
			wantError: "invalid requested_fidelity",
		},
		{
			name: "missing required field",
			mutate: func(req *ReplayAccessRequest) {
				req.PrincipalID = ""
			},
			wantError: "tenant_id, principal_id, role, purpose, and requested_scope are required",
		},
		{
			name: "invalid payload class",
			mutate: func(req *ReplayAccessRequest) {
				req.RequestedClasses = []eventabi.PayloadClass{"invalid"}
			},
			wantError: "invalid payload_class",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := validReplayAccessRequest()
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			err := req.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid request, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func TestReplayAccessDecisionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		decision  ReplayAccessDecision
		wantError string
	}{
		{
			name: "allow with redaction marker",
			decision: ReplayAccessDecision{
				Allowed: true,
				RedactionMarkers: []ReplayRedactionMarker{{
					PayloadClass: eventabi.PayloadTextRaw,
					Action:       eventabi.RedactionMask,
					Reason:       "policy",
				}},
			},
		},
		{
			name: "deny with reason",
			decision: ReplayAccessDecision{
				Allowed:    false,
				DenyReason: "role_not_authorized",
			},
		},
		{
			name: "deny missing reason",
			decision: ReplayAccessDecision{
				Allowed: false,
			},
			wantError: "deny_reason is required",
		},
		{
			name: "allow with deny reason",
			decision: ReplayAccessDecision{
				Allowed:    true,
				DenyReason: "unexpected",
			},
			wantError: "deny_reason must be empty",
		},
		{
			name: "invalid redaction marker action",
			decision: ReplayAccessDecision{
				Allowed: true,
				RedactionMarkers: []ReplayRedactionMarker{{
					PayloadClass: eventabi.PayloadTextRaw,
					Action:       eventabi.RedactionAction("invalid"),
					Reason:       "policy",
				}},
			},
			wantError: "invalid redaction action",
		},
		{
			name: "missing redaction marker reason",
			decision: ReplayAccessDecision{
				Allowed: true,
				RedactionMarkers: []ReplayRedactionMarker{{
					PayloadClass: eventabi.PayloadTextRaw,
					Action:       eventabi.RedactionMask,
				}},
			},
			wantError: "redaction marker reason is required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.decision.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid decision, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func TestReplayAuditEventValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*ReplayAuditEvent)
		wantError string
	}{
		{
			name: "valid event",
		},
		{
			name: "negative timestamp",
			mutate: func(event *ReplayAuditEvent) {
				event.TimestampMS = -1
			},
			wantError: "timestamp_ms must be >=0",
		},
		{
			name: "invalid nested decision",
			mutate: func(event *ReplayAuditEvent) {
				event.Decision = ReplayAccessDecision{Allowed: false}
			},
			wantError: "deny_reason is required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			event := ReplayAuditEvent{
				TimestampMS: 123,
				Request:     validReplayAccessRequest(),
				Decision: ReplayAccessDecision{
					Allowed:    false,
					DenyReason: "missing_role_scope",
					RedactionMarkers: []ReplayRedactionMarker{
						{
							PayloadClass: eventabi.PayloadTextRaw,
							Action:       eventabi.RedactionMask,
							Reason:       "default_or03_policy",
						},
					},
				},
			}
			if tc.mutate != nil {
				tc.mutate(&event)
			}

			err := event.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid event, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func TestReplayDivergenceValidate(t *testing.T) {
	t.Parallel()

	diff := int64(17)
	tests := []struct {
		name       string
		divergence ReplayDivergence
		wantError  string
	}{
		{
			name: "valid non timing divergence",
			divergence: ReplayDivergence{
				Class:   OutcomeDivergence,
				Scope:   "turn:t-1",
				Message: "output mismatch",
			},
		},
		{
			name: "valid provider choice divergence",
			divergence: ReplayDivergence{
				Class:   ProviderChoiceDivergence,
				Scope:   "turn:t-1",
				Message: "provider mismatch",
			},
		},
		{
			name: "valid timing divergence",
			divergence: ReplayDivergence{
				Class:   TimingDivergence,
				Scope:   "turn:t-1",
				Message: "timing mismatch",
				DiffMS:  &diff,
			},
		},
		{
			name: "invalid class",
			divergence: ReplayDivergence{
				Class:   DivergenceClass("invalid"),
				Scope:   "turn:t-1",
				Message: "mismatch",
			},
			wantError: "invalid divergence class",
		},
		{
			name: "missing scope",
			divergence: ReplayDivergence{
				Class:   OutcomeDivergence,
				Message: "mismatch",
			},
			wantError: "scope and message are required",
		},
		{
			name: "timing requires diff",
			divergence: ReplayDivergence{
				Class:   TimingDivergence,
				Scope:   "turn:t-1",
				Message: "timing mismatch",
			},
			wantError: "diff_ms is required",
		},
		{
			name: "diff must be non negative",
			divergence: ReplayDivergence{
				Class:   TimingDivergence,
				Scope:   "turn:t-1",
				Message: "timing mismatch",
				DiffMS:  ptrInt64(-1),
			},
			wantError: "diff_ms must be >=0",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.divergence.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid divergence, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func TestReplayModeValidate(t *testing.T) {
	t.Parallel()

	valid := []ReplayMode{
		ReplayModeReSimulateNodes,
		ReplayModePlaybackRecordedProvider,
		ReplayModeReplayDecisions,
		ReplayModeRecomputeDecisions,
	}
	for _, mode := range valid {
		mode := mode
		t.Run("valid_"+string(mode), func(t *testing.T) {
			t.Parallel()
			if err := mode.Validate(); err != nil {
				t.Fatalf("expected valid mode %s, got %v", mode, err)
			}
			if !IsReplayMode(string(mode)) {
				t.Fatalf("expected IsReplayMode true for %s", mode)
			}
		})
	}

	if err := ReplayMode("invalid").Validate(); err == nil {
		t.Fatalf("expected invalid replay mode to fail validation")
	}
	if IsReplayMode("invalid") {
		t.Fatalf("expected IsReplayMode false for unknown mode")
	}
}

func TestReplayCursorValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cursor    ReplayCursor
		wantError string
	}{
		{
			name:   "valid cursor",
			cursor: validReplayCursor(),
		},
		{
			name: "invalid lane",
			cursor: ReplayCursor{
				SessionID:       "sess-1",
				TurnID:          "turn-1",
				Lane:            eventabi.Lane("invalid"),
				RuntimeSequence: 14,
				EventID:         "evt-1",
			},
			wantError: "invalid lane",
		},
		{
			name: "missing ids",
			cursor: ReplayCursor{
				SessionID:       "",
				TurnID:          "turn-1",
				Lane:            eventabi.LaneData,
				RuntimeSequence: 14,
				EventID:         "evt-1",
			},
			wantError: "session_id, turn_id, and event_id are required",
		},
		{
			name: "negative runtime sequence",
			cursor: ReplayCursor{
				SessionID:       "sess-1",
				TurnID:          "turn-1",
				Lane:            eventabi.LaneData,
				RuntimeSequence: -1,
				EventID:         "evt-1",
			},
			wantError: "runtime_sequence must be >=0",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cursor.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid cursor, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func TestReplayRunRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*ReplayRunRequest)
		wantError string
	}{
		{
			name: "valid request",
		},
		{
			name: "invalid mode",
			mutate: func(req *ReplayRunRequest) {
				req.Mode = ReplayMode("invalid")
			},
			wantError: "invalid replay mode",
		},
		{
			name: "missing baseline ref",
			mutate: func(req *ReplayRunRequest) {
				req.BaselineRef = ""
			},
			wantError: "baseline_ref and candidate_plan_ref are required",
		},
		{
			name: "invalid cursor",
			mutate: func(req *ReplayRunRequest) {
				req.Cursor = &ReplayCursor{
					SessionID:       "sess-1",
					TurnID:          "turn-1",
					Lane:            eventabi.LaneData,
					RuntimeSequence: -1,
					EventID:         "evt-1",
				}
			},
			wantError: "runtime_sequence must be >=0",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := ReplayRunRequest{
				BaselineRef:      "artifact://baseline/1",
				CandidatePlanRef: "pipeline://v2",
				Mode:             ReplayModeReplayDecisions,
				Cursor:           ptrReplayCursor(validReplayCursor()),
			}
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			err := req.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid request, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func TestReplayRunResultValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*ReplayRunResult)
		wantError string
	}{
		{
			name: "valid result",
		},
		{
			name: "missing run id",
			mutate: func(result *ReplayRunResult) {
				result.RunID = ""
			},
			wantError: "run_id is required",
		},
		{
			name: "invalid mode",
			mutate: func(result *ReplayRunResult) {
				result.Mode = ReplayMode("invalid")
			},
			wantError: "invalid replay mode",
		},
		{
			name: "invalid divergence",
			mutate: func(result *ReplayRunResult) {
				result.Divergences = []ReplayDivergence{{Class: DivergenceClass("invalid"), Scope: "turn:t-1", Message: "bad"}}
			},
			wantError: "invalid divergence class",
		},
		{
			name: "invalid cursor",
			mutate: func(result *ReplayRunResult) {
				result.Cursor = &ReplayCursor{
					SessionID:       "sess-1",
					TurnID:          "turn-1",
					Lane:            eventabi.LaneData,
					RuntimeSequence: -1,
					EventID:         "evt-1",
				}
			},
			wantError: "runtime_sequence must be >=0",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := ReplayRunResult{
				RunID: "run-1",
				Mode:  ReplayModeReplayDecisions,
				Divergences: []ReplayDivergence{{
					Class:   OutcomeDivergence,
					Scope:   "turn:t-1",
					Message: "decision mismatch",
				}},
				Cursor: ptrReplayCursor(validReplayCursor()),
			}
			if tc.mutate != nil {
				tc.mutate(&result)
			}
			err := result.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid result, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func TestReplayRunReportValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*ReplayRunReport)
		wantError string
	}{
		{
			name: "valid report",
		},
		{
			name: "negative generated timestamp",
			mutate: func(report *ReplayRunReport) {
				report.GeneratedAtMS = -1
			},
			wantError: "generated_at_ms must be >=0",
		},
		{
			name: "request and result mode mismatch",
			mutate: func(report *ReplayRunReport) {
				report.Result.Mode = ReplayModeRecomputeDecisions
			},
			wantError: "request mode and result mode must match",
		},
		{
			name: "invalid request",
			mutate: func(report *ReplayRunReport) {
				report.Request.BaselineRef = ""
			},
			wantError: "invalid request",
		},
		{
			name: "invalid result",
			mutate: func(report *ReplayRunReport) {
				report.Result.RunID = ""
			},
			wantError: "invalid result",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			report := ReplayRunReport{
				Request: ReplayRunRequest{
					BaselineRef:      "artifact://baseline/1",
					CandidatePlanRef: "pipeline://v2",
					Mode:             ReplayModeReplayDecisions,
					Cursor:           ptrReplayCursor(validReplayCursor()),
				},
				Result: ReplayRunResult{
					RunID: "run-1",
					Mode:  ReplayModeReplayDecisions,
					Divergences: []ReplayDivergence{{
						Class:   OutcomeDivergence,
						Scope:   "turn:t-1",
						Message: "decision mismatch",
					}},
					Cursor: ptrReplayCursor(validReplayCursor()),
				},
				GeneratedAtMS: 555,
			}
			if tc.mutate != nil {
				tc.mutate(&report)
			}
			err := report.Validate()
			if tc.wantError == "" && err != nil {
				t.Fatalf("expected valid report, got %v", err)
			}
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			}
		})
	}
}

func validReplayAccessRequest() ReplayAccessRequest {
	return ReplayAccessRequest{
		TenantID:          "tenant-a",
		PrincipalID:       "user-1",
		Role:              "oncall",
		Purpose:           "incident",
		RequestedScope:    "turn:123",
		RequestedFidelity: ReplayFidelityL0,
		RequestedClasses:  []eventabi.PayloadClass{eventabi.PayloadMetadata},
	}
}

func validReplayCursor() ReplayCursor {
	return ReplayCursor{
		SessionID:       "sess-1",
		TurnID:          "turn-1",
		Lane:            eventabi.LaneData,
		RuntimeSequence: 14,
		EventID:         "evt-1",
	}
}

func ptrReplayCursor(cursor ReplayCursor) *ReplayCursor {
	return &cursor
}

func ptrInt64(v int64) *int64 {
	return &v
}
