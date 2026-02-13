package contracts

import (
	"fmt"
	"sort"
)

// Modality defines provider families supported by runtime invocation.
type Modality string

const (
	ModalitySTT Modality = "stt"
	ModalityLLM Modality = "llm"
	ModalityTTS Modality = "tts"
)

// Validate enforces supported provider modality values.
func (m Modality) Validate() error {
	switch m {
	case ModalitySTT, ModalityLLM, ModalityTTS:
		return nil
	default:
		return fmt.Errorf("unsupported modality: %q", m)
	}
}

// OutcomeClass is the normalized invocation-outcome taxonomy.
type OutcomeClass string

const (
	OutcomeSuccess               OutcomeClass = "success"
	OutcomeTimeout               OutcomeClass = "timeout"
	OutcomeOverload              OutcomeClass = "overload"
	OutcomeBlocked               OutcomeClass = "blocked"
	OutcomeInfrastructureFailure OutcomeClass = "infrastructure_failure"
	OutcomeCancelled             OutcomeClass = "cancelled"
)

// Validate enforces supported outcome classes.
func (o OutcomeClass) Validate() error {
	switch o {
	case OutcomeSuccess, OutcomeTimeout, OutcomeOverload, OutcomeBlocked, OutcomeInfrastructureFailure, OutcomeCancelled:
		return nil
	default:
		return fmt.Errorf("unsupported outcome_class: %q", o)
	}
}

// StreamChunkKind classifies streaming events emitted by provider adapters.
type StreamChunkKind string

const (
	StreamChunkStart    StreamChunkKind = "start"
	StreamChunkDelta    StreamChunkKind = "delta"
	StreamChunkFinal    StreamChunkKind = "final"
	StreamChunkAudio    StreamChunkKind = "audio"
	StreamChunkMetadata StreamChunkKind = "metadata"
	StreamChunkError    StreamChunkKind = "error"
)

// Validate enforces supported stream chunk kinds.
func (k StreamChunkKind) Validate() error {
	switch k {
	case StreamChunkStart, StreamChunkDelta, StreamChunkFinal, StreamChunkAudio, StreamChunkMetadata, StreamChunkError:
		return nil
	default:
		return fmt.Errorf("unsupported stream_chunk_kind: %q", k)
	}
}

// StreamChunk captures one streaming emission from a provider invocation.
type StreamChunk struct {
	SessionID            string
	TurnID               string
	PipelineVersion      string
	EventID              string
	ProviderInvocationID string
	ProviderID           string
	Modality             Modality
	Attempt              int
	Sequence             int
	RuntimeTimestampMS   int64
	WallClockTimestampMS int64
	Kind                 StreamChunkKind
	TextDelta            string
	TextFinal            string
	AudioBytes           []byte
	MimeType             string
	Metadata             map[string]string
	ErrorReason          string
}

// Validate enforces deterministic stream chunk invariants.
func (c StreamChunk) Validate() error {
	if c.SessionID == "" || c.PipelineVersion == "" || c.EventID == "" {
		return fmt.Errorf("stream chunk requires session_id, pipeline_version, and event_id")
	}
	if c.ProviderInvocationID == "" || c.ProviderID == "" {
		return fmt.Errorf("stream chunk requires provider_invocation_id and provider_id")
	}
	if err := c.Modality.Validate(); err != nil {
		return err
	}
	if c.Attempt < 1 {
		return fmt.Errorf("stream chunk attempt must be >=1")
	}
	if c.Sequence < 0 {
		return fmt.Errorf("stream chunk sequence must be >=0")
	}
	if c.RuntimeTimestampMS < 0 || c.WallClockTimestampMS < 0 {
		return fmt.Errorf("stream chunk timestamps must be >=0")
	}
	if err := c.Kind.Validate(); err != nil {
		return err
	}
	if c.Kind == StreamChunkAudio && len(c.AudioBytes) == 0 {
		return fmt.Errorf("stream chunk audio requires non-empty audio bytes")
	}
	if c.Kind == StreamChunkError && c.ErrorReason == "" {
		return fmt.Errorf("stream chunk error requires error_reason")
	}
	return nil
}

// StreamObserver consumes ordered stream chunks for one provider invocation.
type StreamObserver interface {
	OnStart(StreamChunk) error
	OnChunk(StreamChunk) error
	OnComplete(StreamChunk) error
	OnError(StreamChunk) error
}

// NoopStreamObserver is a no-op stream observer implementation.
type NoopStreamObserver struct{}

func (NoopStreamObserver) OnStart(StreamChunk) error    { return nil }
func (NoopStreamObserver) OnChunk(StreamChunk) error    { return nil }
func (NoopStreamObserver) OnComplete(StreamChunk) error { return nil }
func (NoopStreamObserver) OnError(StreamChunk) error    { return nil }

// StreamObserverFuncs adapts callback functions to StreamObserver.
type StreamObserverFuncs struct {
	OnStartFn    func(StreamChunk) error
	OnChunkFn    func(StreamChunk) error
	OnCompleteFn func(StreamChunk) error
	OnErrorFn    func(StreamChunk) error
}

func (f StreamObserverFuncs) OnStart(chunk StreamChunk) error {
	if f.OnStartFn == nil {
		return nil
	}
	return f.OnStartFn(chunk)
}

func (f StreamObserverFuncs) OnChunk(chunk StreamChunk) error {
	if f.OnChunkFn == nil {
		return nil
	}
	return f.OnChunkFn(chunk)
}

func (f StreamObserverFuncs) OnComplete(chunk StreamChunk) error {
	if f.OnCompleteFn == nil {
		return nil
	}
	return f.OnCompleteFn(chunk)
}

func (f StreamObserverFuncs) OnError(chunk StreamChunk) error {
	if f.OnErrorFn == nil {
		return nil
	}
	return f.OnErrorFn(chunk)
}

// InvocationRequest is passed to adapter implementations per attempt.
type InvocationRequest struct {
	SessionID              string
	TurnID                 string
	PipelineVersion        string
	EventID                string
	ProviderInvocationID   string
	ProviderID             string
	Modality               Modality
	Attempt                int
	TransportSequence      int64
	RuntimeSequence        int64
	AuthorityEpoch         int64
	RuntimeTimestampMS     int64
	WallClockTimestampMS   int64
	CancelRequested        bool
	AllowedAdaptiveActions []string
	RetryBudgetRemaining   int
	CandidateProviderCount int
}

// Validate enforces deterministic required fields.
func (r InvocationRequest) Validate() error {
	if r.SessionID == "" || r.PipelineVersion == "" || r.EventID == "" {
		return fmt.Errorf("session_id, pipeline_version, and event_id are required")
	}
	if r.ProviderInvocationID == "" || r.ProviderID == "" {
		return fmt.Errorf("provider_invocation_id and provider_id are required")
	}
	if err := r.Modality.Validate(); err != nil {
		return err
	}
	if r.Attempt < 1 {
		return fmt.Errorf("attempt must be >=1")
	}
	if r.TransportSequence < 0 || r.RuntimeSequence < 0 || r.AuthorityEpoch < 0 {
		return fmt.Errorf("sequence and authority values must be >=0")
	}
	if r.RuntimeTimestampMS < 0 || r.WallClockTimestampMS < 0 {
		return fmt.Errorf("timestamps must be >=0")
	}
	if _, err := NormalizeAdaptiveActions(r.AllowedAdaptiveActions); err != nil {
		return err
	}
	if r.RetryBudgetRemaining < 0 {
		return fmt.Errorf("retry_budget_remaining must be >=0")
	}
	if r.CandidateProviderCount < 0 {
		return fmt.Errorf("candidate_provider_count must be >=0")
	}
	return nil
}

// NormalizeAdaptiveActions validates and returns sorted unique adaptive actions.
func NormalizeAdaptiveActions(actions []string) ([]string, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action {
		case "retry", "provider_switch", "fallback":
			if _, exists := seen[action]; exists {
				return nil, fmt.Errorf("duplicate adaptive action: %q", action)
			}
			seen[action] = struct{}{}
			out = append(out, action)
		default:
			return nil, fmt.Errorf("unsupported adaptive action: %q", action)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Outcome is an adapter-normalized invocation result.
type Outcome struct {
	Class            OutcomeClass
	Retryable        bool
	Reason           string
	CircuitOpen      bool
	BackoffMS        int64
	InputPayload     string
	OutputPayload    string
	OutputStatusCode int
	PayloadTruncated bool
}

// Validate enforces normalized outcome invariants.
func (o Outcome) Validate() error {
	if err := o.Class.Validate(); err != nil {
		return err
	}
	if o.Class != OutcomeSuccess && o.Reason == "" {
		return fmt.Errorf("reason is required for non-success outcomes")
	}
	if o.BackoffMS < 0 {
		return fmt.Errorf("backoff_ms must be >=0")
	}
	if o.CircuitOpen && o.Class == OutcomeSuccess {
		return fmt.Errorf("circuit_open cannot be true for success")
	}
	if o.OutputStatusCode < 0 {
		return fmt.Errorf("output_status_code must be >=0")
	}
	return nil
}

// Adapter defines RK-10 provider adapter behavior.
type Adapter interface {
	ProviderID() string
	Modality() Modality
	Invoke(InvocationRequest) (Outcome, error)
}

// StreamingAdapter extends Adapter with streaming invocation support.
type StreamingAdapter interface {
	Adapter
	InvokeStream(InvocationRequest, StreamObserver) (Outcome, error)
}

// StaticAdapter is a small utility adapter for tests and static catalogs.
type StaticAdapter struct {
	ID             string
	Mode           Modality
	InvokeFn       func(InvocationRequest) (Outcome, error)
	InvokeStreamFn func(InvocationRequest, StreamObserver) (Outcome, error)
}

func (a StaticAdapter) ProviderID() string {
	return a.ID
}

func (a StaticAdapter) Modality() Modality {
	return a.Mode
}

func (a StaticAdapter) Invoke(req InvocationRequest) (Outcome, error) {
	if a.InvokeFn != nil {
		return a.InvokeFn(req)
	}
	if err := req.Validate(); err != nil {
		return Outcome{}, err
	}
	return Outcome{Class: OutcomeSuccess}, nil
}

func (a StaticAdapter) InvokeStream(req InvocationRequest, observer StreamObserver) (Outcome, error) {
	if a.InvokeStreamFn != nil {
		return a.InvokeStreamFn(req, observer)
	}
	if observer == nil {
		observer = NoopStreamObserver{}
	}
	start := StreamChunk{
		SessionID:            req.SessionID,
		TurnID:               req.TurnID,
		PipelineVersion:      req.PipelineVersion,
		EventID:              req.EventID,
		ProviderInvocationID: req.ProviderInvocationID,
		ProviderID:           req.ProviderID,
		Modality:             req.Modality,
		Attempt:              req.Attempt,
		Sequence:             0,
		RuntimeTimestampMS:   req.RuntimeTimestampMS,
		WallClockTimestampMS: req.WallClockTimestampMS,
		Kind:                 StreamChunkStart,
	}
	if err := start.Validate(); err != nil {
		return Outcome{}, err
	}
	if err := observer.OnStart(start); err != nil {
		return Outcome{}, err
	}

	outcome, err := a.Invoke(req)
	if err != nil {
		return Outcome{}, err
	}
	if err := outcome.Validate(); err != nil {
		return Outcome{}, err
	}
	if outcome.Class != OutcomeSuccess {
		errorChunk := start
		errorChunk.Sequence = 1
		errorChunk.Kind = StreamChunkError
		errorChunk.ErrorReason = outcome.Reason
		if errorChunk.ErrorReason == "" {
			errorChunk.ErrorReason = "provider_stream_error"
		}
		if err := errorChunk.Validate(); err != nil {
			return Outcome{}, err
		}
		if err := observer.OnError(errorChunk); err != nil {
			return Outcome{}, err
		}
		return outcome, nil
	}

	final := start
	final.Sequence = 1
	final.Kind = StreamChunkFinal
	final.TextFinal = outcome.OutputPayload
	if err := final.Validate(); err != nil {
		return Outcome{}, err
	}
	if err := observer.OnComplete(final); err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}
