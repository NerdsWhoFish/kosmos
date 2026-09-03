package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestSetupWithoutExporterUsesFallback(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	logger, shutdown, err := Setup(context.Background(), slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if err != nil || logger == nil {
		t.Fatalf("Setup() logger = %v, err = %v", logger, err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestRequestLoggerCorrelatesTraceAndStatus(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /things/{id}", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusCreated)
	})
	traceID := trace.TraceID{1}
	spanID := trace.SpanID{2}
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	request := httptest.NewRequest(http.MethodGet, "/things/42", nil)
	request = request.WithContext(trace.ContextWithSpanContext(request.Context(), span))
	record := httptest.NewRecorder()

	RequestLogger(logger, mux).ServeHTTP(record, request)

	var entry map[string]any
	if err := json.NewDecoder(&output).Decode(&entry); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if entry["http.route"] != "GET /things/{id}" || entry["http.response.status_code"] != float64(http.StatusCreated) {
		t.Fatalf("unexpected request fields: %#v", entry)
	}
	if entry["trace_id"] != traceID.String() || entry["span_id"] != spanID.String() {
		t.Fatalf("missing trace correlation: %#v", entry)
	}
}
