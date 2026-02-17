package eventabi

import "testing"

func TestCT002ControlSignalEmitterMappingAndUnknownSignal(t *testing.T) {
	t.Parallel()

	base := func() ControlSignal {
		transport := int64(1)
		return ControlSignal{
			SchemaVersion:      "v1.0",
			EventScope:         ScopeTurn,
			SessionID:          "sess-ct2-1",
			TurnID:             "turn-ct2-1",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-ct2-1",
			Lane:               LaneControl,
			TransportSequence:  &transport,
			RuntimeSequence:    2,
			AuthorityEpoch:     3,
			RuntimeTimestampMS: 100,
			WallClockMS:        100,
			PayloadClass:       PayloadMetadata,
			Signal:             "turn_open",
			EmittedBy:          "RK-03",
		}
	}

	tests := []struct {
		name      string
		mutate    func(*ControlSignal)
		shouldErr bool
	}{
		{
			name: "turn_open valid emitter",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "turn_open"
				sig.EmittedBy = "RK-03"
			},
		},
		{
			name: "turn_open invalid emitter",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "turn_open"
				sig.EmittedBy = "RK-24"
			},
			shouldErr: true,
		},
		{
			name: "admit valid RK-25",
			mutate: func(sig *ControlSignal) {
				sig.EventScope = ScopeSession
				sig.TurnID = ""
				sig.Signal = "admit"
				sig.EmittedBy = "RK-25"
				sig.Reason = "admission_ok"
			},
		},
		{
			name: "admit valid CP-05",
			mutate: func(sig *ControlSignal) {
				sig.EventScope = ScopeSession
				sig.TurnID = ""
				sig.Signal = "admit"
				sig.EmittedBy = "CP-05"
				sig.Reason = "admission_ok"
			},
		},
		{
			name: "admit invalid emitter",
			mutate: func(sig *ControlSignal) {
				sig.EventScope = ScopeSession
				sig.TurnID = ""
				sig.Signal = "admit"
				sig.EmittedBy = "RK-24"
				sig.Reason = "admission_ok"
			},
			shouldErr: true,
		},
		{
			name: "drop_notice requires emitter and range context",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "drop_notice"
				sig.EmittedBy = "RK-12"
				sig.EdgeID = "edge-audio"
				sig.TargetLane = LaneData
				sig.Reason = "queue_pressure_drop"
				sig.SeqRange = &SeqRange{Start: 10, End: 20}
			},
		},
		{
			name: "unknown signal rejected",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "unknown_signal"
			},
			shouldErr: true,
		},
		{
			name: "turn_open_proposed valid session emitter",
			mutate: func(sig *ControlSignal) {
				sig.EventScope = ScopeSession
				sig.TurnID = ""
				sig.Signal = "turn_open_proposed"
				sig.EmittedBy = "RK-02"
			},
		},
		{
			name: "turn_open_proposed invalid emitter",
			mutate: func(sig *ControlSignal) {
				sig.EventScope = ScopeSession
				sig.TurnID = ""
				sig.Signal = "turn_open_proposed"
				sig.EmittedBy = "RK-03"
			},
			shouldErr: true,
		},
		{
			name: "barge_in valid emitter",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "barge_in"
				sig.EmittedBy = "RK-06"
			},
		},
		{
			name: "barge_in invalid emitter",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "barge_in"
				sig.EmittedBy = "RK-03"
			},
			shouldErr: true,
		},
		{
			name: "cancel valid scope and emitter",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "cancel"
				sig.Scope = "provider_invocation"
				sig.EmittedBy = "RK-16"
			},
		},
		{
			name: "cancel invalid scope",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "cancel"
				sig.Scope = "edge"
				sig.EmittedBy = "RK-16"
			},
			shouldErr: true,
		},
		{
			name: "budget_warning invalid emitter",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "budget_warning"
				sig.EmittedBy = "RK-13"
				sig.Reason = "budget_threshold_warning"
			},
			shouldErr: true,
		},
		{
			name: "degrade valid emitter with reason",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "degrade"
				sig.EmittedBy = "RK-13"
				sig.Reason = "degrade_due_to_pressure"
			},
		},
		{
			name: "fallback invalid missing reason",
			mutate: func(sig *ControlSignal) {
				sig.Signal = "fallback"
				sig.EmittedBy = "RK-17"
				sig.Reason = ""
			},
			shouldErr: true,
		},
		{
			name: "timestamp_ms negative rejected",
			mutate: func(sig *ControlSignal) {
				ts := int64(-1)
				sig.TimestampMS = &ts
			},
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sig := base()
			tc.mutate(&sig)
			err := sig.Validate()
			if tc.shouldErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected valid signal, got error: %v", err)
			}
		})
	}
}

func TestEventRecordValidateTimestampMS(t *testing.T) {
	t.Parallel()

	transport := int64(1)
	authority := int64(3)
	event := EventRecord{
		SchemaVersion:      "v1.0",
		EventScope:         ScopeTurn,
		SessionID:          "sess-ts-1",
		TurnID:             "turn-ts-1",
		PipelineVersion:    "pipeline-v1",
		EventID:            "evt-ts-1",
		Lane:               LaneData,
		TransportSequence:  &transport,
		RuntimeSequence:    3,
		AuthorityEpoch:     &authority,
		RuntimeTimestampMS: 100,
		WallClockMS:        100,
		PayloadClass:       PayloadMetadata,
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid event record, got %v", err)
	}

	ts := int64(-1)
	event.TimestampMS = &ts
	if err := event.Validate(); err == nil {
		t.Fatalf("expected negative timestamp_ms to fail validation")
	}
}

func TestRedactionDecisionValidate(t *testing.T) {
	t.Parallel()

	valid := RedactionDecision{PayloadClass: PayloadPII, Action: RedactionHash}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid redaction decision, got %v", err)
	}

	invalidAction := valid
	invalidAction.Action = "noop"
	if err := invalidAction.Validate(); err == nil {
		t.Fatalf("expected invalid redaction action to fail validation")
	}
}
