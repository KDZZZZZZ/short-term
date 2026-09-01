// Package logging builds the structured logger every service uses.
//
// docs/software-design.md section 8.4 fixes the field names and forbids
// passwords, whole JWTs, contact details, message bodies and credentials from
// reaching logs. The redacting handler here enforces that at write time rather
// than trusting each call site to remember.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// Canonical field names shared by all services and by the trace exporters.
const (
	FieldService     = "service"
	FieldEnvironment = "environment"
	FieldTraceID     = "trace_id"
	FieldSpanID      = "span_id"
	FieldRequestID   = "request_id"
	FieldActorID     = "actor_id"
	FieldErrorCode   = "error_code"
)

// Redacted replaces the value of any attribute whose key is classified as
// sensitive.
const Redacted = "[REDACTED]"

// deniedKeys are attribute names that must never be written in clear text.
// Matching is case-insensitive and also covers dotted or prefixed variants
// such as "request.password" or "user_wechat".
var deniedKeys = []string{
	"password",
	"password_hash",
	"old_password",
	"new_password",
	"token",
	"access_token",
	"refresh_token",
	"authorization",
	"jwt",
	"secret",
	"wechat",
	"qq",
	"content",
	"message_content",
	"student_no",
	"dsn",
	"database_url",
	"oss_access_key",
	"oss_secret_key",
}

// Options configures the process logger.
type Options struct {
	// Service is the deployable unit name, for example "gateway".
	Service string
	// Environment is the deployment environment, for example "local".
	Environment string
	// Level is the minimum level to emit.
	Level slog.Level
}

// New builds a JSON logger that stamps every record with the service and
// environment and redacts sensitive attributes.
func New(w io.Writer, opts Options) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: opts.Level})
	return slog.New(&redactingHandler{inner: handler}).With(
		slog.String(FieldService, opts.Service),
		slog.String(FieldEnvironment, opts.Environment),
	)
}

// IsSensitive reports whether an attribute key must be redacted. It is
// exported so tests and reviewers can assert the policy directly.
func IsSensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, denied := range deniedKeys {
		if lower == denied || strings.HasSuffix(lower, "."+denied) || strings.HasSuffix(lower, "_"+denied) {
			return true
		}
	}
	return false
}

// redactingHandler rewrites sensitive attributes before delegating.
type redactingHandler struct {
	inner slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		safe.AddAttrs(redact(attr))
		return true
	})
	return h.inner.Handle(ctx, safe)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	safe := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		safe[i] = redact(attr)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(safe)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name)}
}

// redact rewrites one attribute, descending into groups so a sensitive field
// cannot hide one level down.
func redact(attr slog.Attr) slog.Attr {
	if IsSensitive(attr.Key) {
		return slog.String(attr.Key, Redacted)
	}
	if attr.Value.Kind() == slog.KindGroup {
		members := attr.Value.Group()
		safe := make([]any, 0, len(members))
		for _, member := range members {
			safe = append(safe, redact(member))
		}
		return slog.Group(attr.Key, safe...)
	}
	return attr
}
