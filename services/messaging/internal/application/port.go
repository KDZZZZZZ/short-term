package application

import (
	"context"
	"errors"
	"time"

	"github.com/KDZZZZZZ/short-term/services/messaging/internal/domain"
)

const (
	OpGetOrCreateConversation       = "conversation.get_or_create"
	OpSendMessage                   = "message.send"
	SnapshotSchemaVersion     int32 = 1
	MinIdempotencyKeyLength         = 16
	MaxIdempotencyKeyLength         = 128
)

var (
	ErrNotFound        = errors.New("messaging: resource not found")
	ErrIdempotencyRace = errors.New("messaging: concurrent idempotency request committed first")
)

type Product struct {
	ID       string
	SellerID string
}

type ProductReader interface {
	Get(ctx context.Context, productID string) (Product, error)
}

type IDGenerator interface {
	NewConversationID() string
	NewMessageID() string
	NewEventID() string
}

type Clock interface {
	Now() time.Time
}

type IdempotencyKey struct {
	ActorID   string
	Operation string
	Key       string
}

type CommandResult struct {
	Code             string
	ConversationView *ConversationView
	Message          *domain.Message
}

type Event struct {
	ID            string
	Type          string
	SchemaVersion int32
	AggregateType string
	AggregateID   string
	OccurredAt    time.Time
	TraceID       string
	Payload       []byte
}

const (
	EventMessageSent      = "messaging.message.sent"
	EventConversationRead = "messaging.conversation.read"
	AggregateMessage      = "message"
	AggregateConversation = "conversation"
)

type Tx interface {
	GetOrCreateConversation(ctx context.Context, candidate *domain.Conversation) (conversation *domain.Conversation, created bool, err error)
	ConversationView(ctx context.Context, conversationID, actorID string) (ConversationView, error)
	LockConversation(ctx context.Context, conversationID string) (*domain.Conversation, error)
	LockMessage(ctx context.Context, conversationID, messageID string) (*domain.Message, error)
	InsertMessage(ctx context.Context, message *domain.Message) error
	TouchConversation(ctx context.Context, conversationID string, at time.Time) error
	MarkOpponentMessagesRead(ctx context.Context, conversationID, actorID string, through time.Time, throughID string, at time.Time) (int64, error)
	AppendEvent(ctx context.Context, event Event) error
}

type ConversationView struct {
	Conversation *domain.Conversation
	LastMessage  *domain.Message
	UnreadCount  int64
}

type ConversationPage struct {
	Items []ConversationView
	Page  int32
	Size  int32
	Total int64
}

type MessageCursor struct {
	CreatedAt time.Time
	ID        string
}

type MessagePage struct {
	Items      []*domain.Message
	NextBefore *string
}

type Repository interface {
	Execute(ctx context.Context, key *IdempotencyKey, fn func(context.Context, Tx) (*CommandResult, error)) (result *CommandResult, replayed bool, err error)
	Transact(ctx context.Context, fn func(context.Context, Tx) error) error
	ConversationByID(ctx context.Context, conversationID string) (*domain.Conversation, error)
	ListConversations(ctx context.Context, actorID string, page Page) (ConversationPage, error)
	ListMessages(ctx context.Context, conversationID string, before *MessageCursor, limit int32) ([]*domain.Message, error)
	UnreadCount(ctx context.Context, actorID string) (int64, error)
}

type OutboxRepository interface {
	Pending(ctx context.Context, limit int32) ([]Event, error)
	MarkPublished(ctx context.Context, eventID string, at time.Time) error
	MarkFailed(ctx context.Context, eventID string, cause string) error
}

type EventPublisher interface {
	Publish(context.Context, Event) error
}
