// Package event publishes Messaging Outbox envelopes.
package event

import (
	"context"
	"log/slog"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/events/v1"
	"github.com/KDZZZZZZ/short-term/platform/logging"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
)

func Envelope(event application.Event) *eventsv1.EventEnvelope {
	return &eventsv1.EventEnvelope{
		EventId: event.ID, EventType: event.Type, SchemaVersion: event.SchemaVersion,
		AggregateType: event.AggregateType, AggregateId: event.AggregateID,
		OccurredAt: timestamppb.New(event.OccurredAt), TraceId: event.TraceID, Payload: event.Payload,
	}
}

type LogPublisher struct{ logger *slog.Logger }

func NewLogPublisher(logger *slog.Logger) *LogPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogPublisher{logger: logger}
}

var _ application.EventPublisher = (*LogPublisher)(nil)

// Publish deliberately excludes payload because future message events may
// carry private data. The envelope metadata is sufficient for the placeholder.
func (p *LogPublisher) Publish(_ context.Context, event application.Event) error {
	p.logger.Info("messaging outbox event published",
		slog.String("event_id", event.ID),
		slog.String("event_type", event.Type),
		slog.String("aggregate_type", event.AggregateType),
		slog.String("aggregate_id", event.AggregateID),
		slog.Int("schema_version", int(event.SchemaVersion)),
		slog.String(logging.FieldTraceID, event.TraceID),
		slog.Int("envelope_bytes", proto.Size(Envelope(event))),
	)
	return nil
}
