package controlplane

import "testing"

func TestCT005DecisionOutcomeMappingRules(t *testing.T) {
	t.Parallel()

	base := func() DecisionOutcome {
		return DecisionOutcome{
			OutcomeKind:        OutcomeReject,
			Phase:              PhasePreTurn,
			Scope:              ScopeSession,
			SessionID:          "sess-ct5-1",
			EventID:            "evt-ct5-1",
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			EmittedBy:          EmitterRK25,
			Reason:             "admission_capacity_reject",
		}
	}

	tests := []struct {
		name      string
		mutate    func(*DecisionOutcome)
		shouldErr bool
	}{
		{
			name: "authority outcome valid RK-24",
			mutate: func(out *DecisionOutcome) {
				epoch := int64(8)
				out.OutcomeKind = OutcomeStaleEpochReject
				out.Phase = PhasePreTurn
				out.Scope = ScopeSession
				out.EmittedBy = EmitterRK24
				out.AuthorityEpoch = &epoch
				out.Reason = "authority_epoch_mismatch"
			},
		},
		{
			name: "authority outcome invalid emitter",
			mutate: func(out *DecisionOutcome) {
				epoch := int64(8)
				out.OutcomeKind = OutcomeStaleEpochReject
				out.Phase = PhasePreTurn
				out.Scope = ScopeSession
				out.EmittedBy = EmitterRK25
				out.AuthorityEpoch = &epoch
				out.Reason = "authority_epoch_mismatch"
			},
			shouldErr: true,
		},
		{
			name: "admission outcome invalid authority emitter",
			mutate: func(out *DecisionOutcome) {
				out.OutcomeKind = OutcomeAdmit
				out.Phase = PhasePreTurn
				out.Scope = ScopeSession
				out.EmittedBy = EmitterRK24
				out.Reason = "admission_capacity_allow"
			},
			shouldErr: true,
		},
		{
			name: "shed requires scheduling phase",
			mutate: func(out *DecisionOutcome) {
				out.OutcomeKind = OutcomeShed
				out.Phase = PhasePreTurn
				out.Scope = ScopeNodeDispatch
				out.EmittedBy = EmitterRK25
				out.Reason = "scheduling_point_shed"
			},
			shouldErr: true,
		},
		{
			name: "scope turn requires turn_id",
			mutate: func(out *DecisionOutcome) {
				out.OutcomeKind = OutcomeReject
				out.Phase = PhasePreTurn
				out.Scope = ScopeTurn
				out.EmittedBy = EmitterRK25
				out.TurnID = ""
				out.Reason = "admission_capacity_reject"
			},
			shouldErr: true,
		},
		{
			name: "negative timestamp_ms rejected",
			mutate: func(out *DecisionOutcome) {
				ts := int64(-1)
				out.TimestampMS = &ts
			},
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := base()
			tc.mutate(&out)
			err := out.Validate()
			if tc.shouldErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected valid outcome, got error: %v", err)
			}
		})
	}
}

func TestSyncDropPolicyValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policy    SyncDropPolicy
		shouldErr bool
	}{
		{
			name: "valid atomic_drop with group_by",
			policy: SyncDropPolicy{
				GroupBy: "sync_id",
				Policy:  "atomic_drop",
				Scope:   "edge_local",
			},
		},
		{
			name: "invalid group_by enum",
			policy: SyncDropPolicy{
				GroupBy: "lane",
				Policy:  "atomic_drop",
				Scope:   "edge_local",
			},
			shouldErr: true,
		},
		{
			name: "missing group_by for atomic_drop",
			policy: SyncDropPolicy{
				Policy: "atomic_drop",
				Scope:  "edge_local",
			},
			shouldErr: true,
		},
		{
			name: "drop_with_discontinuity requires sync_domain",
			policy: SyncDropPolicy{
				GroupBy: "event_id",
				Policy:  "drop_with_discontinuity",
				Scope:   "plan_wide",
			},
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.policy.Validate()
			if tc.shouldErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected valid sync_drop_policy, got %v", err)
			}
		})
	}
}

func TestRecordingPolicyValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policy    RecordingPolicy
		shouldErr bool
	}{
		{
			name: "valid single replay mode",
			policy: RecordingPolicy{
				RecordingLevel:     "L0",
				AllowedReplayModes: []string{"replay_decisions"},
			},
		},
		{
			name: "valid multiple replay modes",
			policy: RecordingPolicy{
				RecordingLevel: "L1",
				AllowedReplayModes: []string{
					"replay_decisions",
					"recompute_decisions",
				},
			},
		},
		{
			name: "invalid replay mode",
			policy: RecordingPolicy{
				RecordingLevel:     "L0",
				AllowedReplayModes: []string{"unknown_mode"},
			},
			shouldErr: true,
		},
		{
			name: "duplicate replay mode",
			policy: RecordingPolicy{
				RecordingLevel:     "L2",
				AllowedReplayModes: []string{"replay_decisions", "replay_decisions"},
			},
			shouldErr: true,
		},
		{
			name: "no replay modes",
			policy: RecordingPolicy{
				RecordingLevel:     "L0",
				AllowedReplayModes: nil,
			},
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.policy.Validate()
			if tc.shouldErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected valid recording policy, got %v", err)
			}
		})
	}
}

func TestNodeExecutionPolicyValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policy    NodeExecutionPolicy
		shouldErr bool
	}{
		{
			name:   "valid empty policy",
			policy: NodeExecutionPolicy{},
		},
		{
			name: "valid with fairness key and concurrency",
			policy: NodeExecutionPolicy{
				ConcurrencyLimit: 2,
				FairnessKey:      "provider-heavy",
			},
		},
		{
			name: "invalid negative concurrency",
			policy: NodeExecutionPolicy{
				ConcurrencyLimit: -1,
			},
			shouldErr: true,
		},
		{
			name: "invalid whitespace fairness key",
			policy: NodeExecutionPolicy{
				ConcurrencyLimit: 1,
				FairnessKey:      "   ",
			},
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.policy.Validate()
			if tc.shouldErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected valid node_execution policy, got %v", err)
			}
		})
	}
}

func TestResolvedTurnPlanValidateNodeExecutionPolicies(t *testing.T) {
	t.Parallel()

	base := validResolvedTurnPlanForTest()

	t.Run("valid node execution policies", func(t *testing.T) {
		t.Parallel()
		plan := base
		plan.NodeExecutionPolicies = map[string]NodeExecutionPolicy{
			"provider-stt": {
				ConcurrencyLimit: 1,
				FairnessKey:      "provider-heavy",
			},
		}
		if err := plan.Validate(); err != nil {
			t.Fatalf("expected valid resolved turn plan with node execution policies, got %v", err)
		}
	})

	t.Run("invalid empty node policy key", func(t *testing.T) {
		t.Parallel()
		plan := base
		plan.NodeExecutionPolicies = map[string]NodeExecutionPolicy{
			"": {
				ConcurrencyLimit: 1,
			},
		}
		if err := plan.Validate(); err == nil {
			t.Fatalf("expected node execution policy key validation error")
		}
	})

	t.Run("invalid node policy value", func(t *testing.T) {
		t.Parallel()
		plan := base
		plan.NodeExecutionPolicies = map[string]NodeExecutionPolicy{
			"provider-stt": {
				ConcurrencyLimit: -1,
			},
		}
		if err := plan.Validate(); err == nil {
			t.Fatalf("expected node execution policy value validation error")
		}
	})
}

func TestSessionRouteValidate(t *testing.T) {
	t.Parallel()

	route := SessionRoute{
		TenantID:                "tenant-1",
		SessionID:               "sess-1",
		PipelineVersion:         "pipeline-v1",
		RoutingViewSnapshot:     "routing-view/v1",
		AdmissionPolicySnapshot: "admission-policy/v1",
		Endpoint: TransportEndpointRef{
			TransportKind: "livekit",
			Endpoint:      "wss://runtime.rspp.local/livekit/pipeline-v1",
		},
		Lease: PlacementLease{
			AuthorityEpoch: 3,
			Granted:        true,
			Valid:          true,
			TokenRef: LeaseTokenRef{
				TokenID:      "lease-token-1",
				ExpiresAtUTC: "2026-02-17T00:30:00Z",
			},
		},
	}
	if err := route.Validate(); err != nil {
		t.Fatalf("expected valid session route, got %v", err)
	}

	route.Endpoint.Endpoint = ""
	if err := route.Validate(); err == nil {
		t.Fatalf("expected endpoint validation error")
	}
}

func TestSignedSessionTokenValidate(t *testing.T) {
	t.Parallel()

	token := SignedSessionToken{
		Token:        "payload.signature",
		TokenID:      "token-1",
		ExpiresAtUTC: "2026-02-17T00:30:00Z",
		Claims: SessionTokenClaims{
			TenantID:        "tenant-1",
			SessionID:       "sess-1",
			PipelineVersion: "pipeline-v1",
			TransportKind:   "livekit",
			AuthorityEpoch:  4,
			IssuedAtUTC:     "2026-02-17T00:25:00Z",
			ExpiresAtUTC:    "2026-02-17T00:30:00Z",
		},
	}
	if err := token.Validate(); err != nil {
		t.Fatalf("expected valid signed session token, got %v", err)
	}

	token.Claims.ExpiresAtUTC = "2026-02-17T00:20:00Z"
	if err := token.Validate(); err == nil {
		t.Fatalf("expected claims expiry validation error")
	}
}

func TestSessionStatusViewValidate(t *testing.T) {
	t.Parallel()

	view := SessionStatusView{
		TenantID:        "tenant-1",
		SessionID:       "sess-1",
		Status:          SessionStatusRunning,
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  5,
		UpdatedAtUTC:    "2026-02-17T00:25:00Z",
	}
	if err := view.Validate(); err != nil {
		t.Fatalf("expected valid session status view, got %v", err)
	}

	view.Status = SessionStatus("unknown")
	if err := view.Validate(); err == nil {
		t.Fatalf("expected status validation error")
	}
}

func TestSessionRequestValidation(t *testing.T) {
	t.Parallel()

	if err := (SessionRouteRequest{TenantID: "tenant-1", SessionID: "sess-1"}).Validate(); err != nil {
		t.Fatalf("expected valid session route request, got %v", err)
	}
	if err := (SessionRouteRequest{SessionID: "sess-1"}).Validate(); err == nil {
		t.Fatalf("expected route request tenant validation error")
	}

	if err := (SessionTokenRequest{TenantID: "tenant-1", SessionID: "sess-1", TTLSeconds: 300}).Validate(); err != nil {
		t.Fatalf("expected valid token request, got %v", err)
	}
	if err := (SessionTokenRequest{TenantID: "tenant-1", SessionID: "sess-1", TTLSeconds: -1}).Validate(); err == nil {
		t.Fatalf("expected token request ttl validation error")
	}

	if err := (SessionStatusRequest{TenantID: "tenant-1", SessionID: "sess-1"}).Validate(); err != nil {
		t.Fatalf("expected valid status request, got %v", err)
	}
	if err := (SessionStatusRequest{TenantID: "tenant-1"}).Validate(); err == nil {
		t.Fatalf("expected status request session validation error")
	}
}

func validResolvedTurnPlanForTest() ResolvedTurnPlan {
	return ResolvedTurnPlan{
		TurnID:             "turn-types-test-1",
		PipelineVersion:    "pipeline-v1",
		PlanHash:           "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GraphDefinitionRef: "graph/default",
		ExecutionProfile:   "simple",
		AuthorityEpoch:     1,
		Budgets: Budgets{
			TurnBudgetMS:        5000,
			NodeBudgetMSDefault: 1500,
			PathBudgetMSDefault: 3000,
			EdgeBudgetMSDefault: 500,
		},
		ProviderBindings: map[string]string{
			"stt": "stt-default",
			"llm": "llm-default",
			"tts": "tts-default",
		},
		EdgeBufferPolicies: map[string]EdgeBufferPolicy{
			"default": {
				Strategy:                 BufferStrategyDrop,
				MaxQueueItems:            64,
				MaxQueueMS:               300,
				MaxQueueBytes:            262144,
				MaxLatencyContributionMS: 120,
				Watermarks: EdgeWatermarks{
					QueueItems: &WatermarkThreshold{High: 48, Low: 24},
				},
				LaneHandling: LaneHandling{
					DataLane:      "drop",
					ControlLane:   "non_blocking_priority",
					TelemetryLane: "best_effort_drop",
				},
				DefaultingSource: "execution_profile_default",
			},
		},
		FlowControl: FlowControl{
			ModeByLane: ModeByLane{
				DataLane:      "signal",
				ControlLane:   "signal",
				TelemetryLane: "signal",
			},
			Watermarks: FlowWatermarks{
				DataLane:      WatermarkThreshold{High: 100, Low: 50},
				ControlLane:   WatermarkThreshold{High: 20, Low: 10},
				TelemetryLane: WatermarkThreshold{High: 200, Low: 100},
			},
			SheddingStrategyByLane: SheddingStrategyByLane{
				DataLane:      "drop",
				ControlLane:   "none",
				TelemetryLane: "sample",
			},
		},
		AllowedAdaptiveActions: []string{"retry"},
		SnapshotProvenance: SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		RecordingPolicy: RecordingPolicy{
			RecordingLevel:     "L0",
			AllowedReplayModes: []string{"replay_decisions"},
		},
		Determinism: Determinism{
			Seed:                   1,
			OrderingMarkers:        []string{"runtime_sequence:1"},
			MergeRuleID:            "merge/default",
			MergeRuleVersion:       "v1.0",
			NondeterministicInputs: []NondeterministicInput{},
		},
	}
}
