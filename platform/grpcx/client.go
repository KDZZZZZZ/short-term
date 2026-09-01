package grpcx

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientOptions 配置到内部服务的连接。
type ClientOptions struct {
	// Target 是拨号目标，例如 "account:9001"。
	Target string
	// Caller 在下游日志中标识发起调用的可部署单元。
	Caller string
	// DefaultTimeout 限制上下文未携带截止时间的调用。
	// docs/software-design.md 第 7.2 节要求每次下游调用都必须有截止时间，
	// 因此零值会被拒绝。
	DefaultTimeout time.Duration
	// MaxRecvMsgSize 限制单个响应的大小。
	MaxRecvMsgSize int
}

// Dial 构造带有追踪、默认截止时间，并在每次调用中附加调用方身份的客户端连接。
//
// 连接使用明文：内部 gRPC 只能通过私有容器网络访问
// （docs/software-design.md 第 9.2 节）。选择 mTLS 还是工作负载身份是第 11.3 节
// 中尚未决定的事项；决定后只需在这里修改。
func Dial(opts ClientOptions) (*grpc.ClientConn, error) {
	if opts.Target == "" {
		return nil, fmt.Errorf("grpcx: client target is required")
	}
	if opts.DefaultTimeout <= 0 {
		return nil, fmt.Errorf("grpcx: default timeout is required for %s", opts.Target)
	}
	if opts.MaxRecvMsgSize <= 0 {
		opts.MaxRecvMsgSize = 16 << 20
	}

	conn, err := grpc.NewClient(opts.Target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(opts.MaxRecvMsgSize)),
		grpc.WithChainUnaryInterceptor(
			deadlineInterceptor(opts.DefaultTimeout),
			callerInterceptor(opts.Caller),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("grpcx: dial %s: %w", opts.Target, err)
	}
	return conn, nil
}

// deadlineInterceptor 在调用方未设置截止时间时应用默认超时。继承而来的截止时间
// 保持不变，使剩余时间预算沿调用链传递，而不是在每一跳被延长。
func deadlineInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if _, ok := ctx.Deadline(); ok {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		bounded, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(bounded, method, req, reply, cc, opts...)
	}
}

// callerInterceptor 在每个请求上标记调用方服务名称。
func callerInterceptor(caller string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if caller != "" {
			ctx = metadataAppend(ctx, MetadataCaller, caller)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
