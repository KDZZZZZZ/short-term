package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewStampsServiceAndEnvironment(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := New(&buf, Options{Service: "gateway", Environment: "local", Level: slog.LevelInfo})
	logger.Info("started")

	record := decode(t, buf.Bytes())
	if record[FieldService] != "gateway" {
		t.Fatalf("service = %v, want gateway", record[FieldService])
	}
	if record[FieldEnvironment] != "local" {
		t.Fatalf("environment = %v, want local", record[FieldEnvironment])
	}
}

func TestSensitiveAttributesAreRedacted(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := New(&buf, Options{Service: "account", Environment: "test", Level: slog.LevelInfo})
	logger.Info("register",
		slog.String("password", "correct-horse-battery-staple"),
		slog.String("student_no", "20260001"),
		slog.String("request.wechat", "wx_xiaoming"),
		slog.String("actor_id", "u_01ARZ3NDEKTSV4RRFFQ69G5FAV"),
		slog.Group("payload",
			slog.String("content", "线下见面"),
			slog.String("conversation_id", "c_01ARZ3NDEKTSV4RRFFQ69G5FAV"),
		),
	)

	raw := buf.String()
	for _, leaked := range []string{"correct-horse-battery-staple", "20260001", "wx_xiaoming", "线下见面"} {
		if strings.Contains(raw, leaked) {
			t.Fatalf("log leaked %q: %s", leaked, raw)
		}
	}

	record := decode(t, buf.Bytes())
	if record["actor_id"] != "u_01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("actor_id was altered: %v", record["actor_id"])
	}
	payload, ok := record["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload group missing: %v", record["payload"])
	}
	if payload["content"] != Redacted {
		t.Fatalf("grouped content = %v, want %s", payload["content"], Redacted)
	}
	if payload["conversation_id"] != "c_01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("grouped conversation_id was altered: %v", payload["conversation_id"])
	}
}

func TestWithAttrsRedactsPersistentFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := New(&buf, Options{Service: "account", Environment: "test", Level: slog.LevelInfo})
	logger.With(slog.String("access_token", "eyJhbGciOi.payload.sig")).Info("issued")

	if strings.Contains(buf.String(), "eyJhbGciOi") {
		t.Fatalf("persistent attribute leaked a token: %s", buf.String())
	}
}

func TestIsSensitive(t *testing.T) {
	t.Parallel()

	sensitive := []string{"password", "Password", "old_password", "request.qq", "user_wechat", "DATABASE_URL"}
	for _, key := range sensitive {
		if !IsSensitive(key) {
			t.Fatalf("IsSensitive(%q) = false, want true", key)
		}
	}

	safe := []string{"actor_id", "product_id", "status", "error_code", "quality", "contented"}
	for _, key := range safe {
		if IsSensitive(key) {
			t.Fatalf("IsSensitive(%q) = true, want false", key)
		}
	}
}

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, raw)
	}
	return record
}
