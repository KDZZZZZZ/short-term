// Package observability 接入 OpenTelemetry 追踪，并提供保持日志与追踪关联的辅助函数。
//
// docs/software-design.md 第 8.4 节要求在 REST、gRPC、数据库和事件信封之间传递上下文。
// 即使未配置收集器，传递也必须有效，因此端点为空时仍安装追踪器提供程序和
// W3C 传播器，只跳过导出器。
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

// Options 配置追踪器提供程序。
type Options struct {
	// Service 是作为 service.name 上报的可部署单元名称。
	Service string
	// Environment 作为 deployment.environment 上报。
	Environment string
	// OTLPEndpoint 是 host:port 形式的收集器地址。为空时禁用导出。
	OTLPEndpoint string
	// ExportTimeout 限制导出器连接和关闭的时间。
	ExportTimeout time.Duration
}

// Shutdown 刷新并停止追踪器提供程序。
type Shutdown func(context.Context) error

// Setup 安装全局追踪器提供程序和 W3C 追踪上下文传播器。进程退出前必须调用返回的
// Shutdown，以便刷新待处理的 span。
func Setup(ctx context.Context, opts Options) (Shutdown, error) {
	if opts.ExportTimeout <= 0 {
		opts.ExportTimeout = 10 * time.Second
	}

	// schema URL 必须与 resource.Default() 携带的 URL 匹配，否则 Merge 会拒绝合并；
	// semconv 的导入版本必须与 go.mod 中的 OpenTelemetry SDK 版本保持一致。
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

// TraceAttrs 返回 ctx 的追踪关联属性，可直接追加到日志记录中。
// 当 ctx 不包含已记录的 span 时，不返回任何属性。
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

// TraceID 返回当前追踪标识；当 ctx 不包含有效的 span 上下文时返回空字符串。
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// LoggerWith 返回携带 ctx 追踪关联字段的日志记录器，使请求期间写入的每一行日志
// 都可以关联到对应追踪。
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
