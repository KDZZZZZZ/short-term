package handler_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

type messagingResponseCapture struct {
	status int
	data   any
	code   errs.Code
	err    error
}

func (c *messagingResponseCapture) OK(_ http.ResponseWriter, _ *http.Request, data any) {
	c.status, c.data = http.StatusOK, data
}
func (c *messagingResponseCapture) Created(_ http.ResponseWriter, _ *http.Request, data any) {
	c.status, c.data = http.StatusCreated, data
}
func (c *messagingResponseCapture) Success(_ http.ResponseWriter, _ *http.Request, status int, data any) {
	c.status, c.data = status, data
}
func (c *messagingResponseCapture) Empty(_ http.ResponseWriter, _ *http.Request) {
	c.status, c.data = http.StatusOK, struct{}{}
}
func (c *messagingResponseCapture) Error(_ http.ResponseWriter, _ *http.Request, err error) {
	c.code, c.err = errs.CodeOf(err), err
}
func (c *messagingResponseCapture) Fail(_ http.ResponseWriter, _ *http.Request, code errs.Code, _ string) {
	c.code = code
}

type messagingClientStub struct {
	messagingv1.MessagingServiceClient
	conversations []*messagingv1.ConversationItem
	messages      []*messagingv1.MessageItem
	lastCreate    *messagingv1.GetOrCreateConversationRequest
	lastSend      *messagingv1.SendMessageRequest
	lastRead      *messagingv1.MarkConversationReadRequest
	lastList      *messagingv1.ListMessagesRequest
	metadataActor string
	metadataReq   string
	listCalls     int
	sendCalls     int
}

func (s *messagingClientStub) capture(ctx context.Context) {
	md, _ := metadata.FromOutgoingContext(ctx)
	if values := md.Get(grpcx.MetadataActorID); len(values) > 0 {
		s.metadataActor = values[0]
	}
	if values := md.Get(grpcx.MetadataRequestID); len(values) > 0 {
		s.metadataReq = values[0]
	}
}

func (s *messagingClientStub) GetOrCreateConversation(ctx context.Context, req *messagingv1.GetOrCreateConversationRequest, _ ...grpc.CallOption) (*messagingv1.GetOrCreateConversationResponse, error) {
	s.capture(ctx)
	s.lastCreate = req
	return &messagingv1.GetOrCreateConversationResponse{Conversation: s.conversations[0]}, nil
}

func (s *messagingClientStub) ListConversations(ctx context.Context, req *messagingv1.ListConversationsRequest, _ ...grpc.CallOption) (*messagingv1.ListConversationsResponse, error) {
	s.capture(ctx)
	s.listCalls++
	return &messagingv1.ListConversationsResponse{Page: &messagingv1.ConversationPage{
		Items: s.conversations, Page: req.GetPage(), PageSize: req.GetPageSize(), Total: int64(len(s.conversations)),
	}}, nil
}

func (s *messagingClientStub) GetUnreadCount(context.Context, *messagingv1.GetUnreadCountRequest, ...grpc.CallOption) (*messagingv1.GetUnreadCountResponse, error) {
	return &messagingv1.GetUnreadCountResponse{UnreadCount: 7}, nil
}

func (s *messagingClientStub) ListMessages(ctx context.Context, req *messagingv1.ListMessagesRequest, _ ...grpc.CallOption) (*messagingv1.ListMessagesResponse, error) {
	s.capture(ctx)
	s.lastList = req
	next := "cursor-next"
	return &messagingv1.ListMessagesResponse{Page: &messagingv1.MessagePage{Items: s.messages, NextBefore: &next}}, nil
}

func (s *messagingClientStub) SendMessage(ctx context.Context, req *messagingv1.SendMessageRequest, _ ...grpc.CallOption) (*messagingv1.SendMessageResponse, error) {
	s.capture(ctx)
	s.sendCalls++
	s.lastSend = req
	if len(s.messages) == 0 {
		return &messagingv1.SendMessageResponse{}, nil
	}
	return &messagingv1.SendMessageResponse{Message: s.messages[0]}, nil
}

func (s *messagingClientStub) MarkConversationRead(ctx context.Context, req *messagingv1.MarkConversationReadRequest, _ ...grpc.CallOption) (*messagingv1.MarkConversationReadResponse, error) {
	s.capture(ctx)
	s.lastRead = req
	return &messagingv1.MarkConversationReadResponse{}, nil
}

type messagingAccountStub struct {
	accountv1.AccountServiceClient
	users map[string]*accountv1.UserPublic
	calls int
}

func (s *messagingAccountStub) BatchGetUsers(_ context.Context, _ *accountv1.BatchGetUsersRequest, _ ...grpc.CallOption) (*accountv1.BatchGetUsersResponse, error) {
	s.calls++
	return &accountv1.BatchGetUsersResponse{Users: s.users}, nil
}

type messagingMarketplaceStub struct {
	marketplacev1.MarketplaceServiceClient
	products map[string]*marketplacev1.ProductSummary
	calls    int
}

func (s *messagingMarketplaceStub) BatchGetProducts(_ context.Context, _ *marketplacev1.BatchGetProductsRequest, _ ...grpc.CallOption) (*marketplacev1.BatchGetProductsResponse, error) {
	s.calls++
	return &marketplacev1.BatchGetProductsResponse{Products: s.products}, nil
}

func TestMessagingListAggregatesCurrentProductAndParticipantFactsOnce(t *testing.T) {
	t.Parallel()
	now := timestamppb.New(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
	client := &messagingClientStub{conversations: []*messagingv1.ConversationItem{
		{Id: "c_1", ProductId: "p_1", BuyerId: "u_actor", SellerId: "u_seller_1", CreatedAt: now, UnreadCount: 2},
		{Id: "c_2", ProductId: "p_2", BuyerId: "u_actor", SellerId: "u_seller_2", CreatedAt: now},
	}}
	accounts := &messagingAccountStub{users: map[string]*accountv1.UserPublic{
		"u_actor":    {Id: "u_actor", Nickname: "买家"},
		"u_seller_1": {Id: "u_seller_1", Nickname: "卖家一"},
		"u_seller_2": {Id: "u_seller_2", Nickname: "卖家二"},
	}}
	marketplace := &messagingMarketplaceStub{products: map[string]*marketplacev1.ProductSummary{
		"p_1": {Id: "p_1", Title: "键盘", Status: marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED},
		"p_2": {Id: "p_2", Title: "课本", Status: marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD},
	}}
	responder := &messagingResponseCapture{}
	h := handler.NewMessaging(client, aggregation.New(accounts, marketplace, nil), responder)

	h.List(httptest.NewRecorder(), messagingRequest(http.MethodGet, "/conversations?page=2&page_size=10", nil))
	if responder.code != "" || responder.status != http.StatusOK {
		t.Fatalf("response = status %d code %s error %v", responder.status, responder.code, responder.err)
	}
	page, ok := responder.data.(dto.ConversationPage)
	if !ok || len(page.Items) != 2 {
		t.Fatalf("data = %#v (%T)", responder.data, responder.data)
	}
	if page.Items[0].Product.Status != "RESERVED" || page.Items[1].Product.Status != "SOLD" {
		t.Fatalf("statuses = %q, %q", page.Items[0].Product.Status, page.Items[1].Product.Status)
	}
	if page.Items[0].OtherUser.ID != "u_seller_1" || page.Items[0].OtherUser.Nickname != "卖家一" {
		t.Fatalf("other_user = %+v", page.Items[0].OtherUser)
	}
	if client.listCalls != 1 || accounts.calls != 1 || marketplace.calls != 1 {
		t.Fatalf("calls messaging/account/marketplace = %d/%d/%d", client.listCalls, accounts.calls, marketplace.calls)
	}
	if client.metadataActor != "u_actor" || client.metadataReq != "req_messaging" {
		t.Fatalf("metadata actor/request = %q/%q", client.metadataActor, client.metadataReq)
	}
}

func TestMessagingSendForwardsValidatedContentAndIdempotencyKey(t *testing.T) {
	t.Parallel()
	now := timestamppb.Now()
	client := &messagingClientStub{messages: []*messagingv1.MessageItem{
		{Id: "m_1", ConversationId: "c_1", SenderId: "u_actor", Content: "你好", CreatedAt: now},
	}}
	accounts := &messagingAccountStub{users: map[string]*accountv1.UserPublic{
		"u_actor": {Id: "u_actor", Nickname: "买家"},
	}}
	responder := &messagingResponseCapture{}
	h := handler.NewMessaging(client, aggregation.New(accounts, &messagingMarketplaceStub{}, nil), responder)
	request := messagingRequest(http.MethodPost, "/conversations/c_1/messages", []byte(`{"content":"你好"}`))
	request.SetPathValue("conversationId", "c_1")
	request.Header.Set("Idempotency-Key", "message-key-00000001")

	h.SendMessage(httptest.NewRecorder(), request)
	if responder.status != http.StatusCreated || responder.code != "" {
		t.Fatalf("response status/code = %d/%s", responder.status, responder.code)
	}
	message, ok := responder.data.(dto.Message)
	if !ok || message.Sender.Nickname != "买家" || message.Content != "你好" {
		t.Fatalf("message = %#v (%T)", responder.data, responder.data)
	}
	if client.lastSend.GetActorId() != "u_actor" || client.lastSend.GetConversationId() != "c_1" || client.lastSend.GetIdempotencyKey() != "message-key-00000001" {
		t.Fatalf("downstream request = %+v", client.lastSend)
	}

	invalidCapture := &messagingResponseCapture{}
	invalidHandler := handler.NewMessaging(client, aggregation.New(accounts, &messagingMarketplaceStub{}, nil), invalidCapture)
	invalid := messagingRequest(http.MethodPost, "/conversations/c_1/messages", []byte(`{"content":"`+strings.Repeat("中", 1001)+`"}`))
	invalid.SetPathValue("conversationId", "c_1")
	invalidHandler.SendMessage(httptest.NewRecorder(), invalid)
	if invalidCapture.code != errs.CodeValidation || client.sendCalls != 1 {
		t.Fatalf("invalid response code=%s sendCalls=%d", invalidCapture.code, client.sendCalls)
	}
}

func TestMessagingSendRejectsAnEmptyDownstreamMessage(t *testing.T) {
	t.Parallel()
	client := &messagingClientStub{}
	responder := &messagingResponseCapture{}
	h := handler.NewMessaging(client, aggregation.New(&messagingAccountStub{}, &messagingMarketplaceStub{}, nil), responder)
	request := messagingRequest(http.MethodPost, "/conversations/c_1/messages", []byte(`{"content":"你好"}`))
	request.SetPathValue("conversationId", "c_1")

	h.SendMessage(httptest.NewRecorder(), request)
	if responder.code != errs.CodeInternal {
		t.Fatalf("code = %s, want INTERNAL_ERROR", responder.code)
	}
}

func TestMessagingValidatesCursorAndForwardsReadAnchor(t *testing.T) {
	t.Parallel()
	client := &messagingClientStub{messages: []*messagingv1.MessageItem{}}
	responder := &messagingResponseCapture{}
	h := handler.NewMessaging(client, aggregation.New(&messagingAccountStub{}, &messagingMarketplaceStub{}, nil), responder)

	invalid := messagingRequest(http.MethodGet, "/conversations/c_1/messages?before=", nil)
	invalid.SetPathValue("conversationId", "c_1")
	h.ListMessages(httptest.NewRecorder(), invalid)
	if responder.code != errs.CodeValidation || client.lastList != nil {
		t.Fatalf("invalid cursor code=%s downstream=%+v", responder.code, client.lastList)
	}

	readCapture := &messagingResponseCapture{}
	readHandler := handler.NewMessaging(client, aggregation.New(&messagingAccountStub{}, &messagingMarketplaceStub{}, nil), readCapture)
	read := messagingRequest(http.MethodPost, "/conversations/c_1/read", []byte(`{"last_message_id":"m_9"}`))
	read.SetPathValue("conversationId", "c_1")
	readHandler.MarkRead(httptest.NewRecorder(), read)
	if readCapture.status != http.StatusOK || client.lastRead.GetLastMessageId() != "m_9" || client.lastRead.GetConversationId() != "c_1" {
		t.Fatalf("read response=%+v downstream=%+v", readCapture, client.lastRead)
	}
}

func messagingRequest(method, target string, body []byte) *http.Request {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	ctx := middleware.WithActorID(request.Context(), "u_actor")
	ctx = middleware.WithRequestID(ctx, "req_messaging")
	return request.WithContext(ctx)
}
