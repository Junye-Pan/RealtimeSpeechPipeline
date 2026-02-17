package timeline

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	timelineexporter "github.com/tiger/realtime-speech-pipeline/internal/observability/timeline/exporter"
)

func TestAppendBaselineStageACapacity(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{BaselineCapacity: 1, DetailCapacity: 4})
	if err := recorder.AppendBaseline(minimalBaseline("turn-a")); err != nil {
		t.Fatalf("unexpected first append error: %v", err)
	}
	if err := recorder.AppendBaseline(minimalBaseline("turn-b")); !errors.Is(err, ErrBaselineCapacityExhausted) {
		t.Fatalf("expected baseline capacity exhaustion, got %v", err)
	}
}

func TestAppendBaselineEnqueuesDurableRecord(t *testing.T) {
	t.Parallel()

	durable := &capturingDurableExporter{enqueueResult: true}
	recorder := NewRecorderWithDurableExporter(StageAConfig{BaselineCapacity: 2, DetailCapacity: 4}, durable)

	if err := recorder.AppendBaseline(minimalBaseline("turn-durable-1")); err != nil {
		t.Fatalf("append baseline: %v", err)
	}

	records := durable.Records()
	if len(records) != 1 {
		t.Fatalf("expected one durable record, got %+v", records)
	}
	if records[0].Kind != "baseline" || records[0].SessionID != "sess-1" || records[0].TurnID != "turn-durable-1" {
		t.Fatalf("unexpected durable record identity: %+v", records[0])
	}
	if len(records[0].Payload) == 0 {
		t.Fatalf("expected serialized baseline payload")
	}

	var decoded BaselineEvidence
	if err := json.Unmarshal(records[0].Payload, &decoded); err != nil {
		t.Fatalf("decode durable baseline payload: %v", err)
	}
	if decoded.TurnID != "turn-durable-1" || decoded.EventID != "evt-1" {
		t.Fatalf("unexpected decoded durable baseline payload: %+v", decoded)
	}
}

func TestAppendBaselineIgnoresDurableExporterBackpressure(t *testing.T) {
	t.Parallel()

	durable := &capturingDurableExporter{enqueueResult: false}
	recorder := NewRecorderWithDurableExporter(StageAConfig{BaselineCapacity: 2, DetailCapacity: 4}, durable)

	if err := recorder.AppendBaseline(minimalBaseline("turn-durable-drop-1")); err != nil {
		t.Fatalf("append baseline with durable backpressure: %v", err)
	}

	if entries := recorder.BaselineEntries(); len(entries) != 1 {
		t.Fatalf("expected local baseline append to succeed even when durable enqueue drops, got %+v", entries)
	}
}

func TestAppendDetailOverflowEmitsDowngradeOncePerTurn(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{BaselineCapacity: 4, DetailCapacity: 1})
	first, err := recorder.AppendDetail(DetailEvent{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	})
	if err != nil {
		t.Fatalf("unexpected first detail append error: %v", err)
	}
	if !first.Stored || first.Dropped {
		t.Fatalf("expected first detail append to store")
	}

	second, err := recorder.AppendDetail(DetailEvent{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-2",
		TransportSequence:    2,
		RuntimeSequence:      2,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   101,
		WallClockTimestampMS: 101,
	})
	if err != nil {
		t.Fatalf("unexpected overflow append error: %v", err)
	}
	if !second.Dropped || !second.DowngradeEmitted || second.DowngradeSignal == nil {
		t.Fatalf("expected deterministic overflow downgrade signal, got %+v", second)
	}
	if second.DowngradeSignal.Signal != "recording_level_downgraded" || second.DowngradeSignal.EmittedBy != "OR-02" {
		t.Fatalf("unexpected downgrade signal: %+v", second.DowngradeSignal)
	}

	third, err := recorder.AppendDetail(DetailEvent{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-3",
		TransportSequence:    3,
		RuntimeSequence:      3,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   102,
		WallClockTimestampMS: 102,
	})
	if err != nil {
		t.Fatalf("unexpected repeated overflow append error: %v", err)
	}
	if !third.Dropped || third.DowngradeEmitted {
		t.Fatalf("expected repeated overflow to drop without duplicate downgrade marker, got %+v", third)
	}
}

func TestBaselineCompletenessReport(t *testing.T) {
	t.Parallel()

	complete := minimalBaseline("turn-complete")
	incomplete := minimalBaseline("turn-incomplete")
	incomplete.OrderingMarkers = nil

	report := BaselineCompleteness([]BaselineEvidence{complete, incomplete})
	if report.TotalAcceptedTurns != 2 {
		t.Fatalf("expected 2 accepted turns, got %d", report.TotalAcceptedTurns)
	}
	if report.CompleteAcceptedTurns != 1 {
		t.Fatalf("expected 1 complete accepted turn, got %d", report.CompleteAcceptedTurns)
	}
	if report.CompletenessRatio != 0.5 {
		t.Fatalf("expected completeness ratio 0.5, got %.2f", report.CompletenessRatio)
	}
	if len(report.IncompleteAcceptedTurnIDs) != 1 || report.IncompleteAcceptedTurnIDs[0] != "turn-incomplete" {
		t.Fatalf("unexpected incomplete turn ids: %+v", report.IncompleteAcceptedTurnIDs)
	}
}

func TestValidateCompletenessCancelMarkers(t *testing.T) {
	t.Parallel()

	cancelSent := int64(130)
	cancelAccepted := int64(132)
	cancelFence := int64(140)
	baseline := minimalBaseline("turn-cancel")
	baseline.CancelSentAtMS = &cancelSent
	baseline.CancelAcceptedAtMS = &cancelAccepted
	baseline.CancelFenceAppliedAtMS = &cancelFence
	baseline.TerminalOutcome = "abort"
	baseline.TerminalReason = "cancelled"
	if err := baseline.ValidateCompleteness(); err != nil {
		t.Fatalf("expected cancel baseline to validate: %v", err)
	}

	missingCancelSent := baseline
	missingCancelSent.CancelSentAtMS = nil
	if err := missingCancelSent.ValidateCompleteness(); err == nil {
		t.Fatalf("expected missing cancel_sent_at to fail completeness")
	}

	missingFence := baseline
	missingFence.CancelFenceAppliedAtMS = nil
	if err := missingFence.ValidateCompleteness(); err == nil {
		t.Fatalf("expected missing cancel fence marker to fail completeness")
	}
}

func TestValidateCompletenessInvocationOutcomeEvidence(t *testing.T) {
	t.Parallel()

	baseline := minimalBaseline("turn-provider-evidence")
	baseline.InvocationOutcomes = []InvocationOutcomeEvidence{
		{
			ProviderInvocationID:     "pvi-1",
			Modality:                 "stt",
			ProviderID:               "stt-a",
			OutcomeClass:             "success",
			Retryable:                false,
			RetryDecision:            "none",
			AttemptCount:             1,
			FinalAttemptLatencyMS:    0,
			TotalInvocationLatencyMS: 0,
		},
	}

	if err := baseline.ValidateCompleteness(); err != nil {
		t.Fatalf("expected valid invocation evidence, got %v", err)
	}

	invalid := baseline
	invalid.InvocationOutcomes = []InvocationOutcomeEvidence{
		{
			ProviderInvocationID:     "pvi-2",
			Modality:                 "stt",
			ProviderID:               "stt-a",
			OutcomeClass:             "unknown",
			RetryDecision:            "none",
			AttemptCount:             1,
			FinalAttemptLatencyMS:    0,
			TotalInvocationLatencyMS: 0,
		},
	}
	if err := invalid.ValidateCompleteness(); err == nil {
		t.Fatalf("expected invalid invocation outcome class to fail completeness")
	}

	invalidLatency := baseline
	invalidLatency.InvocationOutcomes = []InvocationOutcomeEvidence{
		{
			ProviderInvocationID:     "pvi-3",
			Modality:                 "stt",
			ProviderID:               "stt-a",
			OutcomeClass:             "success",
			RetryDecision:            "none",
			AttemptCount:             1,
			FinalAttemptLatencyMS:    -1,
			TotalInvocationLatencyMS: 0,
		},
	}
	if err := invalidLatency.ValidateCompleteness(); err == nil {
		t.Fatalf("expected negative invocation latency evidence to fail completeness")
	}
}

func TestAppendProviderInvocationAttempts(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{BaselineCapacity: 4, DetailCapacity: 4, AttemptCapacity: 2})
	attempts := []ProviderAttemptEvidence{
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			ProviderInvocationID: "pvi-1",
			Modality:             "stt",
			ProviderID:           "stt-a",
			Attempt:              1,
			OutcomeClass:         "overload",
			Retryable:            true,
			RetryDecision:        "provider_switch",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       2,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			ProviderInvocationID: "pvi-1",
			Modality:             "stt",
			ProviderID:           "stt-b",
			Attempt:              1,
			OutcomeClass:         "success",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    2,
			RuntimeSequence:      2,
			AuthorityEpoch:       2,
			RuntimeTimestampMS:   101,
			WallClockTimestampMS: 101,
		},
	}

	if err := recorder.AppendProviderInvocationAttempts(attempts); err != nil {
		t.Fatalf("unexpected append attempts error: %v", err)
	}

	stored := recorder.ProviderAttemptEntries()
	if len(stored) != 2 {
		t.Fatalf("expected 2 provider attempts, got %d", len(stored))
	}
	if stored[0].ProviderID != "stt-a" || stored[1].ProviderID != "stt-b" {
		t.Fatalf("unexpected stored provider attempt ordering: %+v", stored)
	}

	if err := recorder.AppendProviderInvocationAttempts([]ProviderAttemptEvidence{attempts[0]}); !errors.Is(err, ErrProviderAttemptCapacityExhausted) {
		t.Fatalf("expected provider attempt capacity exhaustion, got %v", err)
	}
}

func TestProviderAttemptEvidenceValidate(t *testing.T) {
	t.Parallel()

	valid := ProviderAttemptEvidence{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		Modality:             "llm",
		ProviderID:           "llm-a",
		Attempt:              1,
		OutcomeClass:         "success",
		RetryDecision:        "none",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid provider attempt evidence, got %v", err)
	}

	invalid := valid
	invalid.OutcomeClass = "unknown"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected invalid provider attempt outcome class to fail")
	}

	invalidLatency := valid
	invalidLatency.AttemptLatencyMS = -1
	if err := invalidLatency.Validate(); err == nil {
		t.Fatalf("expected negative provider attempt latency to fail")
	}
}

func TestProviderAttemptEntriesForTurn(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{BaselineCapacity: 4, DetailCapacity: 4, AttemptCapacity: 8})
	attempts := []ProviderAttemptEvidence{
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			ProviderInvocationID: "pvi-1",
			Modality:             "stt",
			ProviderID:           "stt-a",
			Attempt:              1,
			OutcomeClass:         "success",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       2,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		{
			SessionID:            "sess-1",
			TurnID:               "turn-2",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-2",
			ProviderInvocationID: "pvi-2",
			Modality:             "stt",
			ProviderID:           "stt-b",
			Attempt:              1,
			OutcomeClass:         "success",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    2,
			RuntimeSequence:      2,
			AuthorityEpoch:       2,
			RuntimeTimestampMS:   101,
			WallClockTimestampMS: 101,
		},
	}
	if err := recorder.AppendProviderInvocationAttempts(attempts); err != nil {
		t.Fatalf("append attempts: %v", err)
	}

	filtered := recorder.ProviderAttemptEntriesForTurn("sess-1", "turn-1")
	if len(filtered) != 1 {
		t.Fatalf("expected one filtered attempt, got %d", len(filtered))
	}
	if filtered[0].ProviderInvocationID != "pvi-1" {
		t.Fatalf("unexpected filtered attempt: %+v", filtered[0])
	}
}

func TestAppendInvocationSnapshotDisabledByDefault(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{BaselineCapacity: 4, DetailCapacity: 4, AttemptCapacity: 4})
	if err := recorder.AppendInvocationSnapshot(minimalInvocationSnapshot("evt-snapshot-disabled-1")); err != nil {
		t.Fatalf("expected disabled invocation snapshot append to no-op, got %v", err)
	}
	if got := recorder.InvocationSnapshotEntries(); len(got) != 0 {
		t.Fatalf("expected no stored snapshots when disabled, got %+v", got)
	}
}

func TestAppendInvocationSnapshotEnabledCapacityAndFilter(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{
		BaselineCapacity:         4,
		DetailCapacity:           4,
		AttemptCapacity:          4,
		InvocationSnapshotCap:    1,
		EnableInvocationSnapshot: true,
	})

	first := minimalInvocationSnapshot("evt-snapshot-enabled-1")
	first.TurnID = "turn-snapshot-1"
	if err := recorder.AppendInvocationSnapshot(first); err != nil {
		t.Fatalf("unexpected first snapshot append error: %v", err)
	}

	second := minimalInvocationSnapshot("evt-snapshot-enabled-2")
	second.TurnID = "turn-snapshot-2"
	if err := recorder.AppendInvocationSnapshot(second); !errors.Is(err, ErrInvocationSnapshotCapacityExhausted) {
		t.Fatalf("expected invocation snapshot capacity exhaustion, got %v", err)
	}

	filtered := recorder.InvocationSnapshotEntriesForTurn("sess-1", "turn-snapshot-1")
	if len(filtered) != 1 || filtered[0].EventID != "evt-snapshot-enabled-1" {
		t.Fatalf("unexpected filtered snapshots: %+v", filtered)
	}
}

func TestInvocationSnapshotEvidenceValidate(t *testing.T) {
	t.Parallel()

	valid := minimalInvocationSnapshot("evt-snapshot-validate-1")
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid invocation snapshot, got %v", err)
	}

	invalid := valid
	invalid.OutcomeClass = "unknown"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected invalid invocation snapshot outcome class to fail")
	}

	invalidTS := valid
	invalidTS.RuntimeTimestampMS = -1
	if err := invalidTS.Validate(); err == nil {
		t.Fatalf("expected negative invocation snapshot timestamp to fail")
	}
}

func TestInvocationOutcomesFromProviderAttempts(t *testing.T) {
	t.Parallel()

	attempts := []ProviderAttemptEvidence{
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			ProviderInvocationID: "pvi-b",
			Modality:             "stt",
			ProviderID:           "stt-a",
			Attempt:              1,
			OutcomeClass:         "overload",
			Retryable:            true,
			RetryDecision:        "provider_switch",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       2,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			ProviderInvocationID: "pvi-b",
			Modality:             "stt",
			ProviderID:           "stt-b",
			Attempt:              1,
			OutcomeClass:         "success",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    2,
			RuntimeSequence:      2,
			AuthorityEpoch:       2,
			RuntimeTimestampMS:   101,
			WallClockTimestampMS: 101,
		},
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-2",
			ProviderInvocationID: "pvi-a",
			Modality:             "llm",
			ProviderID:           "llm-a",
			Attempt:              1,
			OutcomeClass:         "success",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    3,
			RuntimeSequence:      3,
			AuthorityEpoch:       2,
			RuntimeTimestampMS:   102,
			WallClockTimestampMS: 102,
		},
	}

	outcomes, err := InvocationOutcomesFromProviderAttempts(attempts)
	if err != nil {
		t.Fatalf("unexpected synthesis error: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 synthesized outcomes, got %d", len(outcomes))
	}

	if outcomes[0].ProviderInvocationID != "pvi-a" || outcomes[1].ProviderInvocationID != "pvi-b" {
		t.Fatalf("expected deterministic provider invocation ordering, got %+v", outcomes)
	}
	if outcomes[1].ProviderID != "stt-b" || outcomes[1].OutcomeClass != "success" || outcomes[1].AttemptCount != 2 {
		t.Fatalf("expected final-attempt synthesis for pvi-b, got %+v", outcomes[1])
	}
	if outcomes[0].FinalAttemptLatencyMS != 0 || outcomes[0].TotalInvocationLatencyMS != 0 {
		t.Fatalf("expected single-attempt latency fields to be zero, got %+v", outcomes[0])
	}
	if outcomes[1].FinalAttemptLatencyMS != 1 || outcomes[1].TotalInvocationLatencyMS != 1 {
		t.Fatalf("expected synthesized latency fields for pvi-b to equal 1ms, got %+v", outcomes[1])
	}
}

func TestInvocationOutcomesFromProviderAttemptsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	_, err := InvocationOutcomesFromProviderAttempts([]ProviderAttemptEvidence{
		{
			SessionID:            "sess-1",
			TurnID:               "turn-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-1",
			ProviderInvocationID: "pvi-1",
			Modality:             "stt",
			ProviderID:           "stt-a",
			Attempt:              1,
			OutcomeClass:         "unknown",
			Retryable:            false,
			RetryDecision:        "none",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   100,
			WallClockTimestampMS: 100,
		},
	})
	if err == nil {
		t.Fatalf("expected invalid attempt evidence error")
	}
}

func TestAppendHandoffEdges(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(StageAConfig{
		BaselineCapacity: 4,
		DetailCapacity:   4,
		AttemptCapacity:  4,
		HandoffCapacity:  1,
	})
	edges := []HandoffEdgeEvidence{
		{
			SessionID:             "sess-1",
			TurnID:                "turn-1",
			PipelineVersion:       "pipeline-v1",
			EventID:               "evt-1",
			HandoffID:             "handoff-1",
			Edge:                  "stt_to_llm",
			UpstreamRevision:      1,
			Action:                "forward",
			PartialAcceptedAtMS:   100,
			DownstreamStartedAtMS: 110,
			HandoffLatencyMS:      10,
			QueueDepth:            0,
			RuntimeTimestampMS:    100,
			WallClockTimestampMS:  100,
		},
	}
	if err := recorder.AppendHandoffEdges(edges); err != nil {
		t.Fatalf("unexpected handoff append error: %v", err)
	}
	stored := recorder.HandoffEntries()
	if len(stored) != 1 {
		t.Fatalf("expected one handoff edge entry, got %d", len(stored))
	}
	filtered := recorder.HandoffEntriesForTurn("sess-1", "turn-1")
	if len(filtered) != 1 || filtered[0].HandoffID != "handoff-1" {
		t.Fatalf("unexpected handoff turn filter result: %+v", filtered)
	}

	if err := recorder.AppendHandoffEdges(edges); !errors.Is(err, ErrHandoffCapacityExhausted) {
		t.Fatalf("expected handoff capacity exhaustion, got %v", err)
	}
}

func TestHandoffEdgeEvidenceValidate(t *testing.T) {
	t.Parallel()

	valid := HandoffEdgeEvidence{
		SessionID:             "sess-1",
		TurnID:                "turn-1",
		PipelineVersion:       "pipeline-v1",
		EventID:               "evt-1",
		HandoffID:             "handoff-1",
		Edge:                  "llm_to_tts",
		UpstreamRevision:      2,
		Action:                "forward",
		PartialAcceptedAtMS:   200,
		DownstreamStartedAtMS: 210,
		HandoffLatencyMS:      10,
		QueueDepth:            0,
		RuntimeTimestampMS:    200,
		WallClockTimestampMS:  200,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid handoff evidence, got %v", err)
	}

	invalid := valid
	invalid.Edge = "invalid"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected invalid edge to fail validation")
	}
}

func minimalBaseline(turnID string) BaselineEvidence {
	decision := controlplane.DecisionOutcome{
		OutcomeKind:        controlplane.OutcomeAdmit,
		Phase:              controlplane.PhasePreTurn,
		Scope:              controlplane.ScopeTurn,
		SessionID:          "sess-1",
		TurnID:             turnID,
		EventID:            "evt-1",
		RuntimeTimestampMS: 100,
		WallClockMS:        100,
		EmittedBy:          controlplane.EmitterRK25,
		Reason:             "admission_capacity_allow",
	}
	openProposed := int64(90)
	open := int64(100)
	terminal := int64(900)
	firstOutput := int64(300)
	return BaselineEvidence{
		SessionID:        "sess-1",
		TurnID:           turnID,
		PipelineVersion:  "pipeline-v1",
		EventID:          "evt-1",
		EnvelopeSnapshot: "event:turn_open",
		PayloadTags:      []eventabi.PayloadClass{eventabi.PayloadMetadata},
		RedactionDecisions: []eventabi.RedactionDecision{
			{PayloadClass: eventabi.PayloadMetadata, Action: eventabi.RedactionAllow},
		},
		PlanHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotProvenance: controlplane.SnapshotProvenance{
			RoutingViewSnapshot:       "routing-view/v1",
			AdmissionPolicySnapshot:   "admission-policy/v1",
			ABICompatibilitySnapshot:  "abi-compat/v1",
			VersionResolutionSnapshot: "version-resolution/v1",
			PolicyResolutionSnapshot:  "policy-resolution/v1",
			ProviderHealthSnapshot:    "provider-health/v1",
		},
		DecisionOutcomes:     []controlplane.DecisionOutcome{decision},
		DeterminismSeed:      42,
		OrderingMarkers:      []string{"runtime_sequence", "event_id"},
		MergeRuleID:          "default-merge-rule",
		MergeRuleVersion:     "v1.0.0",
		AuthorityEpoch:       7,
		MigrationMarkers:     []string{"lease_rotated"},
		TerminalOutcome:      "commit",
		CloseEmitted:         true,
		TurnOpenProposedAtMS: &openProposed,
		TurnOpenAtMS:         &open,
		TurnTerminalAtMS:     &terminal,
		FirstOutputAtMS:      &firstOutput,
		TerminalReason:       "",
	}
}

func minimalInvocationSnapshot(eventID string) InvocationSnapshotEvidence {
	return InvocationSnapshotEvidence{
		SessionID:                "sess-1",
		TurnID:                   "turn-1",
		PipelineVersion:          "pipeline-v1",
		EventID:                  eventID,
		ProviderInvocationID:     "pvi-1",
		Modality:                 "stt",
		ProviderID:               "stt-a",
		OutcomeClass:             "success",
		Retryable:                false,
		RetryDecision:            "none",
		AttemptCount:             1,
		FinalAttemptLatencyMS:    0,
		TotalInvocationLatencyMS: 0,
		RuntimeTimestampMS:       100,
		WallClockTimestampMS:     100,
	}
}

type capturingDurableExporter struct {
	mu            sync.Mutex
	records       []timelineexporter.Record
	enqueueResult bool
}

func (c *capturingDurableExporter) Enqueue(record timelineexporter.Record) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, record)
	return c.enqueueResult
}

func (c *capturingDurableExporter) Records() []timelineexporter.Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]timelineexporter.Record, len(c.records))
	copy(out, c.records)
	return out
}
