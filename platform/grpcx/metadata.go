// Package grpcx builds the gRPC servers and clients used between services and
// carries the cross-cutting policy they share: deadlines, recovery, structured
// access logs, trace propagation and error normalisation.
package grpcx

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// Metadata keys propagated on every internal call. Business authorization is
// still performed by the service that owns the resource; actor identity is
// context, never proof of permission.
const (
	MetadataActorID   = "x-shortterm-actor-id"
	MetadataRequestID = "x-shortterm-request-id"
	MetadataCaller    = "x-shortterm-caller"
)

// WithActor attaches the acting user identity to an outgoing call.
func WithActor(ctx context.Context, actorID string) context.Context {
	if actorID == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataActorID, actorID)
}

// WithRequestID attaches the public request identifier to an outgoing call.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataRequestID, requestID)
}

// ActorID reads the acting user identity from an incoming call.
func ActorID(ctx context.Context) string { return incoming(ctx, MetadataActorID) }

// RequestID reads the public request identifier from an incoming call.
func RequestID(ctx context.Context) string { return incoming(ctx, MetadataRequestID) }

// metadataAppend adds one key/value pair to the outgoing metadata.
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
