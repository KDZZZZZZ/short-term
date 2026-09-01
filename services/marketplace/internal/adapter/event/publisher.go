// Package event 实现 Outbox 发布器及其传输。
package event

import (
	"context"
	"log/slog"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/events/v1"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
)

// Envelope 将 Outbox 记录渲染为 proto/shortterm/events/v1 定义的共享线路信封。
func Envelope(event application.Event) *eventsv1.EventEnvelope {
	return &eventsv1.EventEnvelope{
		EventId:       event.ID,
		EventType:     event.Type,
		SchemaVersion: event.SchemaVersion,
		AggregateType: event.AggregateType,
		AggregateId:   event.AggregateID,
		OccurredAt:    timestamppb.New(event.OccurredAt),
		TraceId:       event.TraceID,
		Payload:       event.Payload,
	}
}

// LogPublisher 将每个事件写入进程日志，而不是写入消息代理。
//
// 消息代理产品、分区方式和保留策略仍待决定（docs/software-design.md 第 11.3 节），
// MVP 目前也没有消费者使用这些事件。在此期间，该发布器维持 Outbox 的正确语义：
// 事件会被产生、在每次成功尝试中恰好交付一次并标记为已发布，因此换成真正的消息代理
// 时只需修改此文件。
type LogPublisher struct {
	logger *slog.Logger
}

// NewLogPublisher 构造一个写入日志的发布器。
func NewLogPublisher(logger *slog.Logger) *LogPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogPublisher{logger: logger}
}

var _ application.EventPublisher = (*LogPublisher)(nil)

// Publish 记录事件。
//
// 不记录 payload：其中包含买家和卖家标识，随着 schema 演进还可能包含更多信息。
// 仅记录信封就足以表明事件已交付。
func (p *LogPublisher) Publish(_ context.Context, event application.Event) error {
	envelope := Envelope(event)
	size := proto.Size(envelope)

	p.logger.Info("outbox event published",
		slog.String("event_id", event.ID),
		slog.String("event_type", event.Type),
		slog.String("aggregate_type", event.AggregateType),
		slog.String("aggregate_id", event.AggregateID),
		slog.Int("schema_version", int(event.SchemaVersion)),
		slog.String(logging.FieldTraceID, event.TraceID),
		slog.Int("envelope_bytes", size),
	)
	return nil
}
