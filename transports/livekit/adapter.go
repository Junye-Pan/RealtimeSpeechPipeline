package livekit

import (
	"context"
	"fmt"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/controlplane"
	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/observability/timeline"
	runtimeeventabi "github.com/tiger/realtime-speech-pipeline/internal/runtime/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/prelude"
	runtimetransport "github.com/tiger/realtime-speech-pipeline/internal/runtime/transport"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/turnarbiter"
)

// EventKind defines deterministic adapter input events used for transport-path evidence.
type EventKind string

const (
	EventConnected           EventKind = "connected"
	EventReconnecting        EventKind = "reconnecting"
	EventDisconnected        EventKind = "disconnected"
	EventEnded               EventKind = "ended"
	EventSilence             EventKind = "silence"
	EventStall               EventKind = "stall"
	EventTurnOpenProposed    EventKind = "turn_open_proposed"
	EventIngressAudio        EventKind = "ingress_audio"
	EventIngressData         EventKind = "ingress_data"
	EventIngressTelemetry    EventKind = "ingress_telemetry"
	EventOutput              EventKind = "output"
	EventCancel              EventKind = "cancel"
	EventAuthorityRevoked    EventKind = "authority_revoked"
	EventTransportDisconnect EventKind = "transport_disconnect"
	EventTerminalSuccess     EventKind = "terminal_success"
)

// Event is a deterministic transport-adapter event fixture.
type Event struct {
	Kind                 EventKind                `json:"kind"`
	SessionID            string                   `json:"session_id,omitempty"`
	TurnID               string                   `json:"turn_id,omitempty"`
	PipelineVersion      string                   `json:"pipeline_version,omitempty"`
	EventID              string                   `json:"event_id,omitempty"`
	TransportSequence    int64                    `json:"transport_sequence,omitempty"`
	RuntimeSequence      int64                    `json:"runtime_sequence,omitempty"`
	AuthorityEpoch       int64                    `json:"authority_epoch,omitempty"`
	RuntimeTimestampMS   int64                    `json:"runtime_timestamp_ms,omitempty"`
	WallClockTimestampMS int64                    `json:"wall_clock_timestamp_ms,omitempty"`
	PayloadClass         eventabi.PayloadClass    `json:"payload_class,omitempty"`
	CancelAccepted       bool                     `json:"cancel_accepted,omitempty"`
	Reason               string                   `json:"reason,omitempty"`
	SnapshotValid        *bool                    `json:"snapshot_valid,omitempty"`
	AuthorityEpochValid  *bool                    `json:"authority_epoch_valid,omitempty"`
	AuthorityAuthorized  *bool                    `json:"authority_authorized,omitempty"`
	PlanFailurePolicy    controlplane.OutcomeKind `json:"plan_failure_policy,omitempty"`
}

// OpenResultSummary records deterministic turn-open outcomes from adapter processing.
type OpenResultSummary struct {
	State           controlplane.TurnState       `json:"state"`
	EventID         string                       `json:"event_id"`
	LifecycleEvents []turnarbiter.LifecycleEvent `json:"lifecycle_events"`
	ControlSignals  []eventabi.ControlSignal     `json:"control_signals,omitempty"`
}

// ActiveResultSummary records deterministic active-turn outcomes from adapter processing.
type ActiveResultSummary struct {
	State           controlplane.TurnState       `json:"state"`
	EventID         string                       `json:"event_id"`
	LifecycleEvents []turnarbiter.LifecycleEvent `json:"lifecycle_events"`
	ControlSignals  []eventabi.ControlSignal     `json:"control_signals,omitempty"`
}

// OutputDecisionSummary captures deterministic egress-fence decisions.
type OutputDecisionSummary struct {
	EventID   string `json:"event_id"`
	Accepted  bool   `json:"accepted"`
	Signal    string `json:"signal"`
	Reason    string `json:"reason"`
	EmittedBy string `json:"emitted_by"`
}

// Report is a deterministic artifact for transport-path evidence.
type Report struct {
	GeneratedAtUTC  string                       `json:"generated_at_utc"`
	SessionID       string                       `json:"session_id"`
	TurnID          string                       `json:"turn_id"`
	PipelineVersion string                       `json:"pipeline_version"`
	ProcessedEvents int                          `json:"processed_events"`
	IngressRecords  []eventabi.EventRecord       `json:"ingress_records,omitempty"`
	ControlSignals  []eventabi.ControlSignal     `json:"control_signals,omitempty"`
	LifecycleEvents []turnarbiter.LifecycleEvent `json:"lifecycle_events,omitempty"`
	OpenResults     []OpenResultSummary          `json:"open_results,omitempty"`
	ActiveResults   []ActiveResultSummary        `json:"active_results,omitempty"`
	OutputDecisions []OutputDecisionSummary      `json:"output_decisions,omitempty"`
	Probe           ProbeResult                  `json:"probe"`
}

// Dependencies wires deterministic seams for tests and runtime integrations.
type Dependencies struct {
	Now         func() time.Time
	Arbiter     turnarbiter.Arbiter
	Prelude     prelude.Engine
	OutputFence *runtimetransport.OutputFence
}

// Adapter maps transport events into RK-22/RK-23 and arbiter contract surfaces.
type Adapter struct {
	cfg     Config
	now     func() time.Time
	arbiter turnarbiter.Arbiter
	prelude prelude.Engine
	fence   *runtimetransport.OutputFence
}

// NewAdapter constructs a deterministic LiveKit transport adapter.
func NewAdapter(cfg Config, deps Dependencies) (Adapter, error) {
	if err := cfg.Validate(); err != nil {
		return Adapter{}, err
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.OutputFence == nil {
		deps.OutputFence = runtimetransport.NewOutputFence()
	}
	if deps.Prelude == (prelude.Engine{}) {
		deps.Prelude = prelude.NewEngine()
	}
	if deps.Arbiter == (turnarbiter.Arbiter{}) {
		recorder := timeline.NewRecorder(timeline.StageAConfig{
			BaselineCapacity: 256,
			DetailCapacity:   512,
		})
		arbiter, err := turnarbiter.NewWithControlPlaneBackendsFromDistributionEnv(&recorder)
		if err != nil {
			arbiter = turnarbiter.NewWithRecorder(&recorder)
		}
		deps.Arbiter = arbiter
	}

	return Adapter{
		cfg:     cfg,
		now:     deps.Now,
		arbiter: deps.Arbiter,
		prelude: deps.Prelude,
		fence:   deps.OutputFence,
	}, nil
}

// Process maps a deterministic event sequence through RK-22/RK-23 and turn-arbiter seams.
func (a Adapter) Process(ctx context.Context, events []Event) (Report, error) {
	report := Report{
		SessionID:       a.cfg.SessionID,
		TurnID:          a.cfg.TurnID,
		PipelineVersion: a.cfg.PipelineVersion,
		Probe:           ProbeResult{Status: ProbeStatusSkipped},
	}
	var transportSeq int64
	var runtimeSeq int64

	for idx, raw := range events {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		default:
		}

		ev, err := a.normalizeEvent(raw, idx, &transportSeq, &runtimeSeq)
		if err != nil {
			return report, err
		}

		switch ev.Kind {
		case EventConnected, EventReconnecting, EventDisconnected, EventEnded, EventSilence, EventStall:
			signal, err := runtimetransport.BuildConnectionSignal(runtimetransport.ConnectionSignalInput{
				SessionID:            ev.SessionID,
				TurnID:               ev.TurnID,
				PipelineVersion:      ev.PipelineVersion,
				EventID:              ev.EventID,
				Signal:               string(ev.Kind),
				TransportSequence:    ev.TransportSequence,
				RuntimeSequence:      ev.RuntimeSequence,
				AuthorityEpoch:       ev.AuthorityEpoch,
				RuntimeTimestampMS:   ev.RuntimeTimestampMS,
				WallClockTimestampMS: ev.WallClockTimestampMS,
				Reason:               defaultString(ev.Reason, "livekit_"+string(ev.Kind)),
			})
			if err != nil {
				return report, err
			}
			report.ControlSignals = append(report.ControlSignals, signal)

		case EventTurnOpenProposed:
			proposal, err := a.prelude.ProposeTurn(prelude.Input{
				SessionID:            ev.SessionID,
				TurnID:               ev.TurnID,
				PipelineVersion:      ev.PipelineVersion,
				EventID:              ev.EventID,
				TransportSequence:    ev.TransportSequence,
				RuntimeSequence:      ev.RuntimeSequence,
				AuthorityEpoch:       ev.AuthorityEpoch,
				RuntimeTimestampMS:   ev.RuntimeTimestampMS,
				WallClockTimestampMS: ev.WallClockTimestampMS,
				Reason:               defaultString(ev.Reason, "livekit_turn_open_proposed"),
			})
			if err != nil {
				return report, err
			}

			openResult, err := a.arbiter.HandleTurnOpenProposed(turnarbiter.OpenRequest{
				SessionID:             ev.SessionID,
				TurnID:                ev.TurnID,
				EventID:               proposal.Signal.EventID,
				RuntimeTimestampMS:    ev.RuntimeTimestampMS,
				WallClockTimestampMS:  ev.WallClockTimestampMS,
				PipelineVersion:       ev.PipelineVersion,
				AuthorityEpoch:        ev.AuthorityEpoch,
				SnapshotValid:         boolOrDefault(ev.SnapshotValid, true),
				AuthorityEpochValid:   boolOrDefault(ev.AuthorityEpochValid, true),
				AuthorityAuthorized:   boolOrDefault(ev.AuthorityAuthorized, true),
				PlanFailurePolicy:     ev.PlanFailurePolicy,
				SnapshotFailurePolicy: controlplane.OutcomeDefer,
			})
			if err != nil {
				return report, err
			}

			report.LifecycleEvents = append(report.LifecycleEvents, openResult.Events...)
			report.ControlSignals = append(report.ControlSignals, openResult.ControlLane...)
			report.OpenResults = append(report.OpenResults, OpenResultSummary{
				State:           openResult.State,
				EventID:         proposal.Signal.EventID,
				LifecycleEvents: append([]turnarbiter.LifecycleEvent(nil), openResult.Events...),
				ControlSignals:  append([]eventabi.ControlSignal(nil), openResult.ControlLane...),
			})

		case EventIngressAudio, EventIngressData, EventIngressTelemetry:
			record, err := a.mapIngressEvent(ev)
			if err != nil {
				return report, err
			}
			report.IngressRecords = append(report.IngressRecords, record)

		case EventOutput:
			decision, err := a.fence.EvaluateOutput(runtimetransport.OutputAttempt{
				SessionID:            ev.SessionID,
				TurnID:               ev.TurnID,
				PipelineVersion:      ev.PipelineVersion,
				EventID:              ev.EventID,
				TransportSequence:    ev.TransportSequence,
				RuntimeSequence:      ev.RuntimeSequence,
				AuthorityEpoch:       ev.AuthorityEpoch,
				RuntimeTimestampMS:   ev.RuntimeTimestampMS,
				WallClockTimestampMS: ev.WallClockTimestampMS,
				CancelAccepted:       ev.CancelAccepted,
			})
			if err != nil {
				return report, err
			}
			report.ControlSignals = append(report.ControlSignals, decision.Signal)
			report.OutputDecisions = append(report.OutputDecisions, OutputDecisionSummary{
				EventID:   ev.EventID,
				Accepted:  decision.Accepted,
				Signal:    decision.Signal.Signal,
				Reason:    decision.Signal.Reason,
				EmittedBy: decision.Signal.EmittedBy,
			})

		case EventCancel:
			activeResult, err := a.arbiter.HandleActive(turnarbiter.ActiveInput{
				SessionID:            ev.SessionID,
				TurnID:               ev.TurnID,
				EventID:              ev.EventID,
				PipelineVersion:      ev.PipelineVersion,
				TransportSequence:    ev.TransportSequence,
				RuntimeSequence:      ev.RuntimeSequence,
				RuntimeTimestampMS:   ev.RuntimeTimestampMS,
				WallClockTimestampMS: ev.WallClockTimestampMS,
				AuthorityEpoch:       ev.AuthorityEpoch,
				CancelAccepted:       true,
			})
			if err != nil {
				return report, err
			}
			// Mark the deterministic transport cancel fence so subsequent output attempts are dropped.
			_, err = a.fence.EvaluateOutput(runtimetransport.OutputAttempt{
				SessionID:            ev.SessionID,
				TurnID:               ev.TurnID,
				PipelineVersion:      ev.PipelineVersion,
				EventID:              ev.EventID + "-cancel-fence",
				TransportSequence:    ev.TransportSequence,
				RuntimeSequence:      ev.RuntimeSequence,
				AuthorityEpoch:       ev.AuthorityEpoch,
				RuntimeTimestampMS:   ev.RuntimeTimestampMS,
				WallClockTimestampMS: ev.WallClockTimestampMS,
				CancelAccepted:       true,
			})
			if err != nil {
				return report, err
			}
			report.LifecycleEvents = append(report.LifecycleEvents, activeResult.Events...)
			report.ControlSignals = append(report.ControlSignals, activeResult.ControlLane...)
			report.ActiveResults = append(report.ActiveResults, ActiveResultSummary{
				State:           activeResult.State,
				EventID:         ev.EventID,
				LifecycleEvents: append([]turnarbiter.LifecycleEvent(nil), activeResult.Events...),
				ControlSignals:  append([]eventabi.ControlSignal(nil), activeResult.ControlLane...),
			})

		case EventAuthorityRevoked:
			activeResult, err := a.arbiter.HandleActive(turnarbiter.ActiveInput{
				SessionID:            ev.SessionID,
				TurnID:               ev.TurnID,
				EventID:              ev.EventID,
				PipelineVersion:      ev.PipelineVersion,
				TransportSequence:    ev.TransportSequence,
				RuntimeSequence:      ev.RuntimeSequence,
				RuntimeTimestampMS:   ev.RuntimeTimestampMS,
				WallClockTimestampMS: ev.WallClockTimestampMS,
				AuthorityEpoch:       ev.AuthorityEpoch,
				AuthorityRevoked:     true,
			})
			if err != nil {
				return report, err
			}
			report.LifecycleEvents = append(report.LifecycleEvents, activeResult.Events...)
			report.ControlSignals = append(report.ControlSignals, activeResult.ControlLane...)
			report.ActiveResults = append(report.ActiveResults, ActiveResultSummary{
				State:           activeResult.State,
				EventID:         ev.EventID,
				LifecycleEvents: append([]turnarbiter.LifecycleEvent(nil), activeResult.Events...),
				ControlSignals:  append([]eventabi.ControlSignal(nil), activeResult.ControlLane...),
			})

		case EventTransportDisconnect:
			activeResult, err := a.arbiter.HandleActive(turnarbiter.ActiveInput{
				SessionID:                  ev.SessionID,
				TurnID:                     ev.TurnID,
				EventID:                    ev.EventID,
				PipelineVersion:            ev.PipelineVersion,
				TransportSequence:          ev.TransportSequence,
				RuntimeSequence:            ev.RuntimeSequence,
				RuntimeTimestampMS:         ev.RuntimeTimestampMS,
				WallClockTimestampMS:       ev.WallClockTimestampMS,
				AuthorityEpoch:             ev.AuthorityEpoch,
				TransportDisconnectOrStall: true,
			})
			if err != nil {
				return report, err
			}
			report.LifecycleEvents = append(report.LifecycleEvents, activeResult.Events...)
			report.ControlSignals = append(report.ControlSignals, activeResult.ControlLane...)
			report.ActiveResults = append(report.ActiveResults, ActiveResultSummary{
				State:           activeResult.State,
				EventID:         ev.EventID,
				LifecycleEvents: append([]turnarbiter.LifecycleEvent(nil), activeResult.Events...),
				ControlSignals:  append([]eventabi.ControlSignal(nil), activeResult.ControlLane...),
			})

		case EventTerminalSuccess:
			activeResult, err := a.arbiter.HandleActive(turnarbiter.ActiveInput{
				SessionID:            ev.SessionID,
				TurnID:               ev.TurnID,
				EventID:              ev.EventID,
				PipelineVersion:      ev.PipelineVersion,
				TransportSequence:    ev.TransportSequence,
				RuntimeSequence:      ev.RuntimeSequence,
				RuntimeTimestampMS:   ev.RuntimeTimestampMS,
				WallClockTimestampMS: ev.WallClockTimestampMS,
				AuthorityEpoch:       ev.AuthorityEpoch,
				TerminalSuccessReady: true,
			})
			if err != nil {
				return report, err
			}
			report.LifecycleEvents = append(report.LifecycleEvents, activeResult.Events...)
			report.ControlSignals = append(report.ControlSignals, activeResult.ControlLane...)
			report.ActiveResults = append(report.ActiveResults, ActiveResultSummary{
				State:           activeResult.State,
				EventID:         ev.EventID,
				LifecycleEvents: append([]turnarbiter.LifecycleEvent(nil), activeResult.Events...),
				ControlSignals:  append([]eventabi.ControlSignal(nil), activeResult.ControlLane...),
			})

		default:
			return report, fmt.Errorf("unsupported livekit event kind %q", ev.Kind)
		}
		report.ProcessedEvents++
	}

	report.GeneratedAtUTC = a.now().UTC().Format(time.RFC3339)
	return report, nil
}

func (a Adapter) normalizeEvent(raw Event, idx int, transportSeq *int64, runtimeSeq *int64) (Event, error) {
	out := raw
	if out.SessionID == "" {
		out.SessionID = a.cfg.SessionID
	}
	if out.PipelineVersion == "" {
		out.PipelineVersion = a.cfg.PipelineVersion
	}
	if out.AuthorityEpoch == 0 {
		out.AuthorityEpoch = a.cfg.AuthorityEpoch
	}
	turnRequired := requiresTurnScope(out.Kind)
	if out.TurnID == "" && turnRequired {
		out.TurnID = a.cfg.TurnID
	}
	if out.EventID == "" {
		out.EventID = fmt.Sprintf("evt-livekit-%03d-%s", idx+1, out.Kind)
	}

	if out.TransportSequence <= 0 {
		*transportSeq = *transportSeq + 1
		out.TransportSequence = *transportSeq
	} else {
		*transportSeq = out.TransportSequence
	}
	if out.RuntimeSequence <= 0 {
		*runtimeSeq = *runtimeSeq + 1
		out.RuntimeSequence = *runtimeSeq
	} else {
		*runtimeSeq = out.RuntimeSequence
	}

	nowMS := a.now().UnixMilli()
	if out.RuntimeTimestampMS <= 0 {
		out.RuntimeTimestampMS = nowMS + int64(idx)
	}
	if out.WallClockTimestampMS <= 0 {
		out.WallClockTimestampMS = out.RuntimeTimestampMS
	}
	if out.Reason == "" {
		out.Reason = "livekit_" + string(out.Kind)
	}
	if out.SessionID == "" || out.PipelineVersion == "" || out.EventID == "" {
		return Event{}, fmt.Errorf("livekit event[%d] missing required envelope fields", idx)
	}
	if turnRequired && out.TurnID == "" {
		return Event{}, fmt.Errorf("livekit event[%d] %s requires turn_id", idx, out.Kind)
	}
	return out, nil
}

func (a Adapter) mapIngressEvent(ev Event) (eventabi.EventRecord, error) {
	lane := eventabi.LaneData
	switch ev.Kind {
	case EventIngressAudio, EventIngressData:
		lane = eventabi.LaneData
	case EventIngressTelemetry:
		lane = eventabi.LaneTelemetry
	}
	authorityEpoch := ev.AuthorityEpoch
	transportSequence := ev.TransportSequence

	record := eventabi.EventRecord{
		SchemaVersion:      "v1.0",
		EventScope:         eventabi.ScopeTurn,
		SessionID:          ev.SessionID,
		TurnID:             ev.TurnID,
		PipelineVersion:    ev.PipelineVersion,
		EventID:            ev.EventID,
		Lane:               lane,
		TransportSequence:  &transportSequence,
		RuntimeSequence:    ev.RuntimeSequence,
		AuthorityEpoch:     &authorityEpoch,
		RuntimeTimestampMS: ev.RuntimeTimestampMS,
		WallClockMS:        ev.WallClockTimestampMS,
		PayloadClass:       ev.PayloadClass,
	}
	if ev.Kind == EventIngressAudio {
		pts := ev.RuntimeTimestampMS
		record.MediaTime = &eventabi.MediaTime{PTSMS: &pts}
	}

	tagged, err := runtimetransport.TagIngressEventRecord(record, runtimetransport.IngressClassificationConfig{
		DefaultDataClass: a.cfg.DefaultDataClass,
	})
	if err != nil {
		return eventabi.EventRecord{}, err
	}
	normalized, err := runtimeeventabi.ValidateAndNormalizeEventRecords([]eventabi.EventRecord{tagged})
	if err != nil {
		return eventabi.EventRecord{}, err
	}
	return normalized[0], nil
}

func requiresTurnScope(kind EventKind) bool {
	switch kind {
	case EventTurnOpenProposed, EventIngressAudio, EventIngressData, EventIngressTelemetry, EventOutput, EventCancel, EventAuthorityRevoked, EventTransportDisconnect, EventTerminalSuccess:
		return true
	default:
		return false
	}
}

func boolOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

// DefaultScenario returns a deterministic transport-path flow used by runtime/operator commands.
func DefaultScenario(cfg Config) []Event {
	return []Event{
		{Kind: EventConnected, Reason: "livekit_connected"},
		{Kind: EventTurnOpenProposed, Reason: "livekit_turn_intent"},
		{Kind: EventIngressAudio},
		{Kind: EventOutput},
		{Kind: EventCancel},
		{Kind: EventOutput, Reason: "late_output_after_cancel"},
		{Kind: EventEnded, TurnID: "", Reason: "livekit_session_ended"},
	}
}
