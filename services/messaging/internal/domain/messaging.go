// Package domain contains the Messaging Service business invariants.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxMessageRunes = 1000

var (
	ErrInvalidConversation = errors.New("messaging: invalid conversation")
	ErrSelfConversation    = errors.New("messaging: buyer and seller must differ")
	ErrInvalidMessage      = errors.New("messaging: invalid message")
	ErrInvalidContent      = errors.New("messaging: content must contain 1 to 1000 characters")
)

// Conversation is the local participant and product context for a private chat.
type Conversation struct {
	ID            string
	ProductID     string
	BuyerID       string
	SellerID      string
	CreatedAt     time.Time
	LastMessageAt *time.Time
}

// NewConversation constructs a product-context conversation.
func NewConversation(id, productID, buyerID, sellerID string, at time.Time) (*Conversation, error) {
	if id == "" || productID == "" || buyerID == "" || sellerID == "" || at.IsZero() {
		return nil, ErrInvalidConversation
	}
	if buyerID == sellerID {
		return nil, ErrSelfConversation
	}
	return &Conversation{
		ID: id, ProductID: productID, BuyerID: buyerID, SellerID: sellerID,
		CreatedAt: at.UTC(),
	}, nil
}

// IsParticipant reports whether actorID is one of the two conversation parties.
func (c *Conversation) IsParticipant(actorID string) bool {
	return actorID != "" && (actorID == c.BuyerID || actorID == c.SellerID)
}

// OtherParticipant returns the opposite party for a participant.
func (c *Conversation) OtherParticipant(actorID string) (string, bool) {
	switch actorID {
	case c.BuyerID:
		return c.SellerID, true
	case c.SellerID:
		return c.BuyerID, true
	default:
		return "", false
	}
}

// Message is one text message in a conversation.
type Message struct {
	ID             string
	ConversationID string
	SenderID       string
	Content        string
	ReadAt         *time.Time
	CreatedAt      time.Time
}

// NewMessage validates sender participation and content length.
func NewMessage(id string, conversation *Conversation, senderID, content string, at time.Time) (*Message, error) {
	if id == "" || conversation == nil || senderID == "" || at.IsZero() {
		return nil, ErrInvalidMessage
	}
	if !conversation.IsParticipant(senderID) {
		return nil, ErrInvalidMessage
	}
	if err := ValidateContent(content); err != nil {
		return nil, err
	}
	return &Message{
		ID: id, ConversationID: conversation.ID, SenderID: senderID,
		Content: content, CreatedAt: at.UTC(),
	}, nil
}

// ValidateContent follows the OpenAPI 1..1000 character constraint. Rune count
// is used rather than byte length so a Chinese character counts as one character.
func ValidateContent(content string) error {
	if !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
		return ErrInvalidContent
	}
	length := utf8.RuneCountInString(content)
	if length < 1 || length > MaxMessageRunes {
		return ErrInvalidContent
	}
	return nil
}
