package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/auth"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	gatewayhttp "github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/handler"
)

// This test exercises the shared router wiring that the isolated M4 and M5
// handler tests cannot cover.
func TestFavoriteAndMessagingRoutesAreWired(t *testing.T) {
	t.Parallel()

	accounts := &stubAccounts{}
	marketplace := &stubMarketplace{
		createCommentResp: &marketplacev1.CreateProductCommentResponse{Comment: &marketplacev1.ProductComment{
			Id: "cm_route", ProductId: testProductID, UserId: testActor, Content: "不错",
			CreatedAt: timestamppb.New(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)),
		}},
	}
	favorites := &routeFavoriteStub{}
	messaging := newRouteMessagingStub()
	aggregator := aggregation.New(accounts, marketplace, nil)
	server, token := newMilestoneServer(t, accounts, marketplace, favorites, messaging, aggregator)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "list favorites", method: http.MethodGet, path: basePath + "/favorites", wantStatus: http.StatusOK},
		{name: "add favorite", method: http.MethodPut, path: basePath + "/favorites/" + testProductID, wantStatus: http.StatusOK},
		{name: "remove favorite", method: http.MethodDelete, path: basePath + "/favorites/" + testProductID, wantStatus: http.StatusOK},
		{name: "list comments", method: http.MethodGet, path: basePath + "/products/" + testProductID + "/comments", wantStatus: http.StatusOK},
		{name: "create comment", method: http.MethodPost, path: basePath + "/products/" + testProductID + "/comments", body: `{"content":"不错"}`, wantStatus: http.StatusCreated},
		{name: "get or create conversation", method: http.MethodPost, path: basePath + "/products/" + testProductID + "/conversations", wantStatus: http.StatusOK},
		{name: "list conversations", method: http.MethodGet, path: basePath + "/conversations", wantStatus: http.StatusOK},
		{name: "unread count", method: http.MethodGet, path: basePath + "/conversations/unread-count", wantStatus: http.StatusOK},
		{name: "list messages", method: http.MethodGet, path: basePath + "/conversations/c_route/messages", wantStatus: http.StatusOK},
		{name: "send message", method: http.MethodPost, path: basePath + "/conversations/c_route/messages", body: `{"content":"你好"}`, wantStatus: http.StatusCreated},
		{name: "mark read", method: http.MethodPost, path: basePath + "/conversations/c_route/read", body: `{"last_message_id":"m_route"}`, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := request(t, server, tt.method, tt.path, token, tt.body)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, tt.wantStatus, body)
			}
		})
	}
}

func newMilestoneServer(
	t *testing.T,
	accounts *stubAccounts,
	marketplace *stubMarketplace,
	favorites favoritev1.FavoriteServiceClient,
	messaging messagingv1.MessagingServiceClient,
	aggregator *aggregation.Aggregator,
) (*httptest.Server, string) {
	t.Helper()

	verifier, err := auth.NewVerifier(tokenConfig(), nil)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	issuer, err := auth.NewIssuer(tokenConfig(), nil, func() string { return "jti-milestones" })
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	token, _, err := issuer.Issue(testActor)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := gatewayhttp.NewResponder(logger)
	router := gatewayhttp.NewRouter(gatewayhttp.RouterOptions{
		BasePath:     basePath,
		Verifier:     verifier,
		MaxBodyBytes: 1 << 20,
		Logger:       logger,
		Ready:        func(context.Context) error { return nil },
		Auth:         handler.NewAuth(accounts, responder),
		Users:        handler.NewUsers(accounts, responder),
		Products:     handler.NewProducts(marketplace, accounts, aggregator, responder),
		Trades:       handler.NewTrades(marketplace, aggregator, responder),
		Comments:     handler.NewComments(marketplace, aggregator, responder),
		Favorites:    handler.NewFavorites(favorites, aggregator, responder),
		Messaging:    handler.NewMessaging(messaging, aggregator, responder),
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, token
}

type routeFavoriteStub struct {
	favoritev1.FavoriteServiceClient
}

func (s *routeFavoriteStub) AddFavorite(context.Context, *favoritev1.AddFavoriteRequest, ...grpc.CallOption) (*favoritev1.AddFavoriteResponse, error) {
	return &favoritev1.AddFavoriteResponse{}, nil
}

func (s *routeFavoriteStub) RemoveFavorite(context.Context, *favoritev1.RemoveFavoriteRequest, ...grpc.CallOption) (*favoritev1.RemoveFavoriteResponse, error) {
	return &favoritev1.RemoveFavoriteResponse{}, nil
}

func (s *routeFavoriteStub) ListFavorites(_ context.Context, req *favoritev1.ListFavoritesRequest, _ ...grpc.CallOption) (*favoritev1.ListFavoritesResponse, error) {
	return &favoritev1.ListFavoritesResponse{Page: &favoritev1.FavoritePage{
		Items: []*favoritev1.FavoriteItem{}, Page: req.GetPage(), PageSize: req.GetPageSize(),
	}}, nil
}

type routeMessagingStub struct {
	messagingv1.MessagingServiceClient
	conversation *messagingv1.ConversationItem
	message      *messagingv1.MessageItem
}

func newRouteMessagingStub() *routeMessagingStub {
	now := timestamppb.New(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
	return &routeMessagingStub{
		conversation: &messagingv1.ConversationItem{
			Id: "c_route", ProductId: testProductID, BuyerId: testActor,
			SellerId: "u_seller", CreatedAt: now,
		},
		message: &messagingv1.MessageItem{
			Id: "m_route", ConversationId: "c_route", SenderId: "u_seller",
			Content: "你好", CreatedAt: now,
		},
	}
}

func (s *routeMessagingStub) GetOrCreateConversation(context.Context, *messagingv1.GetOrCreateConversationRequest, ...grpc.CallOption) (*messagingv1.GetOrCreateConversationResponse, error) {
	return &messagingv1.GetOrCreateConversationResponse{Conversation: s.conversation}, nil
}

func (s *routeMessagingStub) ListConversations(_ context.Context, req *messagingv1.ListConversationsRequest, _ ...grpc.CallOption) (*messagingv1.ListConversationsResponse, error) {
	return &messagingv1.ListConversationsResponse{Page: &messagingv1.ConversationPage{
		Items: []*messagingv1.ConversationItem{s.conversation}, Page: req.GetPage(),
		PageSize: req.GetPageSize(), Total: 1,
	}}, nil
}

func (s *routeMessagingStub) GetUnreadCount(context.Context, *messagingv1.GetUnreadCountRequest, ...grpc.CallOption) (*messagingv1.GetUnreadCountResponse, error) {
	return &messagingv1.GetUnreadCountResponse{UnreadCount: 1}, nil
}

func (s *routeMessagingStub) ListMessages(context.Context, *messagingv1.ListMessagesRequest, ...grpc.CallOption) (*messagingv1.ListMessagesResponse, error) {
	return &messagingv1.ListMessagesResponse{Page: &messagingv1.MessagePage{Items: []*messagingv1.MessageItem{s.message}}}, nil
}

func (s *routeMessagingStub) SendMessage(context.Context, *messagingv1.SendMessageRequest, ...grpc.CallOption) (*messagingv1.SendMessageResponse, error) {
	return &messagingv1.SendMessageResponse{Message: s.message}, nil
}

func (s *routeMessagingStub) MarkConversationRead(context.Context, *messagingv1.MarkConversationReadRequest, ...grpc.CallOption) (*messagingv1.MarkConversationReadResponse, error) {
	return &messagingv1.MarkConversationReadResponse{}, nil
}
