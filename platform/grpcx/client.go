package grpcx

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientOptions configures a connection to an internal service.
type ClientOptions struct {
	// Target is the dial target, for example "account:9001".
	Target string
	// Caller identifies the calling deployable unit in downstream logs.
	Caller string
	// DefaultTimeout bounds any call whose context carries no deadline.
	// docs/software-design.md section 7.2 requires every downstream call to
	// have one, so a zero value is rejected.
	DefaultTimeout time.Duration
	// MaxRecvMsgSize bounds a single response.
	MaxRecvMsgSize int
}

// Dial builds a client connection with tracing, a default deadline and the
// caller identity attached to every call.
//
// The connection is plaintext: internal gRPC is reachable only on the private
// container network (docs/software-design.md section 9.2). Choosing mTLS or
// workload identity is an open decision in section 11.3, and this is the
// single place that changes when it is settled.
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

// deadlineInterceptor applies the default timeout when the caller did not set
// one. An inherited deadline is left untouched so the remaining budget
// propagates down the chain instead of being extended at each hop.
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

// callerInterceptor stamps the calling service name on every request.
func callerInterceptor(caller string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if caller != "" {
			ctx = metadataAppend(ctx, MetadataCaller, caller)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
