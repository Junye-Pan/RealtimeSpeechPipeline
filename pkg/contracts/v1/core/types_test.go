package core

import (
	"testing"

	cp "github.com/tiger/realtime-speech-pipeline/api/controlplane"
	ev "github.com/tiger/realtime-speech-pipeline/api/eventabi"
	obs "github.com/tiger/realtime-speech-pipeline/api/observability"
	tr "github.com/tiger/realtime-speech-pipeline/api/transport"
)

func TestFacadeTypeAliasesMatchCanonicalContracts(t *testing.T) {
	t.Parallel()

	var _ EventRecord = ev.EventRecord{}
	var _ Lane = ev.LaneData
	var _ ControlSignal = ev.ControlSignal{}
	var _ DecisionOutcome = cp.DecisionOutcome{}
	var _ ResolvedTurnPlan = cp.ResolvedTurnPlan{}
	var _ SnapshotProvenance = cp.SnapshotProvenance{}
	var _ OutcomeKind = cp.OutcomeAdmit
	var _ OutcomeScope = cp.ScopeTurn
	var _ ReplayMode = obs.ReplayModeReSimulateNodes
	var _ ReplayCursor = obs.ReplayCursor{}
	var _ ReplayRunRequest = obs.ReplayRunRequest{}
	var _ ReplayRunResult = obs.ReplayRunResult{}
	var _ ReplayRunReport = obs.ReplayRunReport{}
	var _ ReplayDivergence = obs.ReplayDivergence{}
	var _ ReplayAccessDecision = obs.ReplayAccessDecision{}
	var _ SessionBootstrap = tr.SessionBootstrap{}
	var _ AdapterCapabilities = tr.AdapterCapabilities{}
	var _ TransportKind = tr.TransportLiveKit
	var _ ConnectionSignal = tr.SignalConnected
}

func TestTransportFacadeValidators(t *testing.T) {
	t.Parallel()

	bootstrap := SessionBootstrap{
		SchemaVersion:   "v1.0",
		TransportKind:   TransportKind(tr.TransportLiveKit),
		TenantID:        "tenant-a",
		SessionID:       "session-a",
		PipelineVersion: "pipeline-v1",
		AuthorityEpoch:  1,
		LeaseTokenRef:   "lease://token",
		RouteRef:        "route://edge-a",
		RequestedAtMS:   1,
	}
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("expected bootstrap validation to pass, got %v", err)
	}

	capabilities := AdapterCapabilities{
		TransportKind:              TransportKind(tr.TransportLiveKit),
		SupportedConnectionSignals: []ConnectionSignal{ConnectionSignal(tr.SignalConnected)},
		SupportsIngressAudio:       true,
		SupportsIngressControl:     true,
		SupportsEgressAudio:        true,
		SupportsReconnectSemantics: true,
	}
	if err := capabilities.Validate(); err != nil {
		t.Fatalf("expected capability validation to pass, got %v", err)
	}
}
