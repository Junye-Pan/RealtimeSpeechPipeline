//go:build liveproviders

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/buffering"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/executionpool"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/executor"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/lanes"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/nodehost"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/prelude"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/bootstrap"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

const (
	a2RuntimeLiveReportJSONPath = ".codex/providers/a2-runtime-live-report.json"
	a2RuntimeLiveReportMDPath   = ".codex/providers/a2-runtime-live-report.md"
)

type a2ScenarioSpec struct {
	ID          string
	Name        string
	Modules     []string
	Evidence    []string
	Description string
	Run         func(strict bool) a2ScenarioOutcome
}

type a2ScenarioOutcome struct {
	Status    string
	Detail    string
	Err       error
	Providers []a2ProviderOutcome
}

type a2ProviderOutcome struct {
	ProviderID    string `json:"provider_id"`
	Modality      string `json:"modality"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	OutcomeClass  string `json:"outcome_class,omitempty"`
	RetryDecision string `json:"retry_decision,omitempty"`
	Attempts      int    `json:"attempts,omitempty"`
}

type a2ScenarioReport struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Modules     []string            `json:"modules"`
	Status      string              `json:"status"`
	Detail      string              `json:"detail,omitempty"`
	Error       string              `json:"error,omitempty"`
	Evidence    []string            `json:"evidence"`
	Providers   []a2ProviderOutcome `json:"providers,omitempty"`
}

type a2ModuleReport struct {
	Module      string   `json:"module"`
	ScenarioID  string   `json:"scenario_id"`
	Status      string   `json:"status"`
	Detail      string   `json:"detail,omitempty"`
	Evidence    []string `json:"evidence"`
	Description string   `json:"description"`
}

type a2RuntimeLiveReport struct {
	GeneratedAtUTC string             `json:"generated_at_utc"`
	StrictMode     bool               `json:"strict_mode"`
	OverallStatus  string             `json:"overall_status"`
	FailCount      int                `json:"fail_count"`
	SkipCount      int                `json:"skip_count"`
	PassCount      int                `json:"pass_count"`
	ScenarioCount  int                `json:"scenario_count"`
	ModuleCount    int                `json:"module_count"`
	Scenarios      []a2ScenarioReport `json:"scenarios"`
	Modules        []a2ModuleReport   `json:"modules"`
}

func TestA2RuntimeLiveScenarios(t *testing.T) {
	t.Helper()
	if !envBool("RSPP_A2_RUNTIME_LIVE", false) {
		t.Skip("A.2 runtime live suite disabled (set RSPP_A2_RUNTIME_LIVE=1)")
	}
	strict := envBool("RSPP_A2_RUNTIME_LIVE_STRICT", false)

	specs := a2RuntimeScenarioSpecs()
	report := a2RuntimeLiveReport{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		StrictMode:     strict,
		ScenarioCount:  len(specs),
		Scenarios:      make([]a2ScenarioReport, 0, len(specs)),
		Modules:        make([]a2ModuleReport, 0, 21),
	}

	failures := make([]string, 0)
	for _, spec := range specs {
		outcome := spec.Run(strict)
		scenario := a2ScenarioReport{
			ID:          spec.ID,
			Name:        spec.Name,
			Description: spec.Description,
			Modules:     append([]string(nil), spec.Modules...),
			Status:      outcome.Status,
			Detail:      outcome.Detail,
			Evidence:    append([]string(nil), spec.Evidence...),
			Providers:   append([]a2ProviderOutcome(nil), outcome.Providers...),
		}
		if outcome.Err != nil {
			scenario.Error = outcome.Err.Error()
		}
		report.Scenarios = append(report.Scenarios, scenario)

		for _, module := range spec.Modules {
			report.Modules = append(report.Modules, a2ModuleReport{
				Module:      module,
				ScenarioID:  spec.ID,
				Status:      outcome.Status,
				Detail:      outcome.Detail,
				Evidence:    append([]string(nil), spec.Evidence...),
				Description: spec.Description,
			})
		}

		switch outcome.Status {
		case "pass":
			report.PassCount++
		case "skip":
			report.SkipCount++
			if strict {
				failures = append(failures, fmt.Sprintf("%s skipped in strict mode: %s", spec.ID, outcome.Detail))
			}
		case "fail":
			report.FailCount++
			if outcome.Err != nil {
				failures = append(failures, fmt.Sprintf("%s failed: %v", spec.ID, outcome.Err))
			} else {
				failures = append(failures, fmt.Sprintf("%s failed: %s", spec.ID, outcome.Detail))
			}
		default:
			report.FailCount++
			failures = append(failures, fmt.Sprintf("%s returned unknown status: %s", spec.ID, outcome.Status))
		}
	}

	report.ModuleCount = len(report.Modules)
	switch {
	case report.FailCount > 0 || (strict && report.SkipCount > 0):
		report.OverallStatus = "fail"
	case report.SkipCount > 0:
		report.OverallStatus = "partial"
	default:
		report.OverallStatus = "pass"
	}

	if err := writeA2RuntimeLiveReport(report); err != nil {
		t.Fatalf("write A.2 runtime live report: %v", err)
	}

	if len(failures) > 0 {
		t.Fatalf("A.2 runtime live validation failed (%d issue(s)): %s", len(failures), strings.Join(failures, "; "))
	}
}

func a2RuntimeScenarioSpecs() []a2ScenarioSpec {
	return []a2ScenarioSpec{
		{
			ID:      "S1",
			Name:    "prelude-and-turn-lifecycle",
			Modules: []string{"RK-02", "RK-03", "RK-04", "RK-19", "RK-21"},
			Evidence: []string{
				"internal/runtime/prelude/engine.go",
				"internal/runtime/turnarbiter/arbiter.go",
				"internal/runtime/planresolver/resolver.go",
				"internal/runtime/determinism/service.go",
				"internal/runtime/identity/context.go",
				"test/integration/a2_runtime_live_test.go",
			},
			Description: "Prelude proposal + deterministic turn-open + plan determinism + identity-generated event context.",
			Run:         runS1PreludeLifecycle,
		},
		{
			ID:      "S2",
			Name:    "eventabi-transport-cancel-fence",
			Modules: []string{"RK-05", "RK-16", "RK-22", "RK-23"},
			Evidence: []string{
				"internal/runtime/eventabi/gateway.go",
				"internal/runtime/transport/fence.go",
				"internal/runtime/cancellation/fence.go",
				"internal/runtime/transport/signals.go",
				"test/integration/a2_runtime_live_test.go",
			},
			Description: "Event ABI normalization + transport connection signals + deterministic cancel fence behavior.",
			Run:         runS2TransportAndABI,
		},
		{
			ID:      "S3",
			Name:    "lane-router-executor-admission-pool",
			Modules: []string{"RK-06", "RK-07", "RK-25", "RK-26"},
			Evidence: []string{
				"internal/runtime/lanes/router.go",
				"internal/runtime/executor/scheduler.go",
				"internal/runtime/executor/plan.go",
				"internal/runtime/localadmission/localadmission.go",
				"internal/runtime/executionpool/pool.go",
				"test/integration/a2_runtime_live_test.go",
			},
			Description: "Deterministic routing, execution ordering, shed admission, and bounded execution-pool dispatch.",
			Run:         runS3ExecutionDispatch,
		},
		{
			ID:      "S4",
			Name:    "failure-budget-buffering-flowcontrol",
			Modules: []string{"RK-08", "RK-12", "RK-13", "RK-14", "RK-17"},
			Evidence: []string{
				"internal/runtime/nodehost/failure.go",
				"internal/runtime/budget/manager.go",
				"internal/runtime/buffering/pressure.go",
				"internal/runtime/flowcontrol/controller.go",
				"internal/runtime/buffering/drop_notice.go",
				"test/integration/a2_runtime_live_test.go",
			},
			Description: "Node-failure shaping + budget action + pressure watermark/drop + flow-control recovery.",
			Run:         runS4FailurePressureBudget,
		},
		{
			ID:      "S5",
			Name:    "real-provider-invocation",
			Modules: []string{"RK-10", "RK-11"},
			Evidence: []string{
				"internal/runtime/provider/bootstrap/bootstrap.go",
				"internal/runtime/provider/contracts/contracts.go",
				"internal/runtime/provider/invocation/controller.go",
				"internal/observability/timeline/recorder.go",
				"test/integration/provider_live_smoke_test.go",
				"test/integration/a2_runtime_live_test.go",
			},
			Description: "Real provider invocation matrix plus scheduler/timeline attempt evidence path.",
			Run:         runS5ProviderInvocationLive,
		},
		{
			ID:      "S6",
			Name:    "authority-revoke-runtime",
			Modules: []string{"RK-24"},
			Evidence: []string{
				"internal/runtime/guard/guard.go",
				"internal/runtime/turnarbiter/arbiter.go",
				"test/integration/a2_runtime_live_test.go",
			},
			Description: "In-turn authority revoke path emits deterministic deauthorized drain and terminal close sequence.",
			Run:         runS6AuthorityRevoke,
		},
	}
}

func runS1PreludeLifecycle(strict bool) a2ScenarioOutcome {
	now := time.Now().UnixMilli()
	engine := prelude.NewEngine()
	proposal, err := engine.ProposeTurn(prelude.Input{
		SessionID:            "sess-a2-live-s1",
		TurnID:               "turn-a2-live-s1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-a2-live-s1-prelude",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       7,
		RuntimeTimestampMS:   now,
		WallClockTimestampMS: now,
	})
	if err != nil {
		return failOutcome(err)
	}
	if proposal.Signal.Signal != "turn_open_proposed" || proposal.Signal.EmittedBy != "RK-02" {
		return failf("unexpected prelude signal: signal=%s emitted_by=%s", proposal.Signal.Signal, proposal.Signal.EmittedBy)
	}
	if proposal.Identity.EventID == "" || proposal.Identity.IdempotencyKey == "" || proposal.Identity.CorrelationID == "" {
		return failf("incomplete identity context: %+v", proposal.Identity)
	}

	arbiter := turnarbiter.New()
	openResult, err := arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
		SessionID:            "sess-a2-live-s1",
		TurnID:               "turn-a2-live-s1",
		EventID:              proposal.Signal.EventID,
		RuntimeTimestampMS:   now + 1,
		WallClockTimestampMS: now + 1,
		PipelineVersion:      "pipeline-v1",
		AuthorityEpoch:       7,
		SnapshotValid:        true,
		AuthorityEpochValid:  true,
		AuthorityAuthorized:  true,
	})
	if err != nil {
		return failOutcome(err)
	}
	if openResult.State != controlplane.TurnActive || openResult.Plan == nil {
		return failf("expected active turn with plan, got state=%s plan_nil=%t", openResult.State, openResult.Plan == nil)
	}
	if err := openResult.Plan.Determinism.Validate(); err != nil {
		return failf("resolved turn plan determinism invalid: %v", err)
	}

	scheduler := executor.NewScheduler(localadmission.Evaluator{})
	decision, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-a2-live-s1",
		TurnID:               "turn-a2-live-s1",
		EventID:              "",
		PipelineVersion:      "pipeline-v1",
		RuntimeTimestampMS:   now + 2,
		WallClockTimestampMS: now + 2,
		Shed:                 true,
		Reason:               "a2_runtime_live_identity_generation",
	})
	if err != nil {
		return failOutcome(err)
	}
	if decision.Allowed || decision.ControlSignal == nil {
		return failf("expected shed control signal with generated event id, got %+v", decision)
	}
	if strings.TrimSpace(decision.ControlSignal.EventID) == "" {
		return failf("scheduler did not populate generated event_id under identity service")
	}

	return passf(
		"proposal_event=%s plan_hash=%s generated_shed_event=%s",
		proposal.Signal.EventID,
		openResult.Plan.PlanHash,
		decision.ControlSignal.EventID,
	)
}

func runS2TransportAndABI(strict bool) a2ScenarioOutcome {
	now := time.Now().UnixMilli()

	connectionSignal, err := transport.BuildConnectionSignal(transport.ConnectionSignalInput{
		SessionID:            "sess-a2-live-s2",
		TurnID:               "turn-a2-live-s2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-a2-live-s2-connected",
		Signal:               "connected",
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   now,
		WallClockTimestampMS: now,
		Reason:               "a2_runtime_live_transport_connected",
	})
	if err != nil {
		return failOutcome(err)
	}
	if _, err := runtimeeventabi.ValidateAndNormalizeControlSignals([]eventabi.ControlSignal{connectionSignal}); err != nil {
		return failf("control signal normalization failed: %v", err)
	}

	record, err := runtimeeventabi.ValidateAndNormalizeEventRecords([]eventabi.EventRecord{
		{
			SchemaVersion:      "",
			EventScope:         eventabi.ScopeTurn,
			SessionID:          "sess-a2-live-s2",
			TurnID:             "turn-a2-live-s2",
			PipelineVersion:    "pipeline-v1",
			EventID:            "evt-a2-live-s2-record",
			Lane:               eventabi.LaneData,
			TransportSequence:  nil,
			RuntimeSequence:    -1,
			AuthorityEpoch:     nil,
			RuntimeTimestampMS: -1,
			WallClockMS:        -1,
			PayloadClass:       eventabi.PayloadTextRaw,
		},
	})
	if err != nil {
		return failf("event record normalization failed: %v", err)
	}
	if len(record) != 1 || record[0].TransportSequence == nil || *record[0].TransportSequence != 0 {
		return failf("event record normalization produced unexpected transport_sequence: %+v", record)
	}

	fence := transport.NewOutputFence()
	accepted, err := fence.EvaluateOutput(transport.OutputAttempt{
		SessionID:            "sess-a2-live-s2",
		TurnID:               "turn-a2-live-s2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-a2-live-s2-output-accepted",
		TransportSequence:    3,
		RuntimeSequence:      3,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   now + 1,
		WallClockTimestampMS: now + 1,
		CancelAccepted:       false,
	})
	if err != nil {
		return failOutcome(err)
	}
	if !accepted.Accepted || accepted.Signal.Signal != "output_accepted" {
		return failf("expected accepted output before cancel fence, got %+v", accepted)
	}

	cancelled, err := fence.EvaluateOutput(transport.OutputAttempt{
		SessionID:            "sess-a2-live-s2",
		TurnID:               "turn-a2-live-s2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-a2-live-s2-output-cancelled",
		TransportSequence:    4,
		RuntimeSequence:      4,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   now + 2,
		WallClockTimestampMS: now + 2,
		CancelAccepted:       true,
	})
	if err != nil {
		return failOutcome(err)
	}
	if cancelled.Accepted || cancelled.Signal.Signal != "playback_cancelled" {
		return failf("expected playback_cancelled after cancel acceptance, got %+v", cancelled)
	}

	return passf("transport_signal=%s cancel_fence_signal=%s", connectionSignal.Signal, cancelled.Signal.Signal)
}

func runS3ExecutionDispatch(strict bool) a2ScenarioOutcome {
	now := time.Now().UnixMilli()
	router := lanes.NewDefaultRouter()
	route, err := router.Resolve("stt", eventabi.LaneData)
	if err != nil {
		return failOutcome(err)
	}
	if !strings.HasPrefix(route.QueueKey, "runtime/data/") {
		return failf("unexpected lane route key: %s", route.QueueKey)
	}

	pool := executionpool.NewManager(8)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pool.Drain(ctx)
	}()

	scheduler := executor.NewSchedulerWithExecutionPool(localadmission.Evaluator{}, pool)
	trace, err := scheduler.ExecutePlan(executor.SchedulingInput{
		SessionID:            "sess-a2-live-s3",
		TurnID:               "turn-a2-live-s3",
		EventID:              "evt-a2-live-s3-plan",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    10,
		RuntimeSequence:      10,
		AuthorityEpoch:       5,
		RuntimeTimestampMS:   now,
		WallClockTimestampMS: now,
	}, executor.ExecutionPlan{
		Nodes: []executor.NodeSpec{
			{NodeID: "node-stt", NodeType: "stt", Lane: eventabi.LaneData},
			{NodeID: "node-llm", NodeType: "llm", Lane: eventabi.LaneData},
		},
		Edges: []executor.EdgeSpec{
			{From: "node-stt", To: "node-llm"},
		},
	})
	if err != nil {
		return failOutcome(err)
	}
	if !trace.Completed || len(trace.NodeOrder) != 2 || len(trace.Nodes) != 2 {
		return failf("unexpected execution trace completeness: %+v", trace)
	}

	stats := pool.Stats()
	if stats.Submitted < 2 || stats.Completed < 2 {
		return failf("execution pool stats missing expected dispatch volume: %+v", stats)
	}

	shedDecision, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-a2-live-s3",
		TurnID:               "turn-a2-live-s3",
		EventID:              "evt-a2-live-s3-shed",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    50,
		RuntimeSequence:      50,
		AuthorityEpoch:       5,
		RuntimeTimestampMS:   now + 10,
		WallClockTimestampMS: now + 10,
		Shed:                 true,
		Reason:               "a2_runtime_live_shed",
	})
	if err != nil {
		return failOutcome(err)
	}
	if shedDecision.Allowed || shedDecision.ControlSignal == nil || shedDecision.ControlSignal.EmittedBy != "RK-25" {
		return failf("expected RK-25 shed decision, got %+v", shedDecision)
	}

	return passf("node_order=%s pool_submitted=%d shed_signal=%s", strings.Join(trace.NodeOrder, ","), stats.Submitted, shedDecision.ControlSignal.Signal)
}

func runS4FailurePressureBudget(strict bool) a2ScenarioOutcome {
	now := time.Now().UnixMilli()

	failureResult, err := nodehost.HandleFailure(nodehost.NodeFailureInput{
		SessionID:            "sess-a2-live-s4",
		TurnID:               "turn-a2-live-s4",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-a2-live-s4-node-failure",
		TransportSequence:    20,
		RuntimeSequence:      20,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   now,
		WallClockTimestampMS: now,
		NodeBudgetWarningMS:  1200,
		NodeBudgetExhaustMS:  1500,
		ObservedRuntimeMS:    1700,
		AllowDegrade:         true,
		AllowFallback:        false,
	})
	if err != nil {
		return failOutcome(err)
	}
	if failureResult.Terminal {
		return failf("expected degrade non-terminal decision, got terminal result %+v", failureResult)
	}
	if !hasSignal(failureResult.Signals, "budget_warning", "RK-17") ||
		!hasSignal(failureResult.Signals, "budget_exhausted", "RK-17") ||
		!hasSignal(failureResult.Signals, "degrade", "RK-17") {
		return failf("node failure signals missing expected RK-17 markers: %+v", failureResult.Signals)
	}

	pressureResult, err := buffering.HandleEdgePressure(localadmission.Evaluator{}, buffering.PressureInput{
		SessionID:            "sess-a2-live-s4",
		TurnID:               "turn-a2-live-s4",
		PipelineVersion:      "pipeline-v1",
		EdgeID:               "edge-a2-live-s4",
		EventID:              "evt-a2-live-s4-pressure",
		TransportSequence:    30,
		RuntimeSequence:      30,
		AuthorityEpoch:       9,
		RuntimeTimestampMS:   now + 1,
		WallClockTimestampMS: now + 1,
		TargetLane:           eventabi.LaneData,
		DropRangeStart:       30,
		DropRangeEnd:         33,
		HighWatermark:        true,
		EmitRecovery:         true,
		Shed:                 true,
	})
	if err != nil {
		return failOutcome(err)
	}
	if !hasSignal(pressureResult.Signals, "watermark", "RK-13") ||
		!hasSignal(pressureResult.Signals, "drop_notice", "RK-12") ||
		!hasSignal(pressureResult.Signals, "flow_xoff", "RK-14") ||
		!hasSignal(pressureResult.Signals, "flow_xon", "RK-14") {
		return failf("pressure result missing expected signals: %+v", pressureResult.Signals)
	}
	if pressureResult.ShedOutcome == nil || pressureResult.ShedOutcome.OutcomeKind != controlplane.OutcomeShed {
		return failf("expected shed decision from pressure handling, got %+v", pressureResult.ShedOutcome)
	}

	return passf("failure_signals=%d pressure_signals=%d", len(failureResult.Signals), len(pressureResult.Signals))
}

func runS5ProviderInvocationLive(strict bool) a2ScenarioOutcome {
	now := time.Now().UnixMilli()
	runtimeProviders, err := bootstrap.BuildMVPProviders()
	if err != nil {
		return failf("provider bootstrap failed: %v", err)
	}

	cases := []struct {
		ProviderID string
		Modality   contracts.Modality
		EnableEnv  string
		Required   []string
	}{
		{ProviderID: "stt-deepgram", Modality: contracts.ModalitySTT, EnableEnv: "RSPP_STT_DEEPGRAM_ENABLE", Required: []string{"RSPP_STT_DEEPGRAM_API_KEY"}},
		{ProviderID: "stt-google", Modality: contracts.ModalitySTT, EnableEnv: "RSPP_STT_GOOGLE_ENABLE", Required: []string{"RSPP_STT_GOOGLE_API_KEY"}},
		{ProviderID: "stt-assemblyai", Modality: contracts.ModalitySTT, EnableEnv: "RSPP_STT_ASSEMBLYAI_ENABLE", Required: []string{"RSPP_STT_ASSEMBLYAI_API_KEY"}},
		{ProviderID: "llm-anthropic", Modality: contracts.ModalityLLM, EnableEnv: "RSPP_LLM_ANTHROPIC_ENABLE", Required: []string{"RSPP_LLM_ANTHROPIC_API_KEY"}},
		{ProviderID: "llm-gemini", Modality: contracts.ModalityLLM, EnableEnv: "RSPP_LLM_GEMINI_ENABLE", Required: []string{"RSPP_LLM_GEMINI_API_KEY"}},
		{ProviderID: "llm-cohere", Modality: contracts.ModalityLLM, EnableEnv: "RSPP_LLM_COHERE_ENABLE", Required: []string{"RSPP_LLM_COHERE_API_KEY"}},
		{ProviderID: "tts-elevenlabs", Modality: contracts.ModalityTTS, EnableEnv: "RSPP_TTS_ELEVENLABS_ENABLE", Required: []string{"RSPP_TTS_ELEVENLABS_API_KEY"}},
		{ProviderID: "tts-google", Modality: contracts.ModalityTTS, EnableEnv: "RSPP_TTS_GOOGLE_ENABLE", Required: []string{"RSPP_TTS_GOOGLE_API_KEY"}},
		{ProviderID: "tts-amazon-polly", Modality: contracts.ModalityTTS, EnableEnv: "RSPP_TTS_POLLY_ENABLE", Required: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}},
	}

	outcomes := make([]a2ProviderOutcome, 0, len(cases))
	executed := 0
	type firstProvider struct {
		id       string
		modality contracts.Modality
	}
	var firstSuccess *firstProvider

	for _, tc := range cases {
		outcome := a2ProviderOutcome{
			ProviderID: tc.ProviderID,
			Modality:   string(tc.Modality),
		}
		if !envBool(tc.EnableEnv, false) {
			outcome.Status = "skip"
			outcome.Reason = fmt.Sprintf("%s!=1", tc.EnableEnv)
			outcomes = append(outcomes, outcome)
			continue
		}

		missing := missingEnvs(tc.Required)
		if len(missing) > 0 {
			outcome.Status = "skip"
			outcome.Reason = "missing required env: " + strings.Join(missing, ",")
			outcomes = append(outcomes, outcome)
			continue
		}

		result, invokeErr := runtimeProviders.Controller.Invoke(invocation.InvocationInput{
			SessionID:              "sess-a2-live-s5",
			TurnID:                 "turn-a2-live-s5",
			PipelineVersion:        "pipeline-v1",
			EventID:                "evt-a2-live-s5-" + tc.ProviderID,
			Modality:               tc.Modality,
			PreferredProvider:      tc.ProviderID,
			AllowedAdaptiveActions: []string{"retry", "provider_switch", "fallback"},
			TransportSequence:      40,
			RuntimeSequence:        40,
			AuthorityEpoch:         11,
			RuntimeTimestampMS:     now,
			WallClockTimestampMS:   now,
		})
		if invokeErr != nil {
			outcome.Status = "fail"
			outcome.Reason = invokeErr.Error()
			outcomes = append(outcomes, outcome)
			return a2ScenarioOutcome{
				Status:    "fail",
				Detail:    "provider invocation returned error",
				Err:       fmt.Errorf("%s invoke failed: %w", tc.ProviderID, invokeErr),
				Providers: outcomes,
			}
		}

		outcome.OutcomeClass = string(result.Outcome.Class)
		outcome.RetryDecision = result.RetryDecision
		outcome.Attempts = len(result.Attempts)
		if result.Outcome.Class != contracts.OutcomeSuccess {
			outcome.Status = "fail"
			outcome.Reason = fmt.Sprintf("expected success, got class=%s reason=%s", result.Outcome.Class, result.Outcome.Reason)
			outcomes = append(outcomes, outcome)
			return a2ScenarioOutcome{
				Status:    "fail",
				Detail:    "provider returned non-success class",
				Err:       fmt.Errorf("%s outcome=%s reason=%s", tc.ProviderID, result.Outcome.Class, result.Outcome.Reason),
				Providers: outcomes,
			}
		}

		outcome.Status = "pass"
		executed++
		if firstSuccess == nil {
			firstSuccess = &firstProvider{id: tc.ProviderID, modality: tc.Modality}
		}
		outcomes = append(outcomes, outcome)
	}

	if executed == 0 {
		return a2ScenarioOutcome{
			Status:    "skip",
			Detail:    "no enabled providers executed",
			Providers: outcomes,
		}
	}

	recorder := timeline.NewRecorder(timeline.StageAConfig{
		BaselineCapacity: 8,
		DetailCapacity:   16,
		AttemptCapacity:  32,
	})
	scheduler := executor.NewSchedulerWithProviderInvokerAndAttemptAppender(
		localadmission.Evaluator{},
		runtimeProviders.Controller,
		&recorder,
	)
	decision, err := scheduler.NodeDispatch(executor.SchedulingInput{
		SessionID:            "sess-a2-live-s5",
		TurnID:               "turn-a2-live-s5",
		EventID:              "evt-a2-live-s5-scheduler",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    41,
		RuntimeSequence:      41,
		AuthorityEpoch:       11,
		RuntimeTimestampMS:   now + 1,
		WallClockTimestampMS: now + 1,
		ProviderInvocation: &executor.ProviderInvocationInput{
			Modality:               firstSuccess.modality,
			PreferredProvider:      firstSuccess.id,
			AllowedAdaptiveActions: []string{"retry", "provider_switch", "fallback"},
		},
	})
	if err != nil {
		return a2ScenarioOutcome{
			Status:    "fail",
			Detail:    "scheduler invocation integration failed",
			Err:       err,
			Providers: outcomes,
		}
	}
	if !decision.Allowed || decision.Provider == nil {
		return a2ScenarioOutcome{
			Status:    "fail",
			Detail:    "scheduler provider decision not allowed",
			Err:       fmt.Errorf("unexpected scheduler provider decision: %+v", decision),
			Providers: outcomes,
		}
	}
	if len(recorder.ProviderAttemptEntries()) == 0 {
		return a2ScenarioOutcome{
			Status:    "fail",
			Detail:    "no provider attempt evidence persisted to timeline recorder",
			Err:       fmt.Errorf("provider attempt recorder is empty after scheduler invocation"),
			Providers: outcomes,
		}
	}

	return a2ScenarioOutcome{
		Status:    "pass",
		Detail:    fmt.Sprintf("executed_providers=%d first_success=%s scheduler_attempt_entries=%d", executed, firstSuccess.id, len(recorder.ProviderAttemptEntries())),
		Providers: outcomes,
	}
}

func runS6AuthorityRevoke(strict bool) a2ScenarioOutcome {
	now := time.Now().UnixMilli()
	arbiter := turnarbiter.New()
	activeResult, err := arbiter.HandleActive(turnarbiter.ActiveInput{
		SessionID:            "sess-a2-live-s6",
		TurnID:               "turn-a2-live-s6",
		EventID:              "evt-a2-live-s6-revoke",
		PipelineVersion:      "pipeline-v1",
		TransportSequence:    60,
		RuntimeSequence:      60,
		RuntimeTimestampMS:   now,
		WallClockTimestampMS: now,
		AuthorityEpoch:       13,
		AuthorityRevoked:     true,
	})
	if err != nil {
		return failOutcome(err)
	}
	if activeResult.State != controlplane.TurnClosed || activeResult.Decision == nil {
		return failf("authority revoke path did not produce terminal close with decision: %+v", activeResult)
	}
	if activeResult.Decision.OutcomeKind != controlplane.OutcomeDeauthorized {
		return failf("expected deauthorized outcome, got %s", activeResult.Decision.OutcomeKind)
	}
	if !hasSignal(activeResult.ControlLane, "deauthorized_drain", "RK-24") {
		return failf("authority revoke path missing RK-24 deauthorized_drain signal: %+v", activeResult.ControlLane)
	}

	return passf("terminal_state=%s decision=%s", activeResult.State, activeResult.Decision.OutcomeKind)
}

func hasSignal(signals []eventabi.ControlSignal, signalName string, emittedBy string) bool {
	for _, sig := range signals {
		if sig.Signal == signalName && sig.EmittedBy == emittedBy {
			return true
		}
	}
	return false
}

func missingEnvs(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func passf(format string, args ...any) a2ScenarioOutcome {
	return a2ScenarioOutcome{Status: "pass", Detail: fmt.Sprintf(format, args...)}
}

func failOutcome(err error) a2ScenarioOutcome {
	return a2ScenarioOutcome{Status: "fail", Err: err, Detail: err.Error()}
}

func failf(format string, args ...any) a2ScenarioOutcome {
	err := fmt.Errorf(format, args...)
	return failOutcome(err)
}

func writeA2RuntimeLiveReport(report a2RuntimeLiveReport) error {
	sort.SliceStable(report.Modules, func(i, j int) bool {
		return report.Modules[i].Module < report.Modules[j].Module
	})

	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(root, filepath.FromSlash(a2RuntimeLiveReportJSONPath))
	mdPath := filepath.Join(root, filepath.FromSlash(a2RuntimeLiveReportMDPath))

	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, payload, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(mdPath, []byte(renderA2RuntimeLiveMarkdown(report)), 0o644); err != nil {
		return err
	}
	return nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repo root not found from %s", cwd)
		}
		dir = parent
	}
}

func renderA2RuntimeLiveMarkdown(report a2RuntimeLiveReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# A.2 Runtime Live Validation Report\n\n")
	fmt.Fprintf(&b, "- Generated at (UTC): `%s`\n", report.GeneratedAtUTC)
	fmt.Fprintf(&b, "- Strict mode: `%t`\n", report.StrictMode)
	fmt.Fprintf(&b, "- Overall status: `%s`\n", report.OverallStatus)
	fmt.Fprintf(&b, "- Scenarios: pass=%d skip=%d fail=%d total=%d\n", report.PassCount, report.SkipCount, report.FailCount, report.ScenarioCount)
	fmt.Fprintf(&b, "- Modules: total=%d\n\n", report.ModuleCount)

	fmt.Fprintf(&b, "## Scenario Results\n\n")
	fmt.Fprintf(&b, "| Scenario | Status | Modules | Detail |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- |\n")
	for _, scenario := range report.Scenarios {
		detail := scenario.Detail
		if scenario.Error != "" {
			if detail != "" {
				detail += " | "
			}
			detail += scenario.Error
		}
		fmt.Fprintf(
			&b,
			"| `%s` | `%s` | `%s` | %s |\n",
			scenario.ID,
			scenario.Status,
			strings.Join(scenario.Modules, ", "),
			escapeMDCell(detail),
		)
	}

	fmt.Fprintf(&b, "\n## Module Results\n\n")
	fmt.Fprintf(&b, "| Module | Scenario | Status | Detail |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- |\n")
	for _, module := range report.Modules {
		fmt.Fprintf(
			&b,
			"| `%s` | `%s` | `%s` | %s |\n",
			module.Module,
			module.ScenarioID,
			module.Status,
			escapeMDCell(module.Detail),
		)
	}

	fmt.Fprintf(&b, "\n## Provider Outcomes\n\n")
	fmt.Fprintf(&b, "| Provider | Modality | Status | Outcome | Retry | Attempts | Reason |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- |\n")
	providerRows := 0
	for _, scenario := range report.Scenarios {
		for _, provider := range scenario.Providers {
			providerRows++
			fmt.Fprintf(
				&b,
				"| `%s` | `%s` | `%s` | `%s` | `%s` | `%d` | %s |\n",
				provider.ProviderID,
				provider.Modality,
				provider.Status,
				provider.OutcomeClass,
				provider.RetryDecision,
				provider.Attempts,
				escapeMDCell(provider.Reason),
			)
		}
	}
	if providerRows == 0 {
		fmt.Fprintf(&b, "| _none_ | _n/a_ | _n/a_ | _n/a_ | _n/a_ | `0` | no provider scenarios emitted rows |\n")
	}

	return b.String()
}

func escapeMDCell(value string) string {
	v := strings.ReplaceAll(value, "\n", " ")
	v = strings.ReplaceAll(v, "|", "\\|")
	return strings.TrimSpace(v)
}
