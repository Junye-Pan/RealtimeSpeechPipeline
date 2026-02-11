package livekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tiger/realtime-speech-pipeline/api/eventabi"
)

func TestProbeRoomServicePassesWithValidResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/twirp/livekit.RoomService/ListRooms" {
			t.Fatalf("unexpected path: %s", got)
		}
		if got := r.Header.Get("Authorization"); got == "" {
			t.Fatalf("expected authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rooms":[{"name":"room-a"}]}`))
	}))
	defer server.Close()

	cfg := Config{
		URL:              server.URL,
		APIKey:           "key",
		APISecret:        "secret",
		Room:             "room-a",
		SessionID:        "sess",
		TurnID:           "turn",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           false,
		Probe:            true,
		RequestTimeout:   5 * time.Second,
	}
	result, err := ProbeRoomService(context.Background(), cfg, nil, fixedProbeNow)
	if err != nil {
		t.Fatalf("unexpected probe error: %v", err)
	}
	if result.Status != ProbeStatusPassed || result.HTTPStatus != http.StatusOK || result.RoomCount != 1 {
		t.Fatalf("unexpected probe result: %+v", result)
	}
}

func TestProbeRoomServiceReturnsErrorOnFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthenticated"}`))
	}))
	defer server.Close()

	cfg := Config{
		URL:              server.URL,
		APIKey:           "key",
		APISecret:        "secret",
		Room:             "room-a",
		SessionID:        "sess",
		TurnID:           "turn",
		PipelineVersion:  "pipeline-v1",
		AuthorityEpoch:   1,
		DefaultDataClass: eventabi.PayloadAudioRaw,
		DryRun:           false,
		Probe:            true,
		RequestTimeout:   5 * time.Second,
	}
	result, err := ProbeRoomService(context.Background(), cfg, nil, fixedProbeNow)
	if err == nil {
		t.Fatalf("expected probe failure")
	}
	if result.Status != ProbeStatusFailed || result.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("unexpected probe failure result: %+v", result)
	}
}

func fixedProbeNow() time.Time {
	return time.Date(2026, time.February, 11, 10, 0, 0, 0, time.UTC)
}
