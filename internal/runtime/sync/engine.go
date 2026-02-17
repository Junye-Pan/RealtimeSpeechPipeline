package sync

import (
	"fmt"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/buffering"
)

// Policy defines synchronized stream loss handling behavior.
type Policy string

const (
	// PolicyAtomicDrop emits an atomic drop marker for the affected range.
	PolicyAtomicDrop Policy = "atomic_drop"
	// PolicyDropWithDiscontinuity emits drop_notice and discontinuity markers.
	PolicyDropWithDiscontinuity Policy = "drop_with_discontinuity"
)

// Input defines sync-integrity evaluation context.
type Input struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	TransportSequence    int64
	RuntimeSequence      int64
	AuthorityEpoch       int64
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64

	EdgeID         string
	TargetLane     eventabi.Lane
	DropRangeStart int64
	DropRangeEnd   int64

	SyncDomain      string
	DiscontinuityID string
	Policy          Policy
}

// Result contains deterministic sync-integrity control markers.
type Result struct {
	Signals []eventabi.ControlSignal
}

// Engine applies sync-integrity policy decisions.
type Engine struct{}

// NewEngine constructs a sync-integrity engine.
func NewEngine() Engine {
	return Engine{}
}

// Evaluate applies one sync policy decision to an input event.
func (Engine) Evaluate(in Input) (Result, error) {
	if in.SessionID == "" || in.PipelineVersion == "" || in.EventID == "" {
		return Result{}, fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	if in.EdgeID == "" {
		return Result{}, fmt.Errorf("edge_id is required")
	}
	if in.TargetLane == "" {
		return Result{}, fmt.Errorf("target_lane is required")
	}

	switch in.Policy {
	case PolicyAtomicDrop:
		drop, err := buildDropSignal(in, "sync_atomic_drop")
		if err != nil {
			return Result{}, err
		}
		return Result{Signals: []eventabi.ControlSignal{drop}}, nil
	case PolicyDropWithDiscontinuity:
		if in.SyncDomain == "" || in.DiscontinuityID == "" {
			return Result{}, fmt.Errorf("sync_domain and discontinuity_id are required for drop_with_discontinuity")
		}
		drop, err := buildDropSignal(in, "sync_drop_with_discontinuity")
		if err != nil {
			return Result{}, err
		}
		discontinuity, err := buildDiscontinuitySignal(in)
		if err != nil {
			return Result{}, err
		}
		return Result{Signals: []eventabi.ControlSignal{drop, discontinuity}}, nil
	default:
		return Result{}, fmt.Errorf("unsupported sync policy %q", in.Policy)
	}
}

func buildDropSignal(in Input, reason string) (eventabi.ControlSignal, error) {
	return buffering.BuildDropNotice(buffering.DropNoticeInput{
		SessionID:            in.SessionID,
		TurnID:               in.TurnID,
		PipelineVersion:      in.PipelineVersion,
		EdgeID:               in.EdgeID,
		EventID:              in.EventID + "-drop",
		TransportSequence:    in.TransportSequence,
		RuntimeSequence:      in.RuntimeSequence,
		AuthorityEpoch:       in.AuthorityEpoch,
		RuntimeTimestampMS:   in.RuntimeTimestampMS,
		WallClockTimestampMS: in.WallClockTimestampMS,
		TargetLane:           in.TargetLane,
		RangeStart:           in.DropRangeStart,
		RangeEnd:             in.DropRangeEnd,
		Reason:               reason,
		EmittedBy:            "RK-12",
	})
}

func buildDiscontinuitySignal(in Input) (eventabi.ControlSignal, error) {
	eventScope := eventabi.ScopeSession
	scope := "session"
	if in.TurnID != "" {
		eventScope = eventabi.ScopeTurn
		scope = "turn"
	}
	transport := in.TransportSequence
	if transport < 0 {
		transport = 0
	}
	runtimeSeq := in.RuntimeSequence
	if runtimeSeq < 0 {
		runtimeSeq = 0
	}
	authorityEpoch := in.AuthorityEpoch
	if authorityEpoch < 0 {
		authorityEpoch = 0
	}
	runtimeTS := in.RuntimeTimestampMS
	if runtimeTS < 0 {
		runtimeTS = 0
	}
	wallTS := in.WallClockTimestampMS
	if wallTS < 0 {
		wallTS = 0
	}

	sig := eventabi.ControlSignal{
		SchemaVersion:      "v1.0",
		EventScope:         eventScope,
		SessionID:          in.SessionID,
		TurnID:             in.TurnID,
		PipelineVersion:    in.PipelineVersion,
		SyncDomain:         in.SyncDomain,
		DiscontinuityID:    in.DiscontinuityID,
		EventID:            in.EventID + "-discontinuity",
		Lane:               eventabi.LaneControl,
		TransportSequence:  &transport,
		RuntimeSequence:    runtimeSeq,
		AuthorityEpoch:     authorityEpoch,
		RuntimeTimestampMS: runtimeTS,
		WallClockMS:        wallTS,
		PayloadClass:       eventabi.PayloadMetadata,
		Signal:             "discontinuity",
		EmittedBy:          "RK-15",
		Reason:             "sync_domain_partial_loss",
		Scope:              scope,
	}
	if err := sig.Validate(); err != nil {
		return eventabi.ControlSignal{}, err
	}
	return sig, nil
}
