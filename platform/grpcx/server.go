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

// ServerOptions 配置内部 gRPC 服务端。
type ServerOptions struct {
	// Logger 为每次完成的调用接收一条结构化记录。
	Logger *slog.Logger
	// MaxRecvMsgSize 限制单个请求的大小。图片上传通过 AddProductImages 传输，
	// 因此默认值大于 gRPC 的 4MB 默认值。
	MaxRecvMsgSize int
	// HandlerTimeout 在调用方未发送截止时间时限制处理器执行时间。零值表示不限制，
	// 仅适合测试。
	HandlerTimeout time.Duration
}

// NewServer 使用共享拦截器链构造 gRPC 服务端。调用方在返回的服务端上注册服务，
// 然后调用 Serve。
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

// Serve 在 addr 上运行 srv，直到 ctx 被取消，然后优雅地停止服务。
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

// recoveryInterceptor 将 panic 转换为 INTERNAL_ERROR。堆栈写入日志；
// 调用方只会得知调用失败。
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

// timeoutInterceptor 限制调用方未发送截止时间的处理器，避免缺少客户端截止时间
// 而无限期占用数据库连接。
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

// loggingInterceptor 使用规范字段名为每次调用写入一条结构化记录。请求和响应正文
// 从不写入日志，因为其中包含密码、联系方式和消息内容。
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

// normalizeErrorInterceptor 确保离开服务的每个错误都携带契约错误码。
// 既不是领域错误、也不是已构造状态的错误会变成带通用消息的 INTERNAL_ERROR，
// 从而避免意外的数据库或驱动错误文本传给客户端。
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
