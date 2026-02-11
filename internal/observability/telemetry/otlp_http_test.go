package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestOTLPHTTPSinkRoutesByEventKind(t *testing.T) {
	t.Parallel()

	var paths []string
	var envelopes []otlpEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		defer r.Body.Close()
		var env otlpEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		envelopes = append(envelopes, env)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sink, err := NewOTLPHTTPSink(OTLPHTTPSinkConfig{
		Endpoint:    server.URL + "/collector",
		ServiceName: "rspp-test",
	})
	if err != nil {
		t.Fatalf("unexpected sink creation error: %v", err)
	}

	events := []Event{
		{Kind: EventKindMetric, Metric: &MetricEvent{Name: MetricQueueDepth}},
		{Kind: EventKindSpan, Span: &SpanEvent{Name: "turn_span"}},
		{Kind: EventKindLog, Log: &LogEvent{Name: "runtime_event"}},
	}
	for _, event := range events {
		if err := sink.Export(context.Background(), event); err != nil {
			t.Fatalf("unexpected export error: %v", err)
		}
	}

	expectedPaths := []string{"/collector/v1/metrics", "/collector/v1/traces", "/collector/v1/logs"}
	if !reflect.DeepEqual(paths, expectedPaths) {
		t.Fatalf("unexpected otlp paths: got %+v want %+v", paths, expectedPaths)
	}
	if len(envelopes) != 3 {
		t.Fatalf("expected 3 envelopes, got %d", len(envelopes))
	}
	for _, envelope := range envelopes {
		if envelope.ServiceName != "rspp-test" {
			t.Fatalf("unexpected service name: %+v", envelope)
		}
	}
}

func TestOTLPHTTPSinkExportErrorStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	sink, err := NewOTLPHTTPSink(OTLPHTTPSinkConfig{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("unexpected sink creation error: %v", err)
	}
	if err := sink.Export(context.Background(), Event{Kind: EventKindLog, Log: &LogEvent{Name: "x"}}); err == nil {
		t.Fatalf("expected non-2xx status to fail export")
	}
}

func TestNewOTLPHTTPSinkValidatesEndpoint(t *testing.T) {
	t.Parallel()

	if _, err := NewOTLPHTTPSink(OTLPHTTPSinkConfig{Endpoint: ""}); err == nil {
		t.Fatalf("expected empty endpoint validation error")
	}
	if _, err := NewOTLPHTTPSink(OTLPHTTPSinkConfig{Endpoint: "localhost:4318"}); err == nil {
		t.Fatalf("expected endpoint without scheme to fail")
	}
}
