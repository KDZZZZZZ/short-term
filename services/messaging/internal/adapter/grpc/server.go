// Package grpc exposes Messaging application use cases over the internal Proto API.
package grpc

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/domain"
)

type Server struct{ service *application.Service }

func NewServer(service *application.Service) *Server { return &Server{service: service} }

var _ messagingv1.MessagingServiceServer = (*Server)(nil)

func (s *Server) GetOrCreateConversation(ctx context.Context, req *messagingv1.GetOrCreateConversationRequest) (*messagingv1.GetOrCreateConversationResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	result, err := s.service.GetOrCreateConversation(ctx, application.GetOrCreateConversationCommand{
		ActorID: req.GetActorId(), ProductID: req.GetProductId(), IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	return &messagingv1.GetOrCreateConversationResponse{
		Conversation: conversationProto(
			result.View.Conversation, result.View.LastMessage, result.View.UnreadCount,
		),
		Replayed: result.Replayed,
	}, nil
}

func (s *Server) GetConversation(ctx context.Context, req *messagingv1.GetConversationRequest) (*messagingv1.GetConversationResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	conversation, err := s.service.GetConversation(ctx, req.GetActorId(), req.GetConversationId())
	if err != nil {
		return nil, err
	}
	return &messagingv1.GetConversationResponse{Conversation: conversationProto(conversation, nil, 0)}, nil
}

func (s *Server) ListConversations(ctx context.Context, req *messagingv1.ListConversationsRequest) (*messagingv1.ListConversationsResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	page, err := s.service.ListConversations(ctx, application.ListConversationsQuery{
		ActorID: req.GetActorId(), Page: application.Page{Number: req.GetPage(), Size: req.GetPageSize()},
	})
	if err != nil {
		return nil, err
	}
	items := make([]*messagingv1.ConversationItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, conversationProto(item.Conversation, item.LastMessage, item.UnreadCount))
	}
	return &messagingv1.ListConversationsResponse{Page: &messagingv1.ConversationPage{
		Items: items, Page: page.Page, PageSize: page.Size, Total: page.Total,
	}}, nil
}

func (s *Server) GetUnreadCount(ctx context.Context, req *messagingv1.GetUnreadCountRequest) (*messagingv1.GetUnreadCountResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	count, err := s.service.UnreadCount(ctx, req.GetActorId())
	if err != nil {
		return nil, err
	}
	return &messagingv1.GetUnreadCountResponse{UnreadCount: count}, nil
}

func (s *Server) ListMessages(ctx context.Context, req *messagingv1.ListMessagesRequest) (*messagingv1.ListMessagesResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	page, err := s.service.ListMessages(ctx, application.ListMessagesQuery{
		ActorID: req.GetActorId(), ConversationID: req.GetConversationId(), Before: req.Before, Limit: req.GetLimit(),
	})
	if err != nil {
		return nil, err
	}
	items := make([]*messagingv1.MessageItem, 0, len(page.Items))
	for _, message := range page.Items {
		items = append(items, messageProto(message))
	}
	return &messagingv1.ListMessagesResponse{Page: &messagingv1.MessagePage{
		Items: items, NextBefore: page.NextBefore,
	}}, nil
}

func (s *Server) SendMessage(ctx context.Context, req *messagingv1.SendMessageRequest) (*messagingv1.SendMessageResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	result, err := s.service.SendMessage(ctx, application.SendMessageCommand{
		ActorID: req.GetActorId(), ConversationID: req.GetConversationId(),
		Content: req.GetContent(), IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	return &messagingv1.SendMessageResponse{Message: messageProto(result.Message), Replayed: result.Replayed}, nil
}

func (s *Server) MarkConversationRead(ctx context.Context, req *messagingv1.MarkConversationReadRequest) (*messagingv1.MarkConversationReadResponse, error) {
	if err := requireActor(ctx, req.GetActorId()); err != nil {
		return nil, err
	}
	err := s.service.MarkConversationRead(ctx, application.MarkReadCommand{
		ActorID: req.GetActorId(), ConversationID: req.GetConversationId(), LastMessageID: req.GetLastMessageId(),
	})
	if err != nil {
		return nil, err
	}
	return &messagingv1.MarkConversationReadResponse{}, nil
}

// requireActor prevents an internal caller from placing a different user in
// the request body than the authenticated actor propagated by Gateway.
func requireActor(ctx context.Context, actorID string) error {
	if actorID == "" {
		return errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if authenticated := grpcx.ActorID(ctx); authenticated != "" && authenticated != actorID {
		return errs.New(errs.CodeForbidden, "无权执行该操作")
	}
	return nil
}

func conversationProto(conversation *domain.Conversation, last *domain.Message, unread int64) *messagingv1.ConversationItem {
	item := &messagingv1.ConversationItem{
		Id: conversation.ID, ProductId: conversation.ProductID,
		BuyerId: conversation.BuyerID, SellerId: conversation.SellerID,
		UnreadCount: unread, CreatedAt: timestamppb.New(conversation.CreatedAt),
		LastMessageAt: optionalTimestamp(conversation.LastMessageAt),
	}
	if last != nil {
		item.LastMessage = &messagingv1.LastMessage{
			Id: last.ID, SenderId: last.SenderID, Content: last.Content,
			CreatedAt: timestamppb.New(last.CreatedAt),
		}
	}
	return item
}

func messageProto(message *domain.Message) *messagingv1.MessageItem {
	return &messagingv1.MessageItem{
		Id: message.ID, ConversationId: message.ConversationID, SenderId: message.SenderID,
		Content: message.Content, ReadAt: optionalTimestamp(message.ReadAt),
		CreatedAt: timestamppb.New(message.CreatedAt),
	}
}

func optionalTimestamp(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timestamppb.New(*value)
}
