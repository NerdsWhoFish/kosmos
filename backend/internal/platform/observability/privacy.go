package observability

import (
	"context"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type privateTraceExporter struct {
	sdktrace.SpanExporter
}

func (exporter privateTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	private := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, span := range spans {
		private[i] = privateSpan{ReadOnlySpan: span}
	}
	return exporter.SpanExporter.ExportSpans(ctx, private)
}

type privateSpan struct {
	sdktrace.ReadOnlySpan
}

func (span privateSpan) Attributes() []attribute.KeyValue {
	return privateTraceAttributes(span.ReadOnlySpan.Attributes())
}

func (span privateSpan) Events() []sdktrace.Event {
	events := append([]sdktrace.Event(nil), span.ReadOnlySpan.Events()...)
	for i := range events {
		events[i].Attributes = privateTraceAttributes(events[i].Attributes)
	}
	return events
}

func (span privateSpan) Links() []sdktrace.Link {
	links := append([]sdktrace.Link(nil), span.ReadOnlySpan.Links()...)
	for i := range links {
		links[i].Attributes = privateTraceAttributes(links[i].Attributes)
	}
	return links
}

func (span privateSpan) Status() sdktrace.Status {
	status := span.ReadOnlySpan.Status()
	status.Description = ""
	return status
}

func privateTraceAttributes(attributes []attribute.KeyValue) []attribute.KeyValue {
	route := "/{redacted}"
	for _, item := range attributes {
		if item.Key == "http.route" && item.Value.Type() == attribute.STRING {
			route = item.Value.AsString()
			if _, path, ok := strings.Cut(route, " "); ok {
				route = path
			}
			break
		}
	}
	private := make([]attribute.KeyValue, 0, len(attributes))
	for _, item := range attributes {
		key := string(item.Key)
		switch {
		case key == "url.full" || key == "url.original" || key == "http.url":
			item = item.Key.String(privateTraceURL(item.Value.AsString(), route))
		case key == "url.path" || key == "http.target":
			item = item.Key.String(route)
		case key == "url.query" || key == "url.fragment" || key == "url.userinfo",
			key == "client.address" || key == "client.port" || key == "http.client_ip",
			key == "network.peer.address" || key == "net.sock.peer.addr" || key == "net.peer.ip",
			key == "user_agent.original" || key == "http.user_agent",
			key == "exception.message" || key == "exception.stacktrace",
			key == "db.statement" || key == "db.query.text",
			strings.HasPrefix(key, "enduser."),
			strings.HasPrefix(key, "http.request.header."),
			strings.HasPrefix(key, "http.response.header."),
			strings.Contains(key, "object") && (strings.HasPrefix(key, "gcp.") || strings.HasPrefix(key, "gcs.") || strings.HasPrefix(key, "cloud.")):
			continue
		}
		private = append(private, item)
	}
	return private
}

func privateTraceURL(value, route string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "[redacted]"
	}
	// SDK object paths can carry customer identifiers as readily as query strings.
	return parsed.Scheme + "://" + parsed.Host + route
}
