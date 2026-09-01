package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoaderReadsTypedValues(t *testing.T) {
	t.Parallel()

	loader := NewLoaderFrom(map[string]string{
		"GRPC_ADDR":    ":9001",
		"POOL_SIZE":    "12",
		"RPC_TIMEOUT":  "750ms",
		"TRACING_ON":   "true",
		"EMPTY_STRING": "   ",
	})

	if got := loader.String("GRPC_ADDR", ":0"); got != ":9001" {
		t.Fatalf("String = %q, want :9001", got)
	}
	if got := loader.String("EMPTY_STRING", "fallback"); got != "fallback" {
		t.Fatalf("blank value should fall back, got %q", got)
	}
	if got := loader.Int("POOL_SIZE", 4); got != 12 {
		t.Fatalf("Int = %d, want 12", got)
	}
	if got := loader.Duration("RPC_TIMEOUT", time.Second); got != 750*time.Millisecond {
		t.Fatalf("Duration = %s, want 750ms", got)
	}
	if got := loader.Bool("TRACING_ON", false); !got {
		t.Fatal("Bool = false, want true")
	}
	if err := loader.Err(); err != nil {
		t.Fatalf("Err = %v, want nil", err)
	}
}

func TestLoaderCollectsEveryProblem(t *testing.T) {
	t.Parallel()

	loader := NewLoaderFrom(map[string]string{
		"POOL_SIZE":   "many",
		"RPC_TIMEOUT": "soon",
	})

	loader.Required("JWT_SIGNING_KEY")
	loader.Int("POOL_SIZE", 4)
	loader.Duration("RPC_TIMEOUT", time.Second)

	err := loader.Err()
	if err == nil {
		t.Fatal("Err = nil, want three joined problems")
	}
	for _, want := range []string{"JWT_SIGNING_KEY", "POOL_SIZE", "RPC_TIMEOUT"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRuntimeDefaults(t *testing.T) {
	t.Parallel()

	runtime := NewLoaderFrom(nil).LoadRuntime("gateway")

	if runtime.Service != "gateway" {
		t.Fatalf("Service = %q, want gateway", runtime.Service)
	}
	if runtime.Environment != "local" {
		t.Fatalf("Environment = %q, want local", runtime.Environment)
	}
	if runtime.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v, want INFO", runtime.LogLevel)
	}
	if runtime.OTLPEndpoint != "" {
		t.Fatalf("OTLPEndpoint = %q, want empty", runtime.OTLPEndpoint)
	}
	if runtime.ShutdownTimeout != 15*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 15s", runtime.ShutdownTimeout)
	}
}

func TestLoadRuntimeParsesLogLevel(t *testing.T) {
	t.Parallel()

	runtime := NewLoaderFrom(map[string]string{"LOG_LEVEL": "debug"}).LoadRuntime("account")
	if runtime.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want DEBUG", runtime.LogLevel)
	}

	loader := NewLoaderFrom(map[string]string{"LOG_LEVEL": "chatty"})
	loader.LoadRuntime("account")
	if loader.Err() == nil {
		t.Fatal("an unparsable log level should be reported")
	}
}
