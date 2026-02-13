package contracts

import "testing"

func TestInvocationRequestValidate(t *testing.T) {
	t.Parallel()

	req := InvocationRequest{
		SessionID:              "sess-1",
		TurnID:                 "turn-1",
		PipelineVersion:        "pipeline-v1",
		EventID:                "evt-1",
		ProviderInvocationID:   "pvi-1",
		ProviderID:             "stt-a",
		Modality:               ModalitySTT,
		Attempt:                1,
		TransportSequence:      1,
		RuntimeSequence:        2,
		AuthorityEpoch:         3,
		RuntimeTimestampMS:     100,
		WallClockTimestampMS:   100,
		AllowedAdaptiveActions: []string{"retry", "provider_switch"},
		RetryBudgetRemaining:   1,
		CandidateProviderCount: 3,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	req.Attempt = 0
	if err := req.Validate(); err == nil {
		t.Fatalf("expected invalid attempt to fail validation")
	}

	req.Attempt = 1
	req.AllowedAdaptiveActions = []string{"retry", "retry"}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected duplicate adaptive action to fail validation")
	}

	req.AllowedAdaptiveActions = []string{"unsupported_action"}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected unsupported adaptive action to fail validation")
	}

	req.AllowedAdaptiveActions = []string{"retry"}
	req.RetryBudgetRemaining = -1
	if err := req.Validate(); err == nil {
		t.Fatalf("expected negative retry budget to fail validation")
	}

	req.RetryBudgetRemaining = 0
	req.CandidateProviderCount = -1
	if err := req.Validate(); err == nil {
		t.Fatalf("expected negative candidate provider count to fail validation")
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
		SessionID:              "sess-2",
		PipelineVersion:        "pipeline-v1",
		EventID:                "evt-2",
		ProviderInvocationID:   "pvi-2",
		ProviderID:             "tts-a",
		Modality:               ModalityTTS,
		Attempt:                1,
		TransportSequence:      1,
		RuntimeSequence:        1,
		AuthorityEpoch:         1,
		RuntimeTimestampMS:     10,
		WallClockTimestampMS:   10,
		AllowedAdaptiveActions: []string{"retry"},
		RetryBudgetRemaining:   0,
		CandidateProviderCount: 1,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if outcome.Class != OutcomeSuccess {
		t.Fatalf("expected default static adapter success, got %s", outcome.Class)
	}
}

func TestNormalizeAdaptiveActions(t *testing.T) {
	t.Parallel()

	normalized, err := NormalizeAdaptiveActions([]string{"provider_switch", "retry", "fallback"})
	if err != nil {
		t.Fatalf("expected normalize success, got %v", err)
	}
	if len(normalized) != 3 {
		t.Fatalf("expected 3 normalized actions, got %d", len(normalized))
	}
	if normalized[0] != "fallback" || normalized[1] != "provider_switch" || normalized[2] != "retry" {
		t.Fatalf("expected sorted actions, got %+v", normalized)
	}
}

type captureObserver struct {
	startCount    int
	chunkCount    int
	completeCount int
	errorCount    int
}

func (o *captureObserver) OnStart(StreamChunk) error {
	o.startCount++
	return nil
}

func (o *captureObserver) OnChunk(StreamChunk) error {
	o.chunkCount++
	return nil
}

func (o *captureObserver) OnComplete(StreamChunk) error {
	o.completeCount++
	return nil
}

func (o *captureObserver) OnError(StreamChunk) error {
	o.errorCount++
	return nil
}

func TestStreamChunkValidate(t *testing.T) {
	t.Parallel()

	valid := StreamChunk{
		SessionID:            "sess-1",
		TurnID:               "turn-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           "llm-a",
		Modality:             ModalityLLM,
		Attempt:              1,
		Sequence:             0,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
		Kind:                 StreamChunkFinal,
		TextFinal:            "ok",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid stream chunk, got %v", err)
	}

	invalid := valid
	invalid.Kind = StreamChunkAudio
	invalid.AudioBytes = nil
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected audio chunk without bytes to fail")
	}

	invalid = valid
	invalid.Kind = StreamChunkError
	invalid.ErrorReason = ""
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected error chunk without reason to fail")
	}
}

func TestStaticAdapterDefaultInvokeStream(t *testing.T) {
	t.Parallel()

	adapter := StaticAdapter{
		ID:   "llm-a",
		Mode: ModalityLLM,
	}
	observer := &captureObserver{}
	outcome, err := adapter.InvokeStream(InvocationRequest{
		SessionID:              "sess-3",
		TurnID:                 "turn-3",
		PipelineVersion:        "pipeline-v1",
		EventID:                "evt-3",
		ProviderInvocationID:   "pvi-3",
		ProviderID:             "llm-a",
		Modality:               ModalityLLM,
		Attempt:                1,
		TransportSequence:      1,
		RuntimeSequence:        1,
		AuthorityEpoch:         1,
		RuntimeTimestampMS:     1,
		WallClockTimestampMS:   1,
		AllowedAdaptiveActions: []string{"retry"},
		RetryBudgetRemaining:   0,
		CandidateProviderCount: 1,
	}, observer)
	if err != nil {
		t.Fatalf("unexpected stream invoke error: %v", err)
	}
	if outcome.Class != OutcomeSuccess {
		t.Fatalf("expected stream success, got %s", outcome.Class)
	}
	if observer.startCount != 1 || observer.completeCount != 1 || observer.errorCount != 0 {
		t.Fatalf("unexpected observer counts: start=%d chunk=%d complete=%d error=%d", observer.startCount, observer.chunkCount, observer.completeCount, observer.errorCount)
	}
}
