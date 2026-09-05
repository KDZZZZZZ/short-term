package grpc_test

import (
	"context"
	"testing"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/id"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/system"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
)

// fakeConversationVerifier supplies the Messaging-owned facts needed by M3.
// An unknown id is deliberately indistinguishable from an invisible one.
type fakeConversationVerifier struct {
	conversations map[string]application.Conversation
	err           error
}

func (f fakeConversationVerifier) Get(_ context.Context, _ string, conversationID string) (application.Conversation, error) {
	if f.err != nil {
		return application.Conversation{}, f.err
	}
	conversation, ok := f.conversations[conversationID]
	if !ok {
		return application.Conversation{}, errs.New(errs.CodeResourceNotFound, "会话不存在")
	}
	return conversation, nil
}

// createTrade records a purchase intent and returns its identifier.
func (h harness) createTrade(t *testing.T, buyerID, productID string) string {
	t.Helper()

	resp, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerID, ProductId: productID,
	})
	if err != nil {
		t.Fatalf("CreateTrade for %s: %v", buyerID, err)
	}
	return resp.GetTrade().GetId()
}

// accept performs the seller's accept and fails the test if it is refused.
func (h harness) accept(t *testing.T, tradeID string) {
	t.Helper()

	if _, err := h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: tradeID,
	}); err != nil {
		t.Fatalf("AcceptTrade: %v", err)
	}
}

// getTrade reads a trade as one of its parties.
func (h harness) getTrade(t *testing.T, actorID, tradeID string) *marketplacev1.Trade {
	t.Helper()

	resp, err := h.client.GetTrade(t.Context(), &marketplacev1.GetTradeRequest{
		ActorId: actorID, TradeId: tradeID,
	})
	if err != nil {
		t.Fatalf("GetTrade: %v", err)
	}
	return resp.GetTrade()
}

// buyerFor reads a trade's buyer straight from the database, so a test can
// read a trade it did not create without guessing who owns it.
func buyerFor(t *testing.T, h harness, tradeID string) string {
	t.Helper()

	var buyerID string
	if err := h.pool.QueryRow(t.Context(), `SELECT buyer_id FROM trades WHERE id = $1`, tradeID).Scan(&buyerID); err != nil {
		t.Fatalf("read trade buyer: %v", err)
	}
	return buyerID
}

// productStatus reads a product's stored status.
func (h harness) productStatus(t *testing.T, productID string) string {
	t.Helper()

	var status string
	if err := h.pool.QueryRow(t.Context(), `SELECT status FROM products WHERE id = $1`, productID).Scan(&status); err != nil {
		t.Fatalf("read product status: %v", err)
	}
	return status
}

// assertProductStatus checks the stored product status.
func (h harness) assertProductStatus(t *testing.T, productID, want string) {
	t.Helper()

	if got := h.productStatus(t, productID); got != want {
		t.Fatalf("product status = %s, want %s", got, want)
	}
}

// assertTradeCounts checks how many of a product's trades are in each status.
func (h harness) assertTradeCounts(t *testing.T, productID string, want map[string]int) {
	t.Helper()

	rows, err := h.pool.Query(t.Context(),
		`SELECT status, count(*) FROM trades WHERE product_id = $1 GROUP BY status`, productID)
	if err != nil {
		t.Fatalf("count trades: %v", err)
	}
	defer rows.Close()

	got := map[string]int{}
	for rows.Next() {
		var (
			status string
			count  int
		)
		if err := rows.Scan(&status, &count); err != nil {
			t.Fatalf("scan trade counts: %v", err)
		}
		got[status] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read trade counts: %v", err)
	}

	for status, expected := range want {
		if got[status] != expected {
			t.Fatalf("%s trades = %d, want %d (all: %v)", status, got[status], expected, got)
		}
	}
}

// assertTotalTrades checks the lifetime-unique row count independently of
// status so create-or-get and rejected conversation bindings cannot hide an
// extra row in another state.
func (h harness) assertTotalTrades(t *testing.T, productID string, want int) {
	t.Helper()

	var count int
	if err := h.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM trades WHERE product_id = $1`, productID,
	).Scan(&count); err != nil {
		t.Fatalf("count product trades: %v", err)
	}
	if count != want {
		t.Fatalf("product trades = %d, want %d", count, want)
	}
}

// assertIdempotencyRecords checks how many committed command results exist.
func (h harness) assertIdempotencyRecords(t *testing.T, want int) {
	t.Helper()

	var count int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM idempotency_records`).Scan(&count); err != nil {
		t.Fatalf("count idempotency records: %v", err)
	}
	if count != want {
		t.Fatalf("idempotency records = %d, want %d", count, want)
	}
}

// assertOutboxCount checks how many events of a type were written.
func (h harness) assertOutboxCount(t *testing.T, eventType string, want int) {
	t.Helper()

	var count int
	if err := h.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM outbox_events WHERE event_type = $1`, eventType).Scan(&count); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if count != want {
		t.Fatalf("%s events = %d, want %d", eventType, count, want)
	}
}

// newRealIDs builds the production identifier generator.
func newRealIDs() application.IDGenerator { return system.NewIDs() }

// collidingEventIDs returns one fixed event id for every outbox record, which
// makes the second append in a transaction violate the outbox primary key.
// Everything else is generated normally, so only the outbox write fails.
type collidingEventIDs struct {
	inner application.IDGenerator
}

func (c collidingEventIDs) NewProductID() string { return c.inner.NewProductID() }
func (c collidingEventIDs) NewImageID() string   { return c.inner.NewImageID() }
func (c collidingEventIDs) NewTradeID() string   { return c.inner.NewTradeID() }
func (c collidingEventIDs) NewReviewID() string  { return c.inner.NewReviewID() }
func (c collidingEventIDs) NewEventID() string   { return "evt_" + string(id.PrefixEvent) + "_fixed" }
