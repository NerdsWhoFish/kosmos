package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type traceRoundTripper func(*http.Request) (*http.Response, error)

func (transport traceRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestSetupRedactsSDKTelemetryAtOTLPExport(t *testing.T) {
	captured := make(chan *collectortrace.ExportTraceServiceRequest, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("unexpected OTLP path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var request collectortrace.ExportTraceServiceRequest
		if err := proto.Unmarshal(data, &request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		captured <- &request
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	for _, name := range []string{"OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_HEADERS", "OTEL_EXPORTER_OTLP_LOGS_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "OTEL_RESOURCE_ATTRIBUTES"} {
		t.Setenv(name, "")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", server.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_COMPRESSION", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_COMPRESSION", "none")
	t.Setenv("KOSMOS_ENV", "test")
	_, shutdown, err := Setup(context.Background(), slog.NewJSONHandler(io.Discard, nil), "privacy-test-version")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	parentContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	ctx, parent := otel.Tracer("privacy-test").Start(trace.ContextWithRemoteSpanContext(context.Background(), parentContext), "POST /api/v1/signing/{id}/complete",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("http.route", "POST /api/v1/signing/{id}/complete"),
			attribute.String("url.path", "/api/v1/signing/private-request-id/complete"),
			attribute.String("url.full", "https://kosmos.example/api/v1/signing/private-request-id/complete?token=private-token"),
			attribute.String("client.address", "192.0.2.123"),
			attribute.String("user_agent.original", "PrivateBrowser/42"),
			attribute.String("http.request.header.x_kosmos_signer_evidence", "private-signer-envelope"),
			attribute.String("http.request.header.x_kosmos_signer_signature", "private-signer-signature"),
			attribute.String("rpc.service", "google.firestore.v1.Firestore"),
			attribute.Int("custom.retry_count", 2),
		),
		trace.WithLinks(trace.Link{SpanContext: parentContext, Attributes: []attribute.KeyValue{
			attribute.String("http.url", "https://storage.googleapis.com/bucket/private-request-id?secret=private-token"),
			attribute.String("gcp.storage.object.name", "private-request-id/original.pdf"),
		}}),
	)
	parent.SetStatus(codes.Error, "document projects/example/documents/private-request-id was not found")
	parent.AddEvent("exception", trace.WithAttributes(
		attribute.String("exception.type", "google.golang.org/grpc.NotFound"),
		attribute.String("exception.message", "document private-request-id not found"),
		attribute.String("exception.stacktrace", "private-request-id call stack"),
	))
	client := &http.Client{Transport: otelhttp.NewTransport(traceRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
	}))}
	for _, target := range []struct{ method, url string }{
		{"POST", "https://storage.googleapis.com/upload/storage/v1/b/kosmos-files/o?uploadType=multipart&name=nerds-who-fish%2Fsigning%2Fprivate-request-id%2Foriginal.pdf&secret=private-token"},
		{"GET", "https://storage.googleapis.com/kosmos-files/nerds-who-fish/signing/private-request-id/original.pdf?alt=media#private-fragment"},
		{"GET", (&url.URL{Scheme: "https", Host: "storage.googleapis.com", User: url.UserPassword("private-user", "private-password"), Path: "/storage/v1/b/kosmos-files/o/nerds-who-fish/signing/private-request-id/original.pdf"}).String()},
	} {
		request, err := http.NewRequestWithContext(ctx, target.method, target.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	parent.End()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(shutdownContext); err != nil {
		t.Fatal(err)
	}
	var exported []*tracepb.Span
	var encoded bytes.Buffer
	for len(captured) > 0 {
		request := <-captured
		encoded.WriteString(protojson.Format(request))
		for _, resource := range request.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				exported = append(exported, scope.Spans...)
			}
		}
	}
	if len(exported) != 4 {
		t.Fatalf("exported %d spans, want parent and three outgoing SDK HTTP spans", len(exported))
	}
	for _, value := range []string{"private-request-id", "private-token", "private-fragment", "private-user", "private-password", "192.0.2.123", "PrivateBrowser/42", "private-signer-envelope", "private-signer-signature", "nerds-who-fish", "original.pdf", "name=", "uploadType", "exception.message", "exception.stacktrace"} {
		if strings.Contains(encoded.String(), value) {
			t.Errorf("OTLP export leaked %q", value)
		}
	}
	for _, value := range []string{"/api/v1/signing/{id}/complete", "storage.googleapis.com", "http.request.method", "http.response.status_code", "service.name", "kosmos", "service.version", "privacy-test-version", "google.firestore.v1.Firestore", "custom.retry_count", "google.golang.org/grpc.NotFound"} {
		if !strings.Contains(encoded.String(), value) {
			t.Errorf("OTLP export lost useful metadata %q", value)
		}
	}
	var parentExport *tracepb.Span
	traceID := parentContext.TraceID()
	for _, span := range exported {
		if !bytes.Equal(span.TraceId, traceID[:]) {
			t.Error("trace correlation changed")
		}
		if span.Kind == tracepb.Span_SPAN_KIND_SERVER {
			parentExport = span
		}
	}
	if parentExport == nil || parentExport.Status.Code != tracepb.Status_STATUS_CODE_ERROR || parentExport.Status.Message != "" {
		t.Fatalf("parent error status was not preserved safely: %v", parentExport)
	}
	if len(parentExport.Links) != 1 || len(parentExport.Events) != 1 {
		t.Fatal("event or link correlation was removed")
	}
	parentID := parentContext.SpanID()
	if !bytes.Equal(parentExport.ParentSpanId, parentID[:]) || !bytes.Equal(parentExport.Links[0].SpanId, parentID[:]) {
		t.Fatal("incoming parent or linked span correlation changed")
	}
	for _, span := range exported {
		if span.Kind == tracepb.Span_SPAN_KIND_CLIENT && !bytes.Equal(span.ParentSpanId, parentExport.SpanId) {
			t.Error("outgoing request lost its parent span")
		}
	}
}

func TestPrivateTraceAttributesRejectMalformedURLsAndSensitiveVariants(t *testing.T) {
	attributes := []attribute.KeyValue{
		attribute.String("url.full", "https://example.com/%private"),
		attribute.String("url.original", "javascript:private-token"),
		attribute.String("http.target", "/private-id?secret=private-token"),
		attribute.String("url.query", "private-token"),
		attribute.String("url.fragment", "private-token"),
		attribute.String("network.peer.address", "private-ip"),
		attribute.String("http.request.header.x_kosmos_signing_token", "private-token"),
		attribute.String("http.response.header.set_cookie", "private-token"),
		attribute.String("db.query.text", "SELECT private-id"),
		attribute.String("server.address", "storage.googleapis.com"),
	}
	encoded, err := json.Marshal(privateTraceAttributes(attributes))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"private", "secret"} {
		if strings.Contains(string(encoded), value) {
			t.Errorf("attribute sanitizer leaked %q", value)
		}
	}
	if !strings.Contains(string(encoded), "storage.googleapis.com") {
		t.Fatal("server identity removed")
	}
}
