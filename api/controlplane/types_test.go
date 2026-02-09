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
