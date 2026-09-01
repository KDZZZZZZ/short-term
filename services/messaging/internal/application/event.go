package application

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/KDZZZZZZ/short-term/services/messaging/internal/domain"
)

const EventSchemaVersion int32 = 1

type eventFactory struct {
	newID   func() string
	now     time.Time
	traceID string
}

type messageSentPayload struct {
	MessageID      string `json:"message_id"`
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
}

type conversationReadPayload struct {
	ConversationID string    `json:"conversation_id"`
	ActorID        string    `json:"actor_id"`
	LastMessageID  string    `json:"last_message_id"`
	ReadAt         time.Time `json:"read_at"`
}

func (f eventFactory) messageSent(message *domain.Message) (Event, error) {
	payload, err := json.Marshal(messageSentPayload{
		MessageID: message.ID, ConversationID: message.ConversationID, SenderID: message.SenderID,
	})
	if err != nil {
		return Event{}, fmt.Errorf("application: encode message event: %w", err)
	}
	return f.event(EventMessageSent, AggregateMessage, message.ID, payload), nil
}

func (f eventFactory) conversationRead(conversationID, actorID, lastMessageID string) (Event, error) {
	payload, err := json.Marshal(conversationReadPayload{
		ConversationID: conversationID, ActorID: actorID, LastMessageID: lastMessageID, ReadAt: f.now,
	})
	if err != nil {
		return Event{}, fmt.Errorf("application: encode read event: %w", err)
	}
	return f.event(EventConversationRead, AggregateConversation, conversationID, payload), nil
}

func (f eventFactory) event(eventType, aggregateType, aggregateID string, payload []byte) Event {
	return Event{
		ID: f.newID(), Type: eventType, SchemaVersion: EventSchemaVersion,
		AggregateType: aggregateType, AggregateID: aggregateID,
		OccurredAt: f.now, TraceID: f.traceID, Payload: payload,
	}
}
