package polly

import (
	"context"
	"errors"
	"testing"

	pollysdk "github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
	"github.com/aws/smithy-go"
	"github.com/tiger/realtime-speech-pipeline/internal/runtime/provider/contracts"
)

type fakePollyClient struct {
	out *pollysdk.SynthesizeSpeechOutput
	err error
}

func (f fakePollyClient) SynthesizeSpeech(ctx context.Context, params *pollysdk.SynthesizeSpeechInput, optFns ...func(*pollysdk.Options)) (*pollysdk.SynthesizeSpeechOutput, error) {
	return f.out, f.err
}

type fakeAPIError struct {
	code string
	msg  string
}

func (e fakeAPIError) Error() string {
	return e.code + ": " + e.msg
}

func (e fakeAPIError) ErrorCode() string {
	return e.code
}

func (e fakeAPIError) ErrorMessage() string {
	return e.msg
}

func (e fakeAPIError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultServer
}

func TestInvokeSuccess(t *testing.T) {
	t.Parallel()

	adapter, err := NewAdapterWithClient(Config{}, fakePollyClient{
		out: &pollysdk.SynthesizeSpeechOutput{AudioStream: NewTestAudioStream()},
	})
	if err != nil {
		t.Fatalf("unexpected adapter error: %v", err)
	}

	outcome, err := adapter.Invoke(contracts.InvocationRequest{
		SessionID:            "sess-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           ProviderID,
		Modality:             contracts.ModalityTTS,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if outcome.Class != contracts.OutcomeSuccess {
		t.Fatalf("expected success, got %s", outcome.Class)
	}
}

func TestInvokeErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected contracts.OutcomeClass
	}{
		{name: "timeout", err: context.DeadlineExceeded, expected: contracts.OutcomeTimeout},
		{name: "overload", err: fakeAPIError{code: "TooManyRequestsException", msg: "rate"}, expected: contracts.OutcomeOverload},
		{name: "blocked", err: fakeAPIError{code: "TextLengthExceededException", msg: "too long"}, expected: contracts.OutcomeBlocked},
		{name: "infra", err: errors.New("tcp reset"), expected: contracts.OutcomeInfrastructureFailure},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			adapter, err := NewAdapterWithClient(Config{}, fakePollyClient{err: tc.err})
			if err != nil {
				t.Fatalf("unexpected adapter error: %v", err)
			}
			outcome, err := adapter.Invoke(contracts.InvocationRequest{
				SessionID:            "sess-1",
				PipelineVersion:      "pipeline-v1",
				EventID:              "evt-1",
				ProviderInvocationID: "pvi-1",
				ProviderID:           ProviderID,
				Modality:             contracts.ModalityTTS,
				Attempt:              1,
				TransportSequence:    1,
				RuntimeSequence:      1,
				AuthorityEpoch:       1,
				RuntimeTimestampMS:   1,
				WallClockTimestampMS: 1,
			})
			if err != nil {
				t.Fatalf("unexpected invoke error: %v", err)
			}
			if outcome.Class != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, outcome.Class)
			}
		})
	}
}

func TestInvokeCancelled(t *testing.T) {
	t.Parallel()

	adapter, err := NewAdapterWithClient(Config{}, fakePollyClient{
		out: &pollysdk.SynthesizeSpeechOutput{AudioStream: NewTestAudioStream()},
	})
	if err != nil {
		t.Fatalf("unexpected adapter error: %v", err)
	}

	outcome, err := adapter.Invoke(contracts.InvocationRequest{
		SessionID:            "sess-1",
		PipelineVersion:      "pipeline-v1",
		EventID:              "evt-1",
		ProviderInvocationID: "pvi-1",
		ProviderID:           ProviderID,
		Modality:             contracts.ModalityTTS,
		Attempt:              1,
		TransportSequence:    1,
		RuntimeSequence:      1,
		AuthorityEpoch:       1,
		RuntimeTimestampMS:   1,
		WallClockTimestampMS: 1,
		CancelRequested:      true,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if outcome.Class != contracts.OutcomeCancelled {
		t.Fatalf("expected cancelled, got %s", outcome.Class)
	}
}

var _ smithy.APIError = fakeAPIError{}
var _ = types.OutputFormatMp3
