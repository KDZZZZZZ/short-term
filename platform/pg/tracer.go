package pg

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracerName 标识数据库 span 的插桩作用域。
const tracerName = "github.com/KDZZZZZZ/short-term/platform/pg"

// QueryTracer 为每次查询生成一个 span，使请求可以从 HTTP 经过 gRPC 追踪到数据库
// （docs/software-design.md 第 8.4 节）。
//
// SQL 文本会被记录，因为它是静态的、由作者编写的文本。查询参数绝不记录，
// 因为其中包含密码、联系方式和消息内容。
type QueryTracer struct {
	tracer trace.Tracer
}

// NewQueryTracer 构造绑定到全局提供程序的追踪器。
func NewQueryTracer() *QueryTracer {
	return &QueryTracer{tracer: otel.Tracer(tracerName)}
}

type spanKey struct{}

// TraceQueryStart 为查询打开 span。
func (t *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx, span := t.tracer.Start(ctx, "postgresql.query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system", "postgresql")),
		trace.WithAttributes(attribute.String("db.statement", data.SQL)),
	)
	return context.WithValue(ctx, spanKey{}, span)
}

// TraceQueryEnd 关闭 span，并在发生失败时记录失败信息。
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
