package grpcx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/platform/observability"
)

// ServerOptions configures an internal gRPC server.
type ServerOptions struct {
	// Logger receives one structured record per completed call.
	Logger *slog.Logger
	// MaxRecvMsgSize bounds a single request. Image uploads travel through
	// AddProductImages, so the default is larger than the gRPC default of 4MB.
	MaxRecvMsgSize int
	// HandlerTimeout bounds a handler when the caller sent no deadline. Zero
	// leaves such calls unbounded, which is only appropriate in tests.
	HandlerTimeout time.Duration
}

// NewServer builds a gRPC server with the shared interceptor chain. Callers
// register their services on the result and then call Serve.
func NewServer(opts ServerOptions) *grpc.Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.MaxRecvMsgSize <= 0 {
		opts.MaxRecvMsgSize = 16 << 20
	}

	return grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.MaxRecvMsgSize(opts.MaxRecvMsgSize),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(
			recoveryInterceptor(opts.Logger),
			timeoutInterceptor(opts.HandlerTimeout),
			loggingInterceptor(opts.Logger),
			normalizeErrorInterceptor(),
		),
	)
}

// Serve runs srv on addr until ctx is cancelled, then stops it gracefully.
func Serve(ctx context.Context, srv *grpc.Server, addr string, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpcx: listen on %s: %w", addr, err)
	}

	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()

	logger.Info("grpc server listening", slog.String("addr", listener.Addr().String()))

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
		logger.Info("grpc server shutting down")
		srv.GracefulStop()
		return nil
	}
}

// recoveryInterceptor converts a panic into INTERNAL_ERROR. The stack goes to
// the log; the caller only learns that the call failed.
func recoveryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				observability.LoggerWith(ctx, logger).Error("handler panicked",
					slog.String("method", info.FullMethod),
					slog.Any("panic", recovered),
					slog.String("stack", string(debug.Stack())),
				)
				err = errs.New(errs.CodeInternal, "服务暂时不可用")
			}
		}()
		return handler(ctx, req)
	}
}

// timeoutInterceptor bounds handlers whose caller sent no deadline, so a
// missing client deadline cannot pin a database connection indefinitely.
func timeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if timeout <= 0 {
			return handler(ctx, req)
		}
		if _, ok := ctx.Deadline(); ok {
			return handler(ctx, req)
		}
		bounded, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return handler(bounded, req)
	}
}

// loggingInterceptor writes one structured record per call using the canonical
// field names. Request and response bodies are never logged: they carry
// passwords, contact details and message content.
func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		started := time.Now()
		resp, err := handler(ctx, req)

		attrs := []any{
			slog.String("method", info.FullMethod),
			slog.String(logging.FieldActorID, ActorID(ctx)),
			slog.String(logging.FieldRequestID, RequestID(ctx)),
			slog.Duration("duration", time.Since(started)),
		}
		entry := observability.LoggerWith(ctx, logger)
		if err != nil {
			code := errs.CodeOf(err)
			attrs = append(attrs, slog.String(logging.FieldErrorCode, string(code)))
			if code == errs.CodeInternal {
				entry.Error("rpc failed", append(attrs, slog.String("error", err.Error()))...)
			} else {
				entry.Info("rpc rejected", attrs...)
			}
			return resp, err
		}
		entry.Info("rpc served", attrs...)
		return resp, nil
	}
}

// normalizeErrorInterceptor guarantees that every error leaving a service
// carries a contract error code. An error that is neither a domain error nor
// an already-formed status becomes INTERNAL_ERROR with a generic message, so
// an accidental database or driver string can never reach a client.
func normalizeErrorInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		var domainErr *errs.Error
		if errors.As(err, &domainErr) {
			return resp, domainErr
		}
		if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
			return resp, err
		}
		if ctx.Err() != nil {
			return resp, status.Error(codes.DeadlineExceeded, "调用超时")
		}
		return resp, errs.New(errs.CodeInternal, "服务暂时不可用")
	}
}
