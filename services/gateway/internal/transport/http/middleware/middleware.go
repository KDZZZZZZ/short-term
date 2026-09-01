// Package middleware holds the Gateway's cross-cutting HTTP concerns:
// request identity, panic recovery, access logging, authentication and body
// size limits.
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

// RequestIDHeader lets a caller or an upstream proxy supply the request
// identifier that appears in logs and traces.
const RequestIDHeader = "X-Request-Id"

type contextKey int

const (
	requestIDKey contextKey = iota
	actorIDKey
)

// RequestID returns the identifier assigned to the current request.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// ActorID returns the authenticated user identifier, or an empty string on an
// unauthenticated request.
func ActorID(ctx context.Context) string {
	value, _ := ctx.Value(actorIDKey).(string)
	return value
}

// WithActorID stores an authenticated user identifier on the context. It is
// exported so tests can build authenticated requests without minting tokens.
func WithActorID(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, actorIDKey, actorID)
}

// WithRequestID stores a request identifier on the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so that the first listed runs outermost.
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// NewRequestID assigns each request an identifier, reusing a caller-supplied
// one when it is present and plausible.
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

// ErrorWriter writes a contract-shaped error response. The middleware package
// depends on this narrow interface so it does not import the handler package.
type ErrorWriter interface {
	Error(w http.ResponseWriter, r *http.Request, err error)
}

// NewRecovery turns a panic into 500 INTERNAL_ERROR and logs the stack.
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

// NewAccessLog writes one structured record per request. It never logs the
// request body or query values, which carry passwords, contact details and
// message content.
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

// NewBodyLimit rejects request bodies larger than max bytes.
func NewBodyLimit(max int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder remembers the status code for the access log.
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
