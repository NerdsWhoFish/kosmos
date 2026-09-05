package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestSetupPropagatesTraceContextAndBaggage(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	_, shutdown, err := Setup(context.Background(), slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(context.Background())
	traceparent := "00-01000000000000000000000000000000-0200000000000000-01"
	incoming := propagation.MapCarrier{"traceparent": traceparent, "baggage": "workflow=sync"}
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), incoming)
	if !trace.SpanContextFromContext(ctx).IsRemote() || baggage.FromContext(ctx).Member("workflow").Value() != "sync" {
		t.Fatal("inbound trace or baggage was not extracted")
	}
	outgoing := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, outgoing)
	if outgoing.Get("traceparent") != traceparent || outgoing.Get("baggage") != "workflow=sync" {
		t.Fatalf("outbound propagation = %v", outgoing)
	}
}

func TestHTTPTraceDoesNotExportSigningIdentityOrToken(t *testing.T) {
	collector := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))
	defer provider.Shutdown(context.Background())
	var logs bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/signing/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := otelhttp.NewHandler(RequestLogger(slog.New(slog.NewJSONHandler(&logs, nil)), mux), "test", otelhttp.WithTracerProvider(provider))
	r := httptest.NewRequest("GET", "http://localhost/api/v1/signing/private-request-id?secret=private-token", nil)
	r.Header.Set("X-Kosmos-Signing-Token", "private-token")
	handler.ServeHTTP(httptest.NewRecorder(), r)
	spans, err := json.Marshal(collector.GetSpans())
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"private-request-id", "private-token"} {
		if strings.Contains(string(spans), value) || strings.Contains(logs.String(), value) {
			t.Fatalf("telemetry leaked %s", value)
		}
	}
	if !strings.Contains(string(spans), "/api/v1/signing/{id}") {
		t.Fatal("route template missing from trace")
	}
}

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
