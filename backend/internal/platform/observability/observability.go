package observability

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

func Setup(ctx context.Context, fallback slog.Handler, versions ...string) (*slog.Logger, func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return slog.New(fallback), func(context.Context) error { return nil }, nil
	}
	version := "dev"
	if len(versions) > 0 && versions[0] != "" {
		version = versions[0]
	}
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithAttributes(
		semconv.ServiceName("kosmos"),
		semconv.ServiceVersion(version),
		semconv.DeploymentEnvironmentName(os.Getenv("KOSMOS_ENV")),
	))
	if err != nil {
		return nil, nil, err
	}
	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		_ = traceProvider.Shutdown(ctx)
		return nil, nil, err
	}
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)
	logger := slog.New(slog.NewMultiHandler(fallback, otelslog.NewHandler("github.com/NerdsWhoFish/kosmos", otelslog.WithLoggerProvider(logProvider))))
	shutdown := func(ctx context.Context) error {
		return errors.Join(logProvider.Shutdown(ctx), traceProvider.Shutdown(ctx))
	}
	return logger, shutdown, nil
}

type responseCapture struct {
	http.ResponseWriter
	status int
}

func (response *responseCapture) WriteHeader(status int) {
	if response.status != 0 {
		return
	}
	response.status = status
	response.ResponseWriter.WriteHeader(status)
}

func (response *responseCapture) Write(body []byte) (int, error) {
	if response.status == 0 {
		response.WriteHeader(http.StatusOK)
	}
	return response.ResponseWriter.Write(body)
}

func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		started := time.Now()
		captured := &responseCapture{ResponseWriter: response}
		next.ServeHTTP(captured, request)
		if request.Pattern != "" {
			span := trace.SpanFromContext(request.Context())
			span.SetName(request.Pattern)
			_, route, hasMethod := strings.Cut(request.Pattern, " ")
			if !hasMethod {
				route = request.Pattern
			}
			span.SetAttributes(semconv.HTTPRoute(request.Pattern), semconv.URLPath(route))
		}
		if request.Pattern == "GET /api/v1/health" {
			return
		}
		status := captured.status
		if status == 0 {
			status = http.StatusOK
		}
		attributes := []any{
			"http.request.method", request.Method,
			"http.route", request.Pattern,
			"http.response.status_code", status,
			"duration_ms", time.Since(started).Milliseconds(),
		}
		span := trace.SpanContextFromContext(request.Context())
		if span.IsValid() {
			attributes = append(attributes, "trace_id", span.TraceID().String(), "span_id", span.SpanID().String())
		}
		logger.InfoContext(request.Context(), "request completed", attributes...)
	})
}
