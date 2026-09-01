// Package observability wires OpenTelemetry tracing and exposes the helpers
// that keep logs and traces correlated.
//
// docs/software-design.md section 8.4 requires context propagation across
// REST, gRPC, the database and event envelopes. Propagation must work even
// when no collector is configured, so an empty endpoint still installs a
// tracer provider and the W3C propagator; only the exporter is skipped.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/KDZZZZZZ/short-term/platform/logging"
)

// Options configures the tracer provider.
type Options struct {
	// Service is the deployable unit name reported as service.name.
	Service string
	// Environment is reported as deployment.environment.
	Environment string
	// OTLPEndpoint is a host:port collector address. Empty disables export.
	OTLPEndpoint string
	// ExportTimeout bounds the exporter connection and shutdown.
	ExportTimeout time.Duration
}

// Shutdown flushes and stops the tracer provider.
type Shutdown func(context.Context) error

// Setup installs the global tracer provider and the W3C trace context
// propagator. The returned Shutdown must be called before the process exits so
// pending spans are flushed.
func Setup(ctx context.Context, opts Options) (Shutdown, error) {
	if opts.ExportTimeout <= 0 {
		opts.ExportTimeout = 10 * time.Second
	}

	// The schema URL must match the one resource.Default() carries, otherwise
	// Merge refuses to combine them; keep the semconv import in step with the
	// OpenTelemetry SDK version in go.mod.
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(opts.Service),
		semconv.DeploymentEnvironmentNameKey.String(opts.Environment),
	))
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	providerOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if opts.OTLPEndpoint != "" {
		exporter, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(opts.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithTimeout(opts.ExportTimeout),
		)
		if err != nil {
			return nil, fmt.Errorf("observability: build OTLP exporter: %w", err)
		}
		providerOpts = append(providerOpts, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(providerOpts...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}

// TraceAttrs returns the trace correlation attributes for ctx, ready to append
// to a log record. It returns no attributes when ctx carries no recorded span.
func TraceAttrs(ctx context.Context) []slog.Attr {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []slog.Attr{
		slog.String(logging.FieldTraceID, sc.TraceID().String()),
		slog.String(logging.FieldSpanID, sc.SpanID().String()),
	}
}

// TraceID returns the current trace identifier, or an empty string when ctx
// carries no valid span context.
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// LoggerWith returns a logger that carries the trace correlation fields for
// ctx, so every line written during a request can be joined to its trace.
func LoggerWith(ctx context.Context, logger *slog.Logger) *slog.Logger {
	attrs := TraceAttrs(ctx)
	if len(attrs) == 0 {
		return logger
	}
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	return logger.With(args...)
}
