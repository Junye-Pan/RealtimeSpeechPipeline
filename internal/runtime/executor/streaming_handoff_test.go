package executor

import (
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/localadmission"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/invocation"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/registry"
)

func TestExecuteStreamingChainOverlapStartsDownstreamEarly(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{
			ID:   "stt-a",
			Mode: contracts.ModalitySTT,
			InvokeStreamFn: func(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
				start := newChunk(req, contracts.StreamChunkStart, 0, "", nil)
				if err := observer.OnStart(start); err != nil {
					return contracts.Outcome{}, err
				}

				partial := newChunk(req, contracts.StreamChunkDelta, 1, "hello world ", nil)
				if err := observer.OnChunk(partial); err != nil {
					return contracts.Outcome{}, err
				}
				time.Sleep(30 * time.Millisecond)

				final := newChunk(req, contracts.StreamChunkFinal, 2, "hello world from stt", nil)
				if err := observer.OnComplete(final); err != nil {
					return contracts.Outcome{}, err
				}
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
		contracts.StaticAdapter{
			ID:   "llm-a",
			Mode: contracts.ModalityLLM,
			InvokeStreamFn: func(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
				start := newChunk(req, contracts.StreamChunkStart, 0, "", nil)
				if err := observer.OnStart(start); err != nil {
					return contracts.Outcome{}, err
				}
				delta := newChunk(req, contracts.StreamChunkDelta, 1, "ok ", nil)
				if err := observer.OnChunk(delta); err != nil {
					return contracts.Outcome{}, err
				}
				time.Sleep(25 * time.Millisecond)
				final := newChunk(req, contracts.StreamChunkFinal, 2, "ok done", nil)
				if err := observer.OnComplete(final); err != nil {
					return contracts.Outcome{}, err
				}
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
		contracts.StaticAdapter{
			ID:   "tts-a",
			Mode: contracts.ModalityTTS,
			InvokeStreamFn: func(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
				start := newChunk(req, contracts.StreamChunkStart, 0, "", nil)
				if err := observer.OnStart(start); err != nil {
					return contracts.Outcome{}, err
				}
				audio := []byte{1, 2, 3, 4}
				audioChunk := newChunk(req, contracts.StreamChunkAudio, 1, "", audio)
				if err := observer.OnChunk(audioChunk); err != nil {
					return contracts.Outcome{}, err
				}
				time.Sleep(10 * time.Millisecond)
				final := newChunk(req, contracts.StreamChunkFinal, 2, "", nil)
				if err := observer.OnComplete(final); err != nil {
					return contracts.Outcome{}, err
				}
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
			InvokeFn: func(req contracts.InvocationRequest) (contracts.Outcome, error) {
				return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invocation.NewController(catalog))
	result, err := scheduler.ExecuteStreamingChain(
		SchedulingInput{
			SessionID:            "sess-stream-chain-1",
			TurnID:               "turn-stream-chain-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-stream-chain-1",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   time.Now().UnixMilli(),
			WallClockTimestampMS: time.Now().UnixMilli(),
		},
		ProviderInvocationInput{Modality: contracts.ModalitySTT, PreferredProvider: "stt-a"},
		ProviderInvocationInput{Modality: contracts.ModalityLLM, PreferredProvider: "llm-a"},
		ProviderInvocationInput{Modality: contracts.ModalityTTS, PreferredProvider: "tts-a"},
		StreamingHandoffPolicy{
			Enabled:             true,
			STTToLLMEnabled:     true,
			LLMToTTSEnabled:     true,
			MinPartialChars:     1,
			MaxPendingRevisions: 4,
			CoalesceLatestOnly:  true,
		},
	)
	if err != nil {
		t.Fatalf("unexpected streaming chain error: %v", err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected pass status, got %+v", result)
	}
	if result.LLM.StartedAtMS == 0 || result.STT.CompletedAtMS == 0 || result.LLM.StartedAtMS >= result.STT.CompletedAtMS {
		t.Fatalf("expected LLM to start before STT final completion, got stt_complete=%d llm_started=%d", result.STT.CompletedAtMS, result.LLM.StartedAtMS)
	}
	if result.TTS.StartedAtMS == 0 || result.LLM.CompletedAtMS == 0 || result.TTS.StartedAtMS >= result.LLM.CompletedAtMS {
		t.Fatalf("expected TTS to start before LLM final completion, got llm_complete=%d tts_started=%d", result.LLM.CompletedAtMS, result.TTS.StartedAtMS)
	}
	if result.Latency.TurnCompletionE2EMS <= 0 {
		t.Fatalf("expected turn completion e2e latency > 0, got %+v", result.Latency)
	}
	if len(result.Handoffs) < 2 {
		t.Fatalf("expected handoff evidence for two edges, got %+v", result.Handoffs)
	}
}

func TestExecuteStreamingChainSequentialFallbackWhenDisabled(t *testing.T) {
	t.Parallel()

	catalog, err := registry.NewCatalog([]contracts.Adapter{
		contracts.StaticAdapter{ID: "stt-a", Mode: contracts.ModalitySTT},
		contracts.StaticAdapter{ID: "llm-a", Mode: contracts.ModalityLLM},
		contracts.StaticAdapter{ID: "tts-a", Mode: contracts.ModalityTTS},
	})
	if err != nil {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	scheduler := NewSchedulerWithProviderInvoker(localadmission.Evaluator{}, invocation.NewController(catalog))
	result, err := scheduler.ExecuteStreamingChain(
		SchedulingInput{
			SessionID:            "sess-stream-chain-seq-1",
			TurnID:               "turn-stream-chain-seq-1",
			PipelineVersion:      "pipeline-v1",
			EventID:              "evt-stream-chain-seq-1",
			TransportSequence:    1,
			RuntimeSequence:      1,
			AuthorityEpoch:       1,
			RuntimeTimestampMS:   time.Now().UnixMilli(),
			WallClockTimestampMS: time.Now().UnixMilli(),
		},
		ProviderInvocationInput{Modality: contracts.ModalitySTT, PreferredProvider: "stt-a"},
		ProviderInvocationInput{Modality: contracts.ModalityLLM, PreferredProvider: "llm-a"},
		ProviderInvocationInput{Modality: contracts.ModalityTTS, PreferredProvider: "tts-a"},
		StreamingHandoffPolicy{Enabled: false},
	)
	if err != nil {
		t.Fatalf("unexpected sequential fallback error: %v", err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected pass status, got %+v", result)
	}
	if len(result.Handoffs) != 0 {
		t.Fatalf("expected no handoff entries when policy disabled, got %+v", result.Handoffs)
	}
}

func TestHandoffBackpressureSignalsSaturationAndRecovery(t *testing.T) {
	t.Parallel()

	in := SchedulingInput{
		SessionID:            "sess-stream-bp-1",
		TurnID:               "turn-stream-bp-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-stream-bp-1",
		TransportSequence:    10,
		RuntimeSequence:      11,
		AuthorityEpoch:       2,
		RuntimeTimestampMS:   100,
		WallClockTimestampMS: 100,
	}

	saturated := false
	signals, next, err := handoffBackpressureSignals(in, in.EventID, "stt_to_llm", true, saturated, 0)
	if err != nil {
		t.Fatalf("unexpected xoff signal error: %v", err)
	}
	if len(signals) != 1 || signals[0].Signal != "flow_xoff" {
		t.Fatalf("expected one flow_xoff signal, got %+v", signals)
	}
	if signals[0].EdgeID != "stt_to_llm" || signals[0].TargetLane != eventabi.LaneData {
		t.Fatalf("unexpected signal routing metadata: %+v", signals[0])
	}
	saturated = next

	signals, next, err = handoffBackpressureSignals(in, in.EventID, "stt_to_llm", true, saturated, 1)
	if err != nil {
		t.Fatalf("unexpected repeated saturation error: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected no duplicate flow_xoff while saturated, got %+v", signals)
	}
	if !next {
		t.Fatalf("expected saturation state to remain true")
	}
	saturated = next

	signals, next, err = handoffBackpressureSignals(in, in.EventID, "stt_to_llm", false, saturated, 1)
	if err != nil {
		t.Fatalf("unexpected xon recovery error: %v", err)
	}
	if len(signals) != 1 || signals[0].Signal != "flow_xon" {
		t.Fatalf("expected one flow_xon signal, got %+v", signals)
	}
	if next {
		t.Fatalf("expected saturation state to clear on recovery")
	}
}

func TestHandoffBackpressureSignalsRejectInvalidEdge(t *testing.T) {
	t.Parallel()

	_, _, err := handoffBackpressureSignals(
		SchedulingInput{
			SessionID:       "sess-stream-bp-invalid",
			PipelineVersion: "pipeline-v1",
		},
		"evt-stream-bp-invalid",
		"stt_to_unknown",
		true,
		false,
		0,
	)
	if err == nil {
		t.Fatalf("expected invalid handoff edge to fail")
	}
}

func newChunk(req contracts.InvocationRequest, kind contracts.StreamChunkKind, sequence int, text string, audio []byte) contracts.StreamChunk {
	chunk := contracts.StreamChunk{
		SessionID:            req.SessionID,
		TurnID:               req.TurnID,
		PipelineVersion:      req.PipelineVersion,
		EventID:              req.EventID,
		ProviderInvocationID: req.ProviderInvocationID,
		ProviderID:           req.ProviderID,
		Modality:             req.Modality,
		Attempt:              req.Attempt,
		Sequence:             sequence,
		RuntimeTimestampMS:   req.RuntimeTimestampMS,
		WallClockTimestampMS: req.WallClockTimestampMS,
		Kind:                 kind,
		TextDelta:            text,
		TextFinal:            text,
		AudioBytes:           audio,
	}
	if kind == contracts.StreamChunkStart {
		chunk.TextDelta = ""
		chunk.TextFinal = ""
	}
	if kind == contracts.StreamChunkAudio {
		chunk.TextDelta = ""
		chunk.TextFinal = ""
	}
	return chunk
}
