package pg

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracerName identifies the instrumentation scope for database spans.
const tracerName = "github.com/KDZZZZZZ/short-term/platform/pg"

// QueryTracer emits one span per query so a request can be followed from HTTP
// through gRPC into the database (docs/software-design.md section 8.4).
//
// The SQL text is recorded because it is static, author-written text. Query
// arguments are never recorded: they carry passwords, contact details and
// message content.
type QueryTracer struct {
	tracer trace.Tracer
}

// NewQueryTracer builds a tracer bound to the global provider.
func NewQueryTracer() *QueryTracer {
	return &QueryTracer{tracer: otel.Tracer(tracerName)}
}

type spanKey struct{}

// TraceQueryStart opens the span for a query.
func (t *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx, span := t.tracer.Start(ctx, "postgresql.query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
		trace.WithAttributes(attribute.String("db.statement", data.SQL)),
	)
	return context.WithValue(ctx, spanKey{}, span)
}

// TraceQueryEnd closes the span and records a failure when one occurred.
func (t *QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(spanKey{}).(trace.Span)
	if !ok {
		return
	}
	if data.Err != nil && !IsNoRows(data.Err) {
		span.SetStatus(codes.Error, data.Err.Error())
	}
	span.SetAttributes(attribute.String("db.rows_affected", data.CommandTag.String()))
	span.End()
}
