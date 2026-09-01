package handler

import (
	"context"
	"net/http"
	"strconv"
	"unicode/utf8"

	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// Messaging serves product conversations, text messages and unread state.
type Messaging struct {
	messaging  messagingv1.MessagingServiceClient
	aggregator *aggregation.Aggregator
	responder  Responder
}

const (
	messagingIdempotencyKeyHeader = "Idempotency-Key"
	messagingMinIdempotencyLength = 16
	messagingMaxIdempotencyLength = 128
)

func NewMessaging(client messagingv1.MessagingServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Messaging {
	return &Messaging{messaging: client, aggregator: aggregator, responder: responder}
}

func (h *Messaging) GetOrCreate(w http.ResponseWriter, r *http.Request) {
	key, ok := h.idempotencyKey(w, r)
	if !ok {
		return
	}
	actorID := middleware.ActorID(r.Context())
	resp, err := h.messaging.GetOrCreateConversation(h.downstream(r), &messagingv1.GetOrCreateConversationRequest{
		ActorId: actorID, ProductId: r.PathValue("productId"), IdempotencyKey: key,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.respondConversation(w, r, resp.GetConversation())
}

func (h *Messaging) List(w http.ResponseWriter, r *http.Request) {
	page, ok := h.intQuery(w, r, "page", 1, 1, 0)
	if !ok {
		return
	}
	size, ok := h.intQuery(w, r, "page_size", 20, 1, 100)
	if !ok {
		return
	}
	actorID := middleware.ActorID(r.Context())
	ctx := h.downstream(r)
	resp, err := h.messaging.ListConversations(ctx, &messagingv1.ListConversationsRequest{
		ActorId: actorID, Page: page, PageSize: size,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	conversationPage := resp.GetPage()
	if conversationPage == nil {
		h.responder.Error(w, r, errs.New(errs.CodeInternal, "服务暂时不可用"))
		return
	}
	products, err := h.aggregator.Products(ctx, mapper.ConversationProductIDs(conversationPage.GetItems()))
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	users, err := h.aggregator.Users(ctx, mapper.ConversationParticipantIDs(conversationPage.GetItems()))
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	mapped, err := mapper.ConversationPage(conversationPage, actorID, products, users)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.OK(w, r, mapped)
}

func (h *Messaging) UnreadCount(w http.ResponseWriter, r *http.Request) {
	resp, err := h.messaging.GetUnreadCount(h.downstream(r), &messagingv1.GetUnreadCountRequest{
		ActorId: middleware.ActorID(r.Context()),
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.OK(w, r, dto.UnreadCount{UnreadCount: resp.GetUnreadCount()})
}

func (h *Messaging) ListMessages(w http.ResponseWriter, r *http.Request) {
	limit, ok := h.intQuery(w, r, "limit", 30, 1, 100)
	if !ok {
		return
	}
	var before *string
	if values, present := r.URL.Query()["before"]; present {
		if len(values) != 1 || utf8.RuneCountInString(values[0]) < 1 || utf8.RuneCountInString(values[0]) > 256 {
			h.responder.Fail(w, r, errs.CodeValidation, "before 参数不合法")
			return
		}
		before = &values[0]
	}
	ctx := h.downstream(r)
	resp, err := h.messaging.ListMessages(ctx, &messagingv1.ListMessagesRequest{
		ActorId: middleware.ActorID(r.Context()), ConversationId: r.PathValue("conversationId"),
		Before: before, Limit: limit,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	page := resp.GetPage()
	if page == nil {
		h.responder.Error(w, r, errs.New(errs.CodeInternal, "服务暂时不可用"))
		return
	}
	users, err := h.aggregator.Users(ctx, mapper.MessageSenderIDs(page.GetItems()))
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.OK(w, r, mapper.MessagePage(page, users))
}

func (h *Messaging) SendMessage(w http.ResponseWriter, r *http.Request) {
	var body dto.SendMessageRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}
	if length := utf8.RuneCountInString(body.Content); !utf8.ValidString(body.Content) || length < 1 || length > 1000 {
		h.responder.Fail(w, r, errs.CodeValidation, "消息长度必须为 1 至 1000 个字符")
		return
	}
	key, ok := h.idempotencyKey(w, r)
	if !ok {
		return
	}
	actorID := middleware.ActorID(r.Context())
	ctx := h.downstream(r)
	resp, err := h.messaging.SendMessage(ctx, &messagingv1.SendMessageRequest{
		ActorId: actorID, ConversationId: r.PathValue("conversationId"),
		Content: body.Content, IdempotencyKey: key,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	message := resp.GetMessage()
	if message == nil {
		h.responder.Error(w, r, errs.New(errs.CodeInternal, "服务暂时不可用"))
		return
	}
	users, err := h.aggregator.Users(ctx, []string{message.GetSenderId()})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.Created(w, r, mapper.Message(message, users))
}

func (h *Messaging) MarkRead(w http.ResponseWriter, r *http.Request) {
	var body dto.MarkReadRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}
	if length := utf8.RuneCountInString(body.LastMessageID); !utf8.ValidString(body.LastMessageID) || length < 1 || length > 64 {
		h.responder.Fail(w, r, errs.CodeValidation, "last_message_id 不合法")
		return
	}
	_, err := h.messaging.MarkConversationRead(h.downstream(r), &messagingv1.MarkConversationReadRequest{
		ActorId: middleware.ActorID(r.Context()), ConversationId: r.PathValue("conversationId"),
		LastMessageId: body.LastMessageID,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.Empty(w, r)
}

func (h *Messaging) respondConversation(w http.ResponseWriter, r *http.Request, item *messagingv1.ConversationItem) {
	if item == nil {
		h.responder.Error(w, r, errs.New(errs.CodeInternal, "服务暂时不可用"))
		return
	}
	ctx := h.downstream(r)
	products, err := h.aggregator.Products(ctx, []string{item.GetProductId()})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	users, err := h.aggregator.Users(ctx, []string{item.GetBuyerId(), item.GetSellerId()})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	mapped, err := mapper.Conversation(item, middleware.ActorID(r.Context()), products, users)
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}
	h.responder.OK(w, r, mapped)
}

func (h *Messaging) idempotencyKey(w http.ResponseWriter, r *http.Request) (*string, bool) {
	raw := r.Header.Get(messagingIdempotencyKeyHeader)
	if raw == "" {
		return nil, true
	}
	length := utf8.RuneCountInString(raw)
	if !utf8.ValidString(raw) || length < messagingMinIdempotencyLength || length > messagingMaxIdempotencyLength {
		h.responder.Fail(w, r, errs.CodeValidation, "Idempotency-Key 长度必须为 16 至 128 个字符")
		return nil, false
	}
	return &raw, true
}

func (h *Messaging) intQuery(w http.ResponseWriter, r *http.Request, name string, fallback, minimum, maximum int32) (int32, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || int32(value) < minimum || (maximum > 0 && int32(value) > maximum) {
		h.responder.Fail(w, r, errs.CodeValidation, name+" 参数不合法")
		return 0, false
	}
	return int32(value), true
}

func (h *Messaging) downstream(r *http.Request) context.Context {
	ctx := grpcx.WithActor(r.Context(), middleware.ActorID(r.Context()))
	return grpcx.WithRequestID(ctx, middleware.RequestID(r.Context()))
}
