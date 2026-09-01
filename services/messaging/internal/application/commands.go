package application

// Public pagination limits mirror openapi/components/parameters.yaml.
const (
	DefaultPageSize     int32 = 20
	MaxPageSize         int32 = 100
	DefaultMessageLimit int32 = 30
	MaxMessageLimit     int32 = 100
)

type Page struct {
	Number int32
	Size   int32
}

func (p Page) normalize() Page {
	if p.Number < 1 {
		p.Number = 1
	}
	if p.Size < 1 {
		p.Size = DefaultPageSize
	}
	if p.Size > MaxPageSize {
		p.Size = MaxPageSize
	}
	return p
}

func (p Page) Offset() int32 { return (p.Number - 1) * p.Size }

type GetOrCreateConversationCommand struct {
	ActorID        string
	ProductID      string
	IdempotencyKey *string
}

type SendMessageCommand struct {
	ActorID        string
	ConversationID string
	Content        string
	IdempotencyKey *string
}

type MarkReadCommand struct {
	ActorID        string
	ConversationID string
	LastMessageID  string
}

type ListConversationsQuery struct {
	ActorID string
	Page    Page
}

type ListMessagesQuery struct {
	ActorID        string
	ConversationID string
	Before         *string
	Limit          int32
}

func normalizeMessageLimit(limit int32) int32 {
	if limit < 1 {
		return DefaultMessageLimit
	}
	if limit > MaxMessageLimit {
		return MaxMessageLimit
	}
	return limit
}
