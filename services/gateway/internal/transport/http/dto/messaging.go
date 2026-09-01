package dto

type ConversationProduct struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	CoverURL *string `json:"cover_url"`
	Status   string  `json:"status"`
}

type LastMessage struct {
	ID        string `json:"id"`
	SenderID  string `json:"sender_id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type Conversation struct {
	ID            string              `json:"id"`
	Product       ConversationProduct `json:"product"`
	Buyer         UserPublic          `json:"buyer"`
	Seller        UserPublic          `json:"seller"`
	OtherUser     UserPublic          `json:"other_user"`
	LastMessage   *LastMessage        `json:"last_message"`
	UnreadCount   int64               `json:"unread_count"`
	CreatedAt     string              `json:"created_at"`
	LastMessageAt *string             `json:"last_message_at"`
}

type ConversationPage struct {
	Items    []Conversation `json:"items"`
	Page     int32          `json:"page"`
	PageSize int32          `json:"page_size"`
	Total    int64          `json:"total"`
}

type Message struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	Sender         UserPublic `json:"sender"`
	Content        string     `json:"content"`
	ReadAt         *string    `json:"read_at"`
	CreatedAt      string     `json:"created_at"`
}

type MessagePage struct {
	Items      []Message `json:"items"`
	NextBefore *string   `json:"next_before"`
}

type UnreadCount struct {
	UnreadCount int64 `json:"unread_count"`
}

type SendMessageRequest struct {
	Content string `json:"content"`
}

type MarkReadRequest struct {
	LastMessageID string `json:"last_message_id"`
}
