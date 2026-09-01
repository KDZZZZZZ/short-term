// Package middleware 保存 Gateway 的 HTTP 横切关注点：请求身份、panic 恢复、
// 访问日志、身份认证和正文大小限制。
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/id"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
)

// RequestIDHeader 允许调用方或上游代理提供会出现在日志和追踪中的请求标识。
const RequestIDHeader = "X-Request-Id"

type contextKey int

const (
	requestIDKey contextKey = iota
	actorIDKey
)

// RequestID 返回当前请求分配到的标识。
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// ActorID 返回通过认证的用户标识；未认证请求返回空字符串。
func ActorID(ctx context.Context) string {
	value, _ := ctx.Value(actorIDKey).(string)
	return value
}

// WithActorID 将通过认证的用户标识存入上下文。该函数导出后，测试可以构造已认证
// 请求，而无需签发令牌。
func WithActorID(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, actorIDKey, actorID)
}

// WithRequestID 将请求标识存入上下文。
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// Middleware 包装处理器。
type Middleware func(http.Handler) http.Handler

// Chain 应用中间件，使列表中的第一个中间件在最外层运行。
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// NewRequestID 为每个请求分配标识；调用方提供的标识存在且看起来合理时复用它。
func NewRequestID() Middleware {
	generator := id.NewGenerator(nil)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(RequestIDHeader)
			if len(requestID) == 0 || len(requestID) > 128 {
				requestID = generator.New("req")
			}
			w.Header().Set(RequestIDHeader, requestID)
			next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), requestID)))
		})
	}
}

// ErrorWriter 写入符合契约结构的错误响应。middleware 包依赖这个窄接口，
// 因此无需导入 handler 包。
type ErrorWriter interface {
	Error(w http.ResponseWriter, r *http.Request, err error)
}

// NewRecovery 将 panic 转换为 500 INTERNAL_ERROR，并记录堆栈。
func NewRecovery(logger *slog.Logger, responder ErrorWriter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					observability.LoggerWith(r.Context(), logger).Error("handler panicked",
						slog.String("path", r.URL.Path),
						slog.Any("panic", recovered),
						slog.String("stack", string(debug.Stack())),
					)
					responder.Error(w, r, errs.New(errs.CodeInternal, "服务暂时不可用"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// NewAccessLog 为每个请求写入一条结构化记录。它从不记录请求正文或查询值，
// 因为其中包含密码、联系方式和消息内容。
func NewAccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(recorder, r)

			observability.LoggerWith(r.Context(), logger).Info("request served",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.status),
				slog.String(logging.FieldRequestID, RequestID(r.Context())),
				slog.String(logging.FieldActorID, ActorID(r.Context())),
				slog.Duration("duration", time.Since(started)),
			)
		})
	}
}

// NewBodyLimit 拒绝大于 max 字节的请求正文。
func NewBodyLimit(max int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder 为访问日志记录状态码。
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.written {
		r.status = status
		r.written = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}
