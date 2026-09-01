package config

import (
	"log/slog"
	"strings"
	"time"
)

// Runtime is the configuration every deployable unit shares.
type Runtime struct {
	// Service is the deployable unit name used in logs, traces and metrics.
	Service string
	// Environment is the deployment environment name.
	Environment string
	// LogLevel is the minimum level written to stdout.
	LogLevel slog.Level
	// OTLPEndpoint is the OpenTelemetry collector endpoint. Empty disables
	// span export; the tracer still runs so context propagation keeps working.
	OTLPEndpoint string
	// ShutdownTimeout bounds graceful shutdown before the process exits.
	ShutdownTimeout time.Duration
}

// LoadRuntime reads the shared runtime settings for the named service.
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
