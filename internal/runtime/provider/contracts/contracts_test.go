package contracts

import "testing"

func TestInvocationRequestValidate(t *testing.T) {
	t.Parallel()

	req := InvocationRequest{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           "stt-a",
		Modality:             ModalitySTT,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      2,
		AuthorityEpoch:       3,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	req.Attempt = 0
	if err := req.Validate(); err == nil {
		t.Fatalf("expected invalid attempt to fail validation")
	}
}

func TestOutcomeValidate(t *testing.T) {
	t.Parallel()

	success := Outcome{Class: OutcomeSuccess}
	if err := success.Validate(); err != nil {
		t.Fatalf("expected valid success outcome, got %v", err)
	}

	timeoutMissingReason := Outcome{
		Class:     OutcomeTimeout,
		Retryable: true,
	}
	if err := timeoutMissingReason.Validate(); err == nil {
		t.Fatalf("expected non-success outcome without reason to fail")
	}

	circuitOnSuccess := Outcome{
		Class:       OutcomeSuccess,
		CircuitOpen: true,
	}
	if err := circuitOnSuccess.Validate(); err == nil {
		t.Fatalf("expected success outcome with circuit_open=true to fail")
	}
}

func TestStaticAdapterDefaultInvoke(t *testing.T) {
	t.Parallel()

	adapter := StaticAdapter{
		ID:   "tts-a",
		Mode: ModalityTTS,
	}
	outcome, err := adapter.Invoke(InvocationRequest{
		SessionID:            "sess-2",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-2",
		ProviderInvocationID: "pvi-2",
		ProviderID:           "tts-a",
		Modality:             ModalityTTS,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   10,
		WallClockTimestampMS: 10,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if outcome.Class != OutcomeSuccess {
		t.Fatalf("expected default static adapter success, got %s", outcome.Class)
	}
}
