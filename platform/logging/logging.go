// Package logging 构造所有服务使用的结构化日志记录器。
//
// docs/software-design.md 第 8.4 节规定了字段名，并禁止密码、完整 JWT、联系方式、
// 消息正文和凭据进入日志。本包中的脱敏处理器在写入时强制执行这些规则，
// 不依赖每个调用点自行记住。
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// 所有服务及追踪导出器共享的规范字段名。
const (
	FieldService     = "service"
	FieldEnvironment = "environment"
	FieldTraceID     = "trace_id"
	FieldSpanID      = "span_id"
	FieldRequestID   = "request_id"
	FieldActorID     = "actor_id"
	FieldErrorCode   = "error_code"
)

// Redacted 替换键被归类为敏感信息的属性值。
const Redacted = "[REDACTED]"

// deniedKeys 是绝不能以明文写入的属性名。匹配不区分大小写，也覆盖带点号或
// 前缀的变体，例如 "request.password" 或 "user_wechat"。
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

// Options 配置进程日志记录器。
type Options struct {
	// Service 是可部署单元名称，例如 "gateway"。
	Service string
	// Environment 是部署环境，例如 "local"。
	Environment string
	// Level 是要输出的最低级别。
	Level slog.Level
}

// New 构造 JSON 日志记录器，为每条记录添加服务和环境，并对敏感属性脱敏。
func New(w io.Writer, opts Options) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: opts.Level})
	return slog.New(&redactingHandler{inner: handler}).With(
		slog.String(FieldService, opts.Service),
		slog.String(FieldEnvironment, opts.Environment),
	)
}

// IsSensitive 判断属性键是否必须脱敏。该函数导出后，测试和评审者可以直接验证
// 这项策略。
func IsSensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, denied := range deniedKeys {
		if lower == denied || strings.HasSuffix(lower, "."+denied) || strings.HasSuffix(lower, "_"+denied) {
			return true
		}
	}
	return false
}

// redactingHandler 在委托处理前重写敏感属性。
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

// redact 重写一个属性，并递归处理组，避免敏感字段藏在下一层。
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
