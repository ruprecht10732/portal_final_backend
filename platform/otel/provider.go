package otel

import (
	"context"
	"fmt"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// loggerExporter is a minimal SpanExporter that logs spans via the standard
// logger. It requires no additional OTel exporter dependencies.
type loggerExporter struct{}

func (e *loggerExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, s := range spans {
		attrs := make([]string, 0, len(s.Attributes()))
		for _, attr := range s.Attributes() {
			attrs = append(attrs, fmt.Sprintf("%s=%v", attr.Key, attr.Value))
		}
		log.Printf("[otel] span=%s trace=%s duration=%s attrs=%v",
			s.Name(), s.SpanContext().TraceID(), s.EndTime().Sub(s.StartTime()), attrs)
	}
	return nil
}

func (e *loggerExporter) Shutdown(ctx context.Context) error { return nil }

// InitTracerProvider initializes the global OpenTelemetry tracer provider.
// Spans are emitted to the standard application log.
func InitTracerProvider(serviceName string) *sdktrace.TracerProvider {
	r := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(r),
		sdktrace.WithBatcher(&loggerExporter{}),
	)
	otel.SetTracerProvider(provider)
	return provider
}
