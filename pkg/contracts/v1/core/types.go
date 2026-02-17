package core

import (
	cp "github.com/tiger/realtime-speech-pipeline/api/controlplane"
	ev "github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	tr "github.com/tiger/realtime-speech-pipeline/api/transport"
)

// Event contracts.
type EventRecord = ev.EventRecord
type Lane = ev.Lane
type ControlSignal = ev.ControlSignal

// Control-plane contracts.
type DecisionOutcome = cp.DecisionOutcome
type ResolvedTurnPlan = cp.ResolvedTurnPlan
type SnapshotProvenance = cp.SnapshotProvenance
type OutcomeKind = cp.OutcomeKind
type OutcomeScope = cp.OutcomeScope

// Observability contracts.
type ReplayMode = obs.ReplayMode
type ReplayCursor = obs.ReplayCursor
type ReplayRunRequest = obs.ReplayRunRequest
type ReplayRunResult = obs.ReplayRunResult
type ReplayRunReport = obs.ReplayRunReport
type ReplayDivergence = obs.ReplayDivergence
type ReplayAccessDecision = obs.ReplayAccessDecision

// Transport contracts.
type SessionBootstrap = tr.SessionBootstrap
type AdapterCapabilities = tr.AdapterCapabilities
type TransportKind = tr.TransportKind
type ConnectionSignal = tr.ConnectionSignal
