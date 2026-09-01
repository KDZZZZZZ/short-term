package config

import (
	"log/slog"
	"strings"
	"time"
)

// Runtime 是每个可部署单元共享的配置。
type Runtime struct {
	// Service 是日志、追踪和指标中使用的可部署单元名称。
	Service string
	// Environment 是部署环境名称。
	Environment string
	// LogLevel 是写入 stdout 的最低日志级别。
	LogLevel slog.Level
	// OTLPEndpoint 是 OpenTelemetry 收集器端点。为空时禁用 span 导出；
	// 追踪器仍会运行，以保证上下文传递继续生效。
	OTLPEndpoint string
	// ShutdownTimeout 限制进程退出前的优雅关闭时间。
	ShutdownTimeout time.Duration
}

// LoadRuntime 读取指定服务的共享运行时设置。
func (l *Loader) LoadRuntime(service string) Runtime {
	return Runtime{
		Service:         service,
		Environment:     l.String("ENVIRONMENT", "local"),
		LogLevel:        l.logLevel("LOG_LEVEL", slog.LevelInfo),
		OTLPEndpoint:    l.String("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		ShutdownTimeout: l.Duration("SHUTDOWN_TIMEOUT", 15*time.Second),
	}
}

func (l *Loader) logLevel(key string, fallback slog.Level) slog.Level {
	raw := l.String(key, "")
	if raw == "" {
		return fallback
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(raw))); err != nil {
		l.Fail("config: %s must be a slog level: %w", key, err)
		return fallback
	}
	return level
}
