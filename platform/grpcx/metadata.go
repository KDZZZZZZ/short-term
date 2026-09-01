// Package grpcx 构造服务之间使用的 gRPC 服务端和客户端，并承载它们共享的
// 横切策略：截止时间、恢复、结构化访问日志、追踪传递和错误规范化。
package grpcx

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// 每次内部调用都会传递的元数据键。业务授权仍由资源所属服务执行；
// 当前用户身份只是上下文信息，绝不是权限证明。
const (
	MetadataActorID   = "x-shortterm-actor-id"
	MetadataRequestID = "x-shortterm-request-id"
	MetadataCaller    = "x-shortterm-caller"
)

// WithActor 将当前用户身份附加到出站调用。
func WithActor(ctx context.Context, actorID string) context.Context {
	if actorID == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataActorID, actorID)
}

// WithRequestID 将公开请求标识附加到出站调用。
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataRequestID, requestID)
}

// ActorID 从入站调用中读取当前用户身份。
func ActorID(ctx context.Context) string { return incoming(ctx, MetadataActorID) }

// RequestID 从入站调用中读取公开请求标识。
func RequestID(ctx context.Context) string { return incoming(ctx, MetadataRequestID) }

// metadataAppend 向出站元数据添加一个键值对。
func metadataAppend(ctx context.Context, key, value string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, key, value)
}

func incoming(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
