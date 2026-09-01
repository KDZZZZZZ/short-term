package mapper

import (
	"fmt"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

func Conversation(
	src *messagingv1.ConversationItem,
	actorID string,
	products map[string]*marketplacev1.ProductSummary,
	users map[string]*accountv1.UserPublic,
) (dto.Conversation, error) {
	if src == nil {
		return dto.Conversation{}, fmt.Errorf("messaging conversation is missing")
	}
	product := products[src.GetProductId()]
	if product == nil {
		return dto.Conversation{}, fmt.Errorf("conversation product %q is missing from Marketplace", src.GetProductId())
	}
	otherID := src.GetBuyerId()
	if actorID == src.GetBuyerId() {
		otherID = src.GetSellerId()
	} else if actorID != src.GetSellerId() {
		return dto.Conversation{}, fmt.Errorf("actor %q is not a conversation participant", actorID)
	}

	var last *dto.LastMessage
	if value := src.GetLastMessage(); value != nil {
		last = &dto.LastMessage{
			ID: value.GetId(), SenderID: value.GetSenderId(), Content: value.GetContent(),
			CreatedAt: Timestamp(value.GetCreatedAt()),
		}
	}
	return dto.Conversation{
		ID: src.GetId(),
		Product: dto.ConversationProduct{
			ID: product.GetId(), Title: product.GetTitle(), CoverURL: product.CoverUrl,
			Status: ProductStatus(product.GetStatus()),
		},
		Buyer:         UserPublic(src.GetBuyerId(), users[src.GetBuyerId()]),
		Seller:        UserPublic(src.GetSellerId(), users[src.GetSellerId()]),
		OtherUser:     UserPublic(otherID, users[otherID]),
		LastMessage:   last,
		UnreadCount:   src.GetUnreadCount(),
		CreatedAt:     Timestamp(src.GetCreatedAt()),
		LastMessageAt: OptionalTimestamp(src.GetLastMessageAt()),
	}, nil
}

func ConversationPage(
	src *messagingv1.ConversationPage,
	actorID string,
	products map[string]*marketplacev1.ProductSummary,
	users map[string]*accountv1.UserPublic,
) (dto.ConversationPage, error) {
	if src == nil {
		return dto.ConversationPage{}, fmt.Errorf("messaging conversation page is missing")
	}
	items := make([]dto.Conversation, 0, len(src.GetItems()))
	for _, item := range src.GetItems() {
		mapped, err := Conversation(item, actorID, products, users)
		if err != nil {
			return dto.ConversationPage{}, err
		}
		items = append(items, mapped)
	}
	return dto.ConversationPage{
		Items: items, Page: src.GetPage(), PageSize: src.GetPageSize(), Total: src.GetTotal(),
	}, nil
}

func ConversationProductIDs(items []*messagingv1.ConversationItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.GetProductId())
	}
	return ids
}

func ConversationParticipantIDs(items []*messagingv1.ConversationItem) []string {
	ids := make([]string, 0, len(items)*2)
	for _, item := range items {
		ids = append(ids, item.GetBuyerId(), item.GetSellerId())
	}
	return ids
}

func Message(src *messagingv1.MessageItem, users map[string]*accountv1.UserPublic) dto.Message {
	return dto.Message{
		ID: src.GetId(), ConversationID: src.GetConversationId(),
		Sender:  UserPublic(src.GetSenderId(), users[src.GetSenderId()]),
		Content: src.GetContent(), ReadAt: OptionalTimestamp(src.GetReadAt()),
		CreatedAt: Timestamp(src.GetCreatedAt()),
	}
}

func MessagePage(src *messagingv1.MessagePage, users map[string]*accountv1.UserPublic) dto.MessagePage {
	items := make([]dto.Message, 0, len(src.GetItems()))
	for _, item := range src.GetItems() {
		items = append(items, Message(item, users))
	}
	return dto.MessagePage{Items: items, NextBefore: src.NextBefore}
}

func MessageSenderIDs(items []*messagingv1.MessageItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.GetSenderId())
	}
	return ids
}
