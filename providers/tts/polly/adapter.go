package polly

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/polly"
	pollytypes "github.com/aws/aws-sdk-go-v2/service/polly/types"
	"github.com/aws/smithy-go"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

const ProviderID = "tts-amazon-polly"

type synthClient interface {
	SynthesizeSpeech(ctx context.Context, params *polly.SynthesizeSpeechInput, optFns ...func(*polly.Options)) (*polly.SynthesizeSpeechOutput, error)
}

type Config struct {
	Region     string
	VoiceID    string
	Engine     string
	SampleText string
	Timeout    time.Duration
}

type Adapter struct {
	mu     sync.Mutex
	client synthClient
	cfg    Config
}

func ConfigFromEnv() Config {
	return Config{
		Region:     defaultString(os.Getenv("RSPP_TTS_POLLY_REGION"), defaultString(os.Getenv("AWS_REGION"), "us-east-1")),
		VoiceID:    defaultString(os.Getenv("RSPP_TTS_POLLY_VOICE"), "Joanna"),
		Engine:     defaultString(os.Getenv("RSPP_TTS_POLLY_ENGINE"), "neural"),
		SampleText: defaultString(os.Getenv("RSPP_TTS_POLLY_TEXT"), "Realtime speech pipeline live smoke test."),
		Timeout:    15 * time.Second,
	}
}

func NewAdapter(cfg Config) (contracts.Adapter, error) {
	return NewAdapterWithClient(cfg, nil)
}

func NewAdapterWithClient(cfg Config, client synthClient) (contracts.Adapter, error) {
	if strings.TrimSpace(cfg.Region) == "" {
		cfg.Region = "us-east-1"
	}
	if strings.TrimSpace(cfg.VoiceID) == "" {
		cfg.VoiceID = "Joanna"
	}
	if strings.TrimSpace(cfg.Engine) == "" {
		cfg.Engine = "neural"
	}
	if strings.TrimSpace(cfg.SampleText) == "" {
		cfg.SampleText = "Realtime speech pipeline live smoke test."
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &Adapter{client: client, cfg: cfg}, nil
}

func NewAdapterFromEnv() (contracts.Adapter, error) {
	return NewAdapter(ConfigFromEnv())
}

func (a *Adapter) ProviderID() string {
	return ProviderID
}

func (a *Adapter) Modality() contracts.Modality {
	return contracts.ModalityTTS
}

func (a *Adapter) Invoke(req contracts.InvocationRequest) (contracts.Outcome, error) {
	if err := req.Validate(); err != nil {
		return contracts.Outcome{}, err
	}
	if req.CancelRequested {
		return contracts.Outcome{Class: contracts.OutcomeCancelled, Retryable: false, Reason: "provider_cancelled"}, nil
	}
	client, err := a.resolveClient()
	if err != nil {
		return contracts.Outcome{}, err
	}

	engine := pollytypes.EngineStandard
	if strings.EqualFold(a.cfg.Engine, "neural") {
		engine = pollytypes.EngineNeural
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()

	output, err := client.SynthesizeSpeech(ctx, &polly.SynthesizeSpeechInput{
		Engine:       engine,
		OutputFormat: pollytypes.OutputFormatMp3,
		Text:         &a.cfg.SampleText,
		TextType:     pollytypes.TextTypeText,
		VoiceId:      pollytypes.VoiceId(a.cfg.VoiceID),
	})
	if err != nil {
		return normalizePollyError(err), nil
	}
	if output == nil || output.AudioStream == nil {
		return contracts.Outcome{Class: contracts.OutcomeInfrastructureFailure, Retryable: true, Reason: "provider_empty_audio"}, nil
	}
	defer output.AudioStream.Close()
	_, _ = io.Copy(io.Discard, output.AudioStream)
	return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
}

func (a *Adapter) InvokeStream(req contracts.InvocationRequest, observer contracts.StreamObserver) (contracts.Outcome, error) {
	if observer == nil {
		observer = contracts.NoopStreamObserver{}
	}
	if err := req.Validate(); err != nil {
		return contracts.Outcome{}, err
	}
	if req.CancelRequested {
		outcome := contracts.Outcome{Class: contracts.OutcomeCancelled, Retryable: false, Reason: "provider_cancelled"}
		_ = observer.OnError(pollyStreamChunk(req, contracts.StreamChunkError, 0, nil, outcome.Reason))
		return outcome, nil
	}
	client, err := a.resolveClient()
	if err != nil {
		return contracts.Outcome{}, err
	}

	engine := pollytypes.EngineStandard
	if strings.EqualFold(a.cfg.Engine, "neural") {
		engine = pollytypes.EngineNeural
	}

	start := pollyStreamChunk(req, contracts.StreamChunkStart, 0, nil, "")
	if err := observer.OnStart(start); err != nil {
		return contracts.Outcome{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Timeout)
	defer cancel()

	output, err := client.SynthesizeSpeech(ctx, &polly.SynthesizeSpeechInput{
		Engine:       engine,
		OutputFormat: pollytypes.OutputFormatMp3,
		Text:         &a.cfg.SampleText,
		TextType:     pollytypes.TextTypeText,
		VoiceId:      pollytypes.VoiceId(a.cfg.VoiceID),
	})
	if err != nil {
		outcome := normalizePollyError(err)
		_ = observer.OnError(pollyStreamChunk(req, contracts.StreamChunkError, 1, nil, outcome.Reason))
		return outcome, nil
	}
	if output == nil || output.AudioStream == nil {
		outcome := contracts.Outcome{Class: contracts.OutcomeInfrastructureFailure, Retryable: true, Reason: "provider_empty_audio"}
		_ = observer.OnError(pollyStreamChunk(req, contracts.StreamChunkError, 1, nil, outcome.Reason))
		return outcome, nil
	}
	defer output.AudioStream.Close()

	const chunkSize = 4096
	buf := make([]byte, chunkSize)
	sequence := 1
	totalBytes := 0
	for {
		n, readErr := output.AudioStream.Read(buf)
		if n > 0 {
			audio := make([]byte, n)
			copy(audio, buf[:n])
			totalBytes += n
			if err := observer.OnChunk(pollyStreamChunk(req, contracts.StreamChunkAudio, sequence, audio, "")); err != nil {
				return contracts.Outcome{}, err
			}
			sequence++
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			outcome := contracts.Outcome{Class: contracts.OutcomeInfrastructureFailure, Retryable: true, Reason: "provider_audio_stream_read_error"}
			_ = observer.OnError(pollyStreamChunk(req, contracts.StreamChunkError, sequence, nil, outcome.Reason))
			return outcome, nil
		}
	}

	final := pollyStreamChunk(req, contracts.StreamChunkFinal, sequence, nil, "")
	final.Metadata = map[string]string{
		"audio_bytes": strconv.Itoa(totalBytes),
		"mime_type":   "audio/mpeg",
	}
	if err := observer.OnComplete(final); err != nil {
		return contracts.Outcome{}, err
	}
	return contracts.Outcome{Class: contracts.OutcomeSuccess}, nil
}

func pollyStreamChunk(req contracts.InvocationRequest, kind contracts.StreamChunkKind, sequence int, audio []byte, reason string) contracts.StreamChunk {
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
	}
	if kind == contracts.StreamChunkAudio {
		chunk.AudioBytes = audio
		chunk.MimeType = "audio/mpeg"
	}
	if kind == contracts.StreamChunkError {
		chunk.ErrorReason = reason
	}
	return chunk
}

func normalizePollyError(err error) contracts.Outcome {
	if errors.Is(err, context.Canceled) {
		return contracts.Outcome{Class: contracts.OutcomeCancelled, Retryable: false, Reason: "provider_cancelled"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return contracts.Outcome{Class: contracts.OutcomeTimeout, Retryable: true, Reason: "provider_timeout"}
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "TooManyRequestsException":
			return contracts.Outcome{Class: contracts.OutcomeOverload, Retryable: true, Reason: "provider_overload", CircuitOpen: true, BackoffMS: 500}
		case "InvalidSsmlException", "TextLengthExceededException", "LexiconNotFoundException", "MarksNotSupportedForFormatException", "InvalidSampleRateException":
			return contracts.Outcome{Class: contracts.OutcomeBlocked, Retryable: false, Reason: "provider_client_error"}
		default:
			return contracts.Outcome{Class: contracts.OutcomeInfrastructureFailure, Retryable: true, Reason: "provider_server_error", CircuitOpen: true}
		}
	}

	return contracts.Outcome{Class: contracts.OutcomeInfrastructureFailure, Retryable: true, Reason: "provider_transport_error"}
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (a *Adapter) resolveClient() (synthClient, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		return a.client, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(a.cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	a.client = polly.NewFromConfig(awsCfg)
	return a.client, nil
}

// NewTestAudioStream creates an in-memory stream for adapter tests.
func NewTestAudioStream() io.ReadCloser {
	return io.NopCloser(bytes.NewReader([]byte("mp3")))
}
