package grpc_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	messagingv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/messaging/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/grpc"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
	"github.com/KDZZZZZZ/short-term/services/messaging/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	buyer  = "u_buyer"
	seller = "u_seller"
)

type fakeProducts struct {
	mu       sync.Mutex
	products map[string]application.Product
	err      error
	calls    int
}

func (f *fakeProducts) Get(_ context.Context, productID string) (application.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return application.Product{}, f.err
	}
	product, ok := f.products[productID]
	if !ok {
		return application.Product{}, errs.New(errs.CodeResourceNotFound, "商品不存在")
	}
	return product, nil
}

func (f *fakeProducts) setError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeProducts) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type testIDs struct {
	mu             sync.Mutex
	conversation   int
	message        int
	event          int
	duplicateEvent bool
}

func (i *testIDs) NewConversationID() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.conversation++
	return fmt.Sprintf("c_test_%04d", i.conversation)
}

func (i *testIDs) NewMessageID() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.message++
	return fmt.Sprintf("m_test_%04d", i.message)
}

func (i *testIDs) NewEventID() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.duplicateEvent {
		return "evt_test_duplicate"
	}
	i.event++
	return fmt.Sprintf("evt_test_%04d", i.event)
}

type testClock struct {
	mu   sync.Mutex
	next time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next = c.next.Add(time.Microsecond)
	return c.next
}

type harness struct {
	client   messagingv1.MessagingServiceClient
	pool     *pgxpool.Pool
	products *fakeProducts
}

func newHarness(t *testing.T, ids application.IDGenerator) harness {
	t.Helper()
	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	products := &fakeProducts{products: map[string]application.Product{
		"p_keyboard": {ID: "p_keyboard", SellerID: seller},
		"p_own":      {ID: "p_own", SellerID: buyer},
	}}
	if ids == nil {
		ids = &testIDs{}
	}
	clock := &testClock{next: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service, err := application.NewService(postgres.NewRepository(pool), products, ids, clock, logger)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpcx.NewServer(grpcx.ServerOptions{Logger: logger, HandlerTimeout: 10 * time.Second})
	messagingv1.RegisterMessagingServiceServer(server, grpcadapter.NewServer(service))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpcx.Dial(grpcx.ClientOptions{
		Target: listener.Addr().String(), Caller: "messaging-test", DefaultTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return harness{client: messagingv1.NewMessagingServiceClient(conn), pool: pool, products: products}
}

func TestConversationCreationIsConcurrentUniqueIdempotentAndPrivate(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)

	const requests = 12
	ids := make(chan string, requests)
	errorsCh := make(chan error, requests)
	var wait sync.WaitGroup
	for range requests {
		wait.Add(1)
		go func() {
			defer wait.Done()
			resp, err := h.client.GetOrCreateConversation(t.Context(), &messagingv1.GetOrCreateConversationRequest{
				ActorId: buyer, ProductId: "p_keyboard",
			})
			if err != nil {
				errorsCh <- err
				return
			}
			ids <- resp.GetConversation().GetId()
		}()
	}
	wait.Wait()
	close(ids)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("GetOrCreateConversation: %v", err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("conversation id = %q, want the shared id %q", id, first)
		}
	}
	assertCount(t, h.pool, "conversations", 1)

	message := send(t, h.client, seller, first, "当前会话投影")
	key := "conversation-key-0001"
	created, err := h.client.GetOrCreateConversation(t.Context(), &messagingv1.GetOrCreateConversationRequest{
		ActorId: buyer, ProductId: "p_keyboard", IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("keyed GetOrCreateConversation: %v", err)
	}
	if created.GetConversation().GetLastMessage().GetId() != message.GetId() || created.GetConversation().GetUnreadCount() != 1 {
		t.Fatalf("existing conversation projection = %+v, want current last message and unread count", created.GetConversation())
	}
	callsBeforeReplay := h.products.callCount()
	h.products.setError(errors.New("marketplace unavailable"))
	replayed, err := h.client.GetOrCreateConversation(t.Context(), &messagingv1.GetOrCreateConversationRequest{
		ActorId: buyer, ProductId: "p_keyboard", IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("idempotency replay should not call Marketplace: %v", err)
	}
	if !replayed.GetReplayed() || replayed.GetConversation().GetId() != created.GetConversation().GetId() {
		t.Fatalf("replay = %+v, want the first response", replayed)
	}
	if replayed.GetConversation().GetLastMessage().GetId() != message.GetId() || replayed.GetConversation().GetUnreadCount() != 1 {
		t.Fatal("idempotency replay did not preserve the complete first conversation projection")
	}
	if h.products.callCount() != callsBeforeReplay {
		t.Fatal("an idempotency replay consulted current Marketplace state")
	}
	h.products.setError(nil)

	for _, actor := range []string{buyer, seller} {
		resp, err := h.client.GetConversation(t.Context(), &messagingv1.GetConversationRequest{
			ActorId: actor, ConversationId: first,
		})
		if err != nil || resp.GetConversation().GetProductId() != "p_keyboard" {
			t.Fatalf("participant GetConversation(%s) = %+v, %v", actor, resp, err)
		}
	}
	_, err = h.client.GetConversation(t.Context(), &messagingv1.GetConversationRequest{
		ActorId: "u_intruder", ConversationId: first,
	})
	assertCode(t, err, errs.CodeResourceNotFound)

	_, err = h.client.GetOrCreateConversation(t.Context(), &messagingv1.GetOrCreateConversationRequest{
		ActorId: buyer, ProductId: "p_own",
	})
	assertCode(t, err, errs.CodeSelfActionNotAllowed)
	_, err = h.client.GetOrCreateConversation(t.Context(), &messagingv1.GetOrCreateConversationRequest{
		ActorId: buyer, ProductId: "p_missing",
	})
	assertCode(t, err, errs.CodeResourceNotFound)
}

func TestConversationCreationRejectsMismatchedProductProjection(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)

	h.products.mu.Lock()
	h.products.products["p_keyboard"] = application.Product{ID: "p_other", SellerID: seller}
	h.products.mu.Unlock()

	_, err := h.client.GetOrCreateConversation(t.Context(), &messagingv1.GetOrCreateConversationRequest{
		ActorId: buyer, ProductId: "p_keyboard",
	})
	assertCode(t, err, errs.CodeInternal)

	var count int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM conversations`).Scan(&count); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if count != 0 {
		t.Fatalf("stored conversations = %d, want 0", count)
	}
}

func TestGRPCRejectsMismatchedActorMetadata(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)

	_, err := h.client.GetOrCreateConversation(grpcx.WithActor(t.Context(), "u_intruder"), &messagingv1.GetOrCreateConversationRequest{
		ActorId: buyer, ProductId: "p_keyboard",
	})
	assertCode(t, err, errs.CodeForbidden)
}

func TestSendMessageIsIdempotentAndOutboxFailureRollsBackEverything(t *testing.T) {
	t.Parallel()
	ids := &testIDs{duplicateEvent: true}
	h := newHarness(t, ids)
	conversationID := createConversation(t, h.client)

	firstKey := "message-key-00000001"
	first, err := h.client.SendMessage(t.Context(), &messagingv1.SendMessageRequest{
		ActorId: buyer, ConversationId: conversationID, Content: "first", IdempotencyKey: &firstKey,
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	replayed, err := h.client.SendMessage(t.Context(), &messagingv1.SendMessageRequest{
		ActorId: buyer, ConversationId: conversationID, Content: "changed but valid", IdempotencyKey: &firstKey,
	})
	if err != nil {
		t.Fatalf("SendMessage replay: %v", err)
	}
	if !replayed.GetReplayed() || replayed.GetMessage().GetId() != first.GetMessage().GetId() || replayed.GetMessage().GetContent() != "first" {
		t.Fatalf("replayed message = %+v, want exact first snapshot", replayed.GetMessage())
	}
	assertCount(t, h.pool, "messages", 1)
	assertCount(t, h.pool, "outbox_events", 1)

	var firstLastMessageAt time.Time
	if err := h.pool.QueryRow(t.Context(),
		`SELECT last_message_at FROM conversations WHERE id = $1`, conversationID,
	).Scan(&firstLastMessageAt); err != nil {
		t.Fatalf("read last_message_at: %v", err)
	}
	secondKey := "message-key-00000002"
	_, err = h.client.SendMessage(t.Context(), &messagingv1.SendMessageRequest{
		ActorId: seller, ConversationId: conversationID, Content: "must roll back", IdempotencyKey: &secondKey,
	})
	assertCode(t, err, errs.CodeInternal)
	assertCount(t, h.pool, "messages", 1)
	assertCount(t, h.pool, "outbox_events", 1)
	assertCount(t, h.pool, "idempotency_records", 1)

	var lastMessageAt time.Time
	if err := h.pool.QueryRow(t.Context(),
		`SELECT last_message_at FROM conversations WHERE id = $1`, conversationID,
	).Scan(&lastMessageAt); err != nil {
		t.Fatalf("read last_message_at after rollback: %v", err)
	}
	if !lastMessageAt.Equal(firstLastMessageAt) {
		t.Fatalf("last_message_at = %s, want rolled-back value %s", lastMessageAt, firstLastMessageAt)
	}

	_, err = h.client.SendMessage(t.Context(), &messagingv1.SendMessageRequest{
		ActorId: "u_intruder", ConversationId: conversationID, Content: "hidden",
	})
	assertCode(t, err, errs.CodeResourceNotFound)
}

func TestMessageCursorUnreadCountsAndReadRules(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil)
	conversationID := createConversation(t, h.client)

	messages := []*messagingv1.MessageItem{
		send(t, h.client, buyer, conversationID, "buyer one"),
		send(t, h.client, seller, conversationID, "seller one"),
		send(t, h.client, buyer, conversationID, "buyer two"),
		send(t, h.client, seller, conversationID, "seller two"),
	}

	firstPage, err := h.client.ListMessages(t.Context(), &messagingv1.ListMessagesRequest{
		ActorId: buyer, ConversationId: conversationID, Limit: 2,
	})
	if err != nil {
		t.Fatalf("ListMessages first page: %v", err)
	}
	if got := firstPage.GetPage().GetItems(); len(got) != 2 || got[0].GetId() != messages[3].GetId() || got[1].GetId() != messages[2].GetId() {
		t.Fatalf("first page = %+v, want newest two", got)
	}
	if firstPage.GetPage().NextBefore == nil {
		t.Fatal("first page is missing next_before")
	}
	secondPage, err := h.client.ListMessages(t.Context(), &messagingv1.ListMessagesRequest{
		ActorId: buyer, ConversationId: conversationID,
		Before: firstPage.GetPage().NextBefore, Limit: 2,
	})
	if err != nil {
		t.Fatalf("ListMessages second page: %v", err)
	}
	if got := secondPage.GetPage().GetItems(); len(got) != 2 || got[0].GetId() != messages[1].GetId() || got[1].GetId() != messages[0].GetId() {
		t.Fatalf("second page = %+v, want remaining two", got)
	}
	if secondPage.GetPage().NextBefore != nil {
		t.Fatal("last page unexpectedly has next_before")
	}

	for actor, want := range map[string]int64{buyer: 2, seller: 2} {
		unread, err := h.client.GetUnreadCount(t.Context(), &messagingv1.GetUnreadCountRequest{ActorId: actor})
		if err != nil || unread.GetUnreadCount() != want {
			t.Fatalf("GetUnreadCount(%s) = %+v, %v; want %d", actor, unread, err, want)
		}
		list, err := h.client.ListConversations(t.Context(), &messagingv1.ListConversationsRequest{
			ActorId: actor, Page: 1, PageSize: 20,
		})
		if err != nil {
			t.Fatalf("ListConversations(%s): %v", actor, err)
		}
		item := list.GetPage().GetItems()[0]
		if item.GetUnreadCount() != want || item.GetLastMessage().GetId() != messages[3].GetId() {
			t.Fatalf("conversation projection for %s = %+v", actor, item)
		}
	}

	// Reading through the older seller message must leave the newer one unread.
	_, err = h.client.MarkConversationRead(t.Context(), &messagingv1.MarkConversationReadRequest{
		ActorId: buyer, ConversationId: conversationID, LastMessageId: messages[1].GetId(),
	})
	if err != nil {
		t.Fatalf("MarkConversationRead: %v", err)
	}
	assertUnread(t, h.client, buyer, 1)
	assertEventCount(t, h.pool, application.EventConversationRead, 1)

	// The same read is idempotent and emits no duplicate event.
	_, err = h.client.MarkConversationRead(t.Context(), &messagingv1.MarkConversationReadRequest{
		ActorId: buyer, ConversationId: conversationID, LastMessageId: messages[1].GetId(),
	})
	if err != nil {
		t.Fatalf("repeated MarkConversationRead: %v", err)
	}
	assertEventCount(t, h.pool, application.EventConversationRead, 1)

	_, err = h.client.MarkConversationRead(t.Context(), &messagingv1.MarkConversationReadRequest{
		ActorId: buyer, ConversationId: conversationID, LastMessageId: messages[2].GetId(),
	})
	assertCode(t, err, errs.CodeValidation)
	assertUnread(t, h.client, buyer, 1)

	_, err = h.client.MarkConversationRead(t.Context(), &messagingv1.MarkConversationReadRequest{
		ActorId: buyer, ConversationId: conversationID, LastMessageId: messages[3].GetId(),
	})
	if err != nil {
		t.Fatalf("MarkConversationRead latest: %v", err)
	}
	assertUnread(t, h.client, buyer, 0)
	assertUnread(t, h.client, seller, 2)
	assertEventCount(t, h.pool, application.EventConversationRead, 2)

	var buyerRead, sellerRead int
	if err := h.pool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE sender_id = $1 AND read_at IS NOT NULL),
		       count(*) FILTER (WHERE sender_id = $2 AND read_at IS NOT NULL)
		  FROM messages WHERE conversation_id = $3`, buyer, seller, conversationID,
	).Scan(&buyerRead, &sellerRead); err != nil {
		t.Fatalf("inspect read state: %v", err)
	}
	if buyerRead != 0 || sellerRead != 2 {
		t.Fatalf("read messages buyer=%d seller=%d, want 0 and 2", buyerRead, sellerRead)
	}

	_, err = h.client.ListMessages(t.Context(), &messagingv1.ListMessagesRequest{
		ActorId: "u_intruder", ConversationId: conversationID, Limit: 30,
	})
	assertCode(t, err, errs.CodeResourceNotFound)
	_, err = h.client.MarkConversationRead(t.Context(), &messagingv1.MarkConversationReadRequest{
		ActorId: "u_intruder", ConversationId: conversationID, LastMessageId: messages[3].GetId(),
	})
	assertCode(t, err, errs.CodeResourceNotFound)
}

func createConversation(t *testing.T, client messagingv1.MessagingServiceClient) string {
	t.Helper()
	resp, err := client.GetOrCreateConversation(t.Context(), &messagingv1.GetOrCreateConversationRequest{
		ActorId: buyer, ProductId: "p_keyboard",
	})
	if err != nil {
		t.Fatalf("GetOrCreateConversation: %v", err)
	}
	return resp.GetConversation().GetId()
}

func send(t *testing.T, client messagingv1.MessagingServiceClient, actorID, conversationID, content string) *messagingv1.MessageItem {
	t.Helper()
	resp, err := client.SendMessage(t.Context(), &messagingv1.SendMessageRequest{
		ActorId: actorID, ConversationId: conversationID, Content: content,
	})
	if err != nil {
		t.Fatalf("SendMessage(%s): %v", content, err)
	}
	return resp.GetMessage()
}

func assertUnread(t *testing.T, client messagingv1.MessagingServiceClient, actorID string, want int64) {
	t.Helper()
	resp, err := client.GetUnreadCount(t.Context(), &messagingv1.GetUnreadCountRequest{ActorId: actorID})
	if err != nil {
		t.Fatalf("GetUnreadCount(%s): %v", actorID, err)
	}
	if resp.GetUnreadCount() != want {
		t.Fatalf("unread(%s) = %d, want %d", actorID, resp.GetUnreadCount(), want)
	}
}

func assertCount(t *testing.T, pool *pgxpool.Pool, table string, want int64) {
	t.Helper()
	var count int64
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func assertEventCount(t *testing.T, pool *pgxpool.Pool, eventType string, want int64) {
	t.Helper()
	var count int64
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM outbox_events WHERE event_type = $1`, eventType,
	).Scan(&count); err != nil {
		t.Fatalf("count event %s: %v", eventType, err)
	}
	if count != want {
		t.Fatalf("event %s count = %d, want %d", eventType, count, want)
	}
}

func assertCode(t *testing.T, err error, want errs.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	if got := errs.CodeOf(err); got != want {
		t.Fatalf("error code = %s, want %s: %v", got, want, err)
	}
}
