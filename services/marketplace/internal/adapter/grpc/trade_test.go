package grpc_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
)

const (
	buyerA = "u_buyer_a"
	buyerB = "u_buyer_b"
	buyerC = "u_buyer_c"
)

// key builds a contract-valid Idempotency-Key of at least 16 characters.
func key(name string) *string {
	value := "idem-" + name + strings.Repeat("0", 16)
	return &value
}

func TestCreateTradeLeavesTheProductOnSale(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)

	resp, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(),
	})
	if err != nil {
		t.Fatalf("CreateTrade: %v", err)
	}
	if !resp.GetCreated() || resp.GetReplayed() {
		t.Fatalf("created/replayed = %v/%v, want true/false", resp.GetCreated(), resp.GetReplayed())
	}

	trade := resp.GetTrade()
	if trade.GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_PENDING {
		t.Fatalf("status = %s, want PENDING", trade.GetStatus())
	}
	if trade.GetPriceSnapshotMinor() != product.GetPriceMinor() {
		t.Fatalf("price snapshot = %d, want %d", trade.GetPriceSnapshotMinor(), product.GetPriceMinor())
	}
	if trade.GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE {
		t.Fatalf("creating a trade changed the product to %s", trade.GetProduct().GetStatus())
	}
	if trade.GetBuyerConfirmed() || trade.GetSellerConfirmed() {
		t.Fatal("a new trade is already confirmed")
	}
}

func TestCreateTradeIsRefusedForOwnAndUntradableProducts(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)

	_, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: seller, ProductId: product.GetId(),
	})
	assertCode(t, err, errs.CodeSelfActionNotAllowed)

	offShelf := h.create(t, seller, "下架商品", nil)
	if _, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: offShelf.GetId(),
	}); err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}
	_, err = h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: offShelf.GetId(),
	})
	assertCode(t, err, errs.CodeProductNotAvailable)

	_, err = h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: "p_missing",
	})
	assertCode(t, err, errs.CodeResourceNotFound)
}

func TestAcceptReservesTheProductAndCancelsCompetingTrades(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)

	winner := h.createTrade(t, buyerA, product.GetId())
	loserOne := h.createTrade(t, buyerB, product.GetId())
	loserTwo := h.createTrade(t, buyerC, product.GetId())

	accepted, err := h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: winner,
	})
	if err != nil {
		t.Fatalf("AcceptTrade: %v", err)
	}
	if accepted.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED {
		t.Fatalf("status = %s, want ACCEPTED", accepted.GetTrade().GetStatus())
	}
	if accepted.GetTrade().GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED {
		t.Fatalf("product = %s, want RESERVED", accepted.GetTrade().GetProduct().GetStatus())
	}

	// The competing buyers must not be left holding a pending trade on a
	// reserved product.
	for _, tradeID := range []string{loserOne, loserTwo} {
		trade := h.getTrade(t, buyerFor(t, h, tradeID), tradeID)
		if trade.GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED {
			t.Fatalf("competing trade %s = %s, want CANCELLED", tradeID, trade.GetStatus())
		}
		if trade.GetCancelReason() == "" {
			t.Fatal("an auto-cancelled trade carries no reason")
		}
	}

	h.assertStatusEverywhere(t, product.GetId(), marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED)
}

func TestConcurrentAcceptsLeaveExactlyOneAcceptedTrade(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)

	const buyers = 6
	trades := make([]string, buyers)
	for i := range trades {
		trades[i] = h.createTrade(t, fmt.Sprintf("u_buyer_%02d", i), product.GetId())
	}

	// Every accept races for the same product. Only the product row lock and
	// the partial unique index decide the winner.
	results := make([]error, buyers)
	var wg sync.WaitGroup
	for i, tradeID := range trades {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
				ActorId: seller, TradeId: tradeID,
			})
		}()
	}
	wg.Wait()

	var accepted int
	for i, err := range results {
		switch {
		case err == nil:
			accepted++
		case errs.CodeOf(err) == errs.CodeTradeStateConflict,
			errs.CodeOf(err) == errs.CodeProductNotAvailable:
		default:
			t.Fatalf("trade %d failed unexpectedly: %v", i, err)
		}
	}
	if accepted != 1 {
		t.Fatalf("%d accepts succeeded, want exactly 1", accepted)
	}

	h.assertTradeCounts(t, product.GetId(), map[string]int{
		"ACCEPTED":  1,
		"CANCELLED": buyers - 1,
	})
	h.assertProductStatus(t, product.GetId(), "RESERVED")
}

func TestAcceptIsSellerOnly(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())

	for _, tt := range []struct {
		actor string
		code  errs.Code
	}{
		{actor: buyerA, code: errs.CodeForbidden},
		{actor: "u_stranger", code: errs.CodeResourceNotFound},
	} {
		_, err := h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
			ActorId: tt.actor, TradeId: tradeID,
		})
		assertCode(t, err, tt.code)
	}
	h.assertProductStatus(t, product.GetId(), "ON_SALE")
}

func TestRejectCancelsWithoutTouchingTheProduct(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())

	resp, err := h.client.RejectTrade(t.Context(), &marketplacev1.RejectTradeRequest{
		ActorId: seller, TradeId: tradeID, Reason: "已经卖掉了",
	})
	if err != nil {
		t.Fatalf("RejectTrade: %v", err)
	}
	if resp.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED {
		t.Fatalf("status = %s, want CANCELLED", resp.GetTrade().GetStatus())
	}
	h.assertProductStatus(t, product.GetId(), "ON_SALE")

	_, err = h.client.RejectTrade(t.Context(), &marketplacev1.RejectTradeRequest{
		ActorId: seller, TradeId: tradeID, Reason: "再拒一次",
	})
	assertCode(t, err, errs.CodeTradeStateConflict)
}

func TestCancellingAnAcceptedTradeReleasesTheProduct(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	h.accept(t, tradeID)
	h.assertProductStatus(t, product.GetId(), "RESERVED")

	resp, err := h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
		ActorId: buyerA, TradeId: tradeID, Reason: "临时有事",
	})
	if err != nil {
		t.Fatalf("CancelTrade: %v", err)
	}
	if resp.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED {
		t.Fatalf("status = %s, want CANCELLED", resp.GetTrade().GetStatus())
	}
	if resp.GetTrade().GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE {
		t.Fatalf("product = %s, want ON_SALE", resp.GetTrade().GetProduct().GetStatus())
	}
	h.assertStatusEverywhere(t, product.GetId(), marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE)

	// The product is tradable again, which is the point of releasing it.
	h.createTrade(t, buyerB, product.GetId())
}

func TestPendingTradeCancellationIsBuyerOnly(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())

	// The seller's way out of a pending trade is reject, not cancel.
	_, err := h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
		ActorId: seller, TradeId: tradeID, Reason: "不卖了",
	})
	assertCode(t, err, errs.CodeForbidden)

	_, err = h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
		ActorId: "u_stranger", TradeId: tradeID, Reason: "路过",
	})
	assertCode(t, err, errs.CodeResourceNotFound)

	if _, err := h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
		ActorId: buyerA, TradeId: tradeID, Reason: "不买了",
	}); err != nil {
		t.Fatalf("CancelTrade by the buyer: %v", err)
	}
}

func TestBothConfirmationsCompleteTheTradeAndSellTheProduct(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	h.accept(t, tradeID)

	first, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: buyerA, TradeId: tradeID,
	})
	if err != nil {
		t.Fatalf("first ConfirmTrade: %v", err)
	}
	if first.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED {
		t.Fatalf("status after one confirmation = %s, want ACCEPTED", first.GetTrade().GetStatus())
	}
	if !first.GetTrade().GetBuyerConfirmed() || first.GetTrade().GetSellerConfirmed() {
		t.Fatal("the recorded confirmations are wrong after one confirmation")
	}
	h.assertProductStatus(t, product.GetId(), "RESERVED")

	second, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: seller, TradeId: tradeID,
	})
	if err != nil {
		t.Fatalf("second ConfirmTrade: %v", err)
	}
	if second.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_COMPLETED {
		t.Fatalf("status = %s, want COMPLETED", second.GetTrade().GetStatus())
	}
	if second.GetTrade().GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD {
		t.Fatalf("product = %s, want SOLD", second.GetTrade().GetProduct().GetStatus())
	}
	h.assertStatusEverywhere(t, product.GetId(), marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD)
}

func TestRepeatedConfirmationByOnePartyDoesNotComplete(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	h.accept(t, tradeID)

	for range 3 {
		resp, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
			ActorId: buyerA, TradeId: tradeID,
		})
		if err != nil {
			t.Fatalf("ConfirmTrade: %v", err)
		}
		if resp.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED {
			t.Fatalf("one party confirming repeatedly reached %s", resp.GetTrade().GetStatus())
		}
	}
	h.assertProductStatus(t, product.GetId(), "RESERVED")
}

func TestCancelAndSecondConfirmationRaceHasExactlyOneWinner(t *testing.T) {
	t.Parallel()

	// The two commands both start from ACCEPTED + RESERVED and lead to
	// opposite outcomes. Only one may commit, and neither may leave the
	// product and the trade disagreeing.
	for attempt := range 6 {
		t.Run(fmt.Sprintf("attempt-%d", attempt), func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			product := h.create(t, seller, "机械键盘", nil)
			tradeID := h.createTrade(t, buyerA, product.GetId())
			h.accept(t, tradeID)

			// The buyer has already confirmed, so the seller's confirmation
			// would complete the trade.
			if _, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
				ActorId: buyerA, TradeId: tradeID,
			}); err != nil {
				t.Fatalf("buyer ConfirmTrade: %v", err)
			}

			var (
				wg                    sync.WaitGroup
				cancelErr, confirmErr error
			)
			start := make(chan struct{})
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, cancelErr = h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
					ActorId: buyerA, TradeId: tradeID, Reason: "临时有事",
				})
			}()
			go func() {
				defer wg.Done()
				<-start
				_, confirmErr = h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
					ActorId: seller, TradeId: tradeID,
				})
			}()
			close(start)
			wg.Wait()

			succeeded := 0
			for _, err := range []error{cancelErr, confirmErr} {
				switch {
				case err == nil:
					succeeded++
				case errs.CodeOf(err) == errs.CodeTradeStateConflict,
					errs.CodeOf(err) == errs.CodeProductNotAvailable:
				default:
					t.Fatalf("unexpected failure: %v", err)
				}
			}
			if succeeded != 1 {
				t.Fatalf("%d of the two racing commands succeeded, want exactly 1", succeeded)
			}

			// Whichever won, the trade and the product must agree. The
			// forbidden combinations are CANCELLED+RESERVED, COMPLETED+RESERVED
			// and ACCEPTED+SOLD.
			trade := h.getTrade(t, buyerA, tradeID)
			productStatus := h.productStatus(t, product.GetId())
			switch trade.GetStatus() {
			case marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED:
				if productStatus != "ON_SALE" {
					t.Fatalf("cancelled trade left the product %s", productStatus)
				}
			case marketplacev1.TradeStatus_TRADE_STATUS_COMPLETED:
				if productStatus != "SOLD" {
					t.Fatalf("completed trade left the product %s", productStatus)
				}
			default:
				t.Fatalf("trade ended as %s with product %s", trade.GetStatus(), productStatus)
			}
		})
	}
}

func TestIdempotentRetryReplaysTheFirstResultInsteadOfConflicting(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", []*marketplacev1.ImageUpload{
		{Data: pngBytes(t), ContentType: "image/png"},
	})
	tradeID := h.createTrade(t, buyerA, product.GetId())

	req := &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: tradeID, IdempotencyKey: key("accept"),
	}

	first, err := h.client.AcceptTrade(t.Context(), req)
	if err != nil {
		t.Fatalf("first AcceptTrade: %v", err)
	}
	if first.GetReplayed() {
		t.Fatal("the first attempt was reported as a replay")
	}
	if _, err := h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
		ActorId: buyerA, TradeId: tradeID, Reason: "响应丢失后的状态变化",
	}); err != nil {
		t.Fatalf("CancelTrade after first accept: %v", err)
	}
	changedTitle := "后来修改的标题"
	if _, err := h.client.UpdateProduct(t.Context(), &marketplacev1.UpdateProductRequest{
		ActorId: seller, ProductId: product.GetId(), Title: &changedTitle,
	}); err != nil {
		t.Fatalf("UpdateProduct after cancellation: %v", err)
	}
	if _, err := h.client.DeleteProductImage(t.Context(), &marketplacev1.DeleteProductImageRequest{
		ActorId: seller, ProductId: product.GetId(), ImageId: product.GetImages()[0].GetId(),
	}); err != nil {
		t.Fatalf("DeleteProductImage after cancellation: %v", err)
	}
	h.assertProductStatus(t, product.GetId(), "ON_SALE")

	// The trade is CANCELLED and the product is ON_SALE now. With the same key,
	// both the trade and product projection must still be the first ACCEPTED +
	// RESERVED response rather than a mixture of stored and current state.
	second, err := h.client.AcceptTrade(t.Context(), req)
	if err != nil {
		t.Fatalf("retry with the same key = %v, want the first result", err)
	}
	if !second.GetReplayed() {
		t.Fatal("the retry was not reported as a replay")
	}
	if second.GetTrade().GetId() != first.GetTrade().GetId() ||
		second.GetTrade().GetStatus() != first.GetTrade().GetStatus() {
		t.Fatalf("replay returned %+v, want the first result %+v", second.GetTrade(), first.GetTrade())
	}
	if second.GetTrade().GetProduct().GetStatus() != marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED {
		t.Fatalf("replay product = %s, want first response RESERVED", second.GetTrade().GetProduct().GetStatus())
	}
	if second.GetTrade().GetProduct().GetTitle() != first.GetTrade().GetProduct().GetTitle() ||
		second.GetTrade().GetProduct().GetCoverUrl() != first.GetTrade().GetProduct().GetCoverUrl() {
		t.Fatalf("replay product projection = %+v, want first response %+v",
			second.GetTrade().GetProduct(), first.GetTrade().GetProduct())
	}

	// Without the key the same call sees the current CANCELLED trade and conflicts.
	_, err = h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: tradeID,
	})
	assertCode(t, err, errs.CodeTradeStateConflict)
}

func TestIdempotencyKeysAreScopedByActorAndOperation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	shared := key("shared")

	if _, err := h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: tradeID, IdempotencyKey: shared,
	}); err != nil {
		t.Fatalf("AcceptTrade: %v", err)
	}

	// The same key on a different operation must not replay the accept.
	confirm, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: seller, TradeId: tradeID, IdempotencyKey: shared,
	})
	if err != nil {
		t.Fatalf("ConfirmTrade with a reused key: %v", err)
	}
	if confirm.GetReplayed() {
		t.Fatal("a key reused on another operation replayed the first command")
	}
	if !confirm.GetTrade().GetSellerConfirmed() {
		t.Fatal("the confirmation was not recorded")
	}

	// The same key held by another actor is a different command entirely.
	other, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: buyerA, TradeId: tradeID, IdempotencyKey: shared,
	})
	if err != nil {
		t.Fatalf("ConfirmTrade by the buyer with the same key: %v", err)
	}
	if other.GetReplayed() {
		t.Fatal("a key reused by another actor replayed their command")
	}
	if other.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_COMPLETED {
		t.Fatalf("status = %s, want COMPLETED", other.GetTrade().GetStatus())
	}
}

func TestConcurrentRetriesOfOneKeyProduceOneEffect(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	shared := key("burst")

	const attempts = 8
	responses := make([]*marketplacev1.AcceptTradeResponse, attempts)
	errsSeen := make([]error, attempts)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			responses[i], errsSeen[i] = h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
				ActorId: seller, TradeId: tradeID, IdempotencyKey: shared,
			})
		}()
	}
	close(start)
	wg.Wait()

	fresh := 0
	for i, err := range errsSeen {
		if err != nil {
			t.Fatalf("attempt %d failed: %v", i, err)
		}
		if !responses[i].GetReplayed() {
			fresh++
		}
		if responses[i].GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED {
			t.Fatalf("attempt %d returned status %s", i, responses[i].GetTrade().GetStatus())
		}
	}
	if fresh != 1 {
		t.Fatalf("%d attempts performed the command, want exactly 1", fresh)
	}
	h.assertProductStatus(t, product.GetId(), "RESERVED")
	h.assertIdempotencyRecords(t, 1)
}

func TestShortIdempotencyKeysAreRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	short := "too-short"

	_, err := h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: tradeID, IdempotencyKey: &short,
	})
	assertCode(t, err, errs.CodeValidation)
	h.assertProductStatus(t, product.GetId(), "ON_SALE")

	// JSON Schema minLength counts Unicode code points, not UTF-8 bytes.
	unicodeKey := strings.Repeat("幂", 16)
	if _, err := h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: tradeID, IdempotencyKey: &unicodeKey,
	}); err != nil {
		t.Fatalf("16-character Unicode Idempotency-Key: %v", err)
	}
}

func TestAFailedOutboxWriteRollsBackTheWholeCommand(t *testing.T) {
	t.Parallel()

	// Every outbox record this harness writes claims the same event id, so the
	// second append in the accept transaction violates the outbox primary key.
	// The domain writes that already happened in that transaction must not
	// survive.
	h := newHarnessWith(t, collidingEventIDs{inner: newRealIDs()})
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())

	_, err := h.client.AcceptTrade(t.Context(), &marketplacev1.AcceptTradeRequest{
		ActorId: seller, TradeId: tradeID, IdempotencyKey: key("rollback"),
	})
	if err == nil {
		t.Fatal("AcceptTrade succeeded despite a failing outbox write")
	}
	assertCode(t, err, errs.CodeInternal)

	// Trade, product, outbox and idempotency ledger must all be untouched.
	trade := h.getTrade(t, buyerA, tradeID)
	if trade.GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_PENDING {
		t.Fatalf("trade = %s, want PENDING after the rollback", trade.GetStatus())
	}
	h.assertProductStatus(t, product.GetId(), "ON_SALE")
	h.assertIdempotencyRecords(t, 0)
	h.assertOutboxCount(t, "marketplace.trade.accepted", 0)
}

func TestCommittedCommandsWriteTheirOutboxRecords(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	h.accept(t, tradeID)

	// The accept transaction produced the trade fact and the product status
	// fact together.
	h.assertOutboxCount(t, application.EventTradeCreated, 1)
	h.assertOutboxCount(t, application.EventTradeAccepted, 1)
	h.assertOutboxCount(t, application.EventProductStatusChanged, 1)
}

func TestGetTradeIsLimitedToTheTwoParties(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())

	for _, actor := range []string{buyerA, seller} {
		if _, err := h.client.GetTrade(t.Context(), &marketplacev1.GetTradeRequest{
			ActorId: actor, TradeId: tradeID,
		}); err != nil {
			t.Fatalf("GetTrade as %s: %v", actor, err)
		}
	}

	// A stranger is told the trade does not exist rather than that it exists
	// but is off limits.
	_, err := h.client.GetTrade(t.Context(), &marketplacev1.GetTradeRequest{
		ActorId: "u_stranger", TradeId: tradeID,
	})
	assertCode(t, err, errs.CodeResourceNotFound)
}

func TestListTradesSeparatesBuyingFromSelling(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())

	asBuyer, err := h.client.ListTrades(t.Context(), &marketplacev1.ListTradesRequest{
		ActorId: buyerA, AsBuyer: true, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListTrades: %v", err)
	}
	if len(asBuyer.GetPage().GetItems()) != 1 || asBuyer.GetPage().GetItems()[0].GetId() != tradeID {
		t.Fatalf("buyer list = %+v", asBuyer.GetPage().GetItems())
	}

	asSeller, err := h.client.ListTrades(t.Context(), &marketplacev1.ListTradesRequest{
		ActorId: seller, AsBuyer: false, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListTrades: %v", err)
	}
	if len(asSeller.GetPage().GetItems()) != 1 {
		t.Fatalf("seller list = %+v", asSeller.GetPage().GetItems())
	}

	// The same user viewing the other side of their own trade sees nothing.
	empty, err := h.client.ListTrades(t.Context(), &marketplacev1.ListTradesRequest{
		ActorId: buyerA, AsBuyer: false, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListTrades: %v", err)
	}
	if len(empty.GetPage().GetItems()) != 0 {
		t.Fatalf("a buyer appeared in their own seller list: %+v", empty.GetPage().GetItems())
	}
}

func TestListTradesCarriesTheCurrentProductStatus(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	h.accept(t, tradeID)

	page, err := h.client.ListTrades(t.Context(), &marketplacev1.ListTradesRequest{
		ActorId: buyerA, AsBuyer: true, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListTrades: %v", err)
	}
	if got := page.GetPage().GetItems()[0].GetProduct().GetStatus(); got != marketplacev1.ProductStatus_PRODUCT_STATUS_RESERVED {
		t.Fatalf("trade list product status = %s, want RESERVED", got)
	}
}

func TestCreateTradeReturnsTheLifetimeUniqueIntentWithoutReopeningIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	first, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(), IdempotencyKey: key("first-create"),
	})
	if err != nil {
		t.Fatalf("first CreateTrade: %v", err)
	}
	if !first.GetCreated() {
		t.Fatal("the first intent was not marked created")
	}

	second, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(), IdempotencyKey: key("second-create"),
	})
	if err != nil {
		t.Fatalf("second CreateTrade: %v", err)
	}
	if second.GetCreated() || second.GetTrade().GetId() != first.GetTrade().GetId() {
		t.Fatalf("second create returned created=%v id=%q, want existing %q",
			second.GetCreated(), second.GetTrade().GetId(), first.GetTrade().GetId())
	}
	replayedExisting, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(), IdempotencyKey: key("second-create"),
	})
	if err != nil {
		t.Fatalf("replay of existing create-or-get: %v", err)
	}
	if replayedExisting.GetCreated() || !replayedExisting.GetReplayed() {
		t.Fatalf("existing replay created/replayed = %v/%v, want false/true",
			replayedExisting.GetCreated(), replayedExisting.GetReplayed())
	}

	// A terminal intent remains the one business intent for this buyer/product.
	if _, err := h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
		ActorId: buyerA, TradeId: first.GetTrade().GetId(), Reason: "换一个",
	}); err != nil {
		t.Fatalf("CancelTrade: %v", err)
	}

	terminal, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(), IdempotencyKey: key("after-cancel"),
	})
	if err != nil {
		t.Fatalf("CreateTrade after cancellation: %v", err)
	}
	if terminal.GetCreated() || terminal.GetTrade().GetId() != first.GetTrade().GetId() ||
		terminal.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED {
		t.Fatalf("terminal create-or-get = %+v", terminal)
	}

	h.assertTradeCounts(t, product.GetId(), map[string]int{"CANCELLED": 1})
	h.assertOutboxCount(t, application.EventTradeCreated, 1)
}

func TestConcurrentCreateTradeCallsCreateOneLifetimeIntent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)

	const attempts = 8
	responses := make([]*marketplacev1.CreateTradeResponse, attempts)
	errsSeen := make([]error, attempts)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			responses[i], errsSeen[i] = h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
				ActorId: buyerA, ProductId: product.GetId(),
				IdempotencyKey: key(fmt.Sprintf("concurrent-create-%d", i)),
			})
		}()
	}
	close(start)
	wg.Wait()

	created := 0
	tradeID := ""
	for i, err := range errsSeen {
		if err != nil {
			t.Fatalf("create %d failed: %v", i, err)
		}
		if responses[i].GetCreated() {
			created++
		}
		if responses[i].GetReplayed() {
			t.Fatalf("create %d with a distinct key was reported as a replay", i)
		}
		if tradeID == "" {
			tradeID = responses[i].GetTrade().GetId()
		}
		if responses[i].GetTrade().GetId() != tradeID {
			t.Fatalf("create %d returned trade %q, want %q", i, responses[i].GetTrade().GetId(), tradeID)
		}
	}
	if created != 1 {
		t.Fatalf("created responses = %d, want exactly 1", created)
	}

	h.assertTotalTrades(t, product.GetId(), 1)
	h.assertOutboxCount(t, application.EventTradeCreated, 1)
	h.assertIdempotencyRecords(t, attempts)
}

func TestCreateTradeValidatesAndFreezesConversationBinding(t *testing.T) {
	t.Parallel()

	const (
		conversationID = "c_matching"
		otherID        = "c_other"
	)
	verifier := &fakeConversationVerifier{conversations: map[string]application.Conversation{}}
	h := newHarnessWithConversationVerifier(t, nil, verifier)
	product := h.create(t, seller, "机械键盘", nil)
	verifier.conversations[conversationID] = application.Conversation{
		ID: conversationID, ProductID: product.GetId(), BuyerID: buyerA, SellerID: seller,
	}
	verifier.conversations[otherID] = application.Conversation{
		ID: otherID, ProductID: product.GetId(), BuyerID: buyerA, SellerID: seller,
	}

	first, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(), ConversationId: stringPtr(conversationID),
		ConversationIdPresent: true,
	})
	if err != nil {
		t.Fatalf("CreateTrade with a matching conversation: %v", err)
	}
	if !first.GetCreated() || first.GetTrade().GetConversationId() != conversationID {
		t.Fatalf("created trade binding = %+v", first)
	}

	// Omission reads the existing intent without imposing a binding condition.
	omitted, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(),
	})
	if err != nil {
		t.Fatalf("omitted binding on existing intent: %v", err)
	}
	if omitted.GetCreated() || omitted.GetTrade().GetId() != first.GetTrade().GetId() {
		t.Fatalf("omitted create-or-get = %+v", omitted)
	}

	// The exact explicit value is accepted, but neither explicit null nor a
	// different valid conversation can mutate the frozen binding.
	if _, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(), ConversationId: stringPtr(conversationID),
		ConversationIdPresent: true,
	}); err != nil {
		t.Fatalf("same explicit binding: %v", err)
	}
	for _, req := range []*marketplacev1.CreateTradeRequest{
		{ActorId: buyerA, ProductId: product.GetId(), ConversationIdPresent: true},
		{ActorId: buyerA, ProductId: product.GetId(), ConversationId: stringPtr(otherID), ConversationIdPresent: true},
	} {
		_, err := h.client.CreateTrade(t.Context(), req)
		assertCode(t, err, errs.CodeConversationMismatch)
	}

	h.assertTotalTrades(t, product.GetId(), 1)
	h.assertOutboxCount(t, application.EventTradeCreated, 1)
}

func TestCreateTradeRejectsMissingOrMismatchedConversationsWithoutWrites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(productID string) application.Conversation
		known bool
		want  errs.Code
	}{
		{name: "missing", known: false, want: errs.CodeResourceNotFound},
		{name: "wrong product", known: true, want: errs.CodeConversationMismatch, build: func(string) application.Conversation {
			return application.Conversation{ID: "c_test", ProductID: "p_other", BuyerID: buyerA, SellerID: seller}
		}},
		{name: "wrong buyer", known: true, want: errs.CodeConversationMismatch, build: func(productID string) application.Conversation {
			return application.Conversation{ID: "c_test", ProductID: productID, BuyerID: buyerB, SellerID: seller}
		}},
		{name: "wrong seller", known: true, want: errs.CodeConversationMismatch, build: func(productID string) application.Conversation {
			return application.Conversation{ID: "c_test", ProductID: productID, BuyerID: buyerA, SellerID: "u_other_seller"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			verifier := &fakeConversationVerifier{conversations: map[string]application.Conversation{}}
			h := newHarnessWithConversationVerifier(t, nil, verifier)
			product := h.create(t, seller, "机械键盘", nil)
			if tt.known {
				verifier.conversations["c_test"] = tt.build(product.GetId())
			}

			_, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
				ActorId: buyerA, ProductId: product.GetId(), ConversationId: stringPtr("c_test"),
				ConversationIdPresent: true,
			})
			assertCode(t, err, tt.want)
			h.assertTotalTrades(t, product.GetId(), 0)
			h.assertOutboxCount(t, application.EventTradeCreated, 0)
		})
	}
}

func TestCreateTradeIdempotentReplayPrecedesConversationRevalidation(t *testing.T) {
	t.Parallel()

	const conversationID = "c_replay"
	verifier := &fakeConversationVerifier{conversations: map[string]application.Conversation{}}
	h := newHarnessWithConversationVerifier(t, nil, verifier)
	product := h.create(t, seller, "机械键盘", nil)
	verifier.conversations[conversationID] = application.Conversation{
		ID: conversationID, ProductID: product.GetId(), BuyerID: buyerA, SellerID: seller,
	}

	req := &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(), ConversationId: stringPtr(conversationID),
		ConversationIdPresent: true, IdempotencyKey: key("conversation-replay"),
	}
	first, err := h.client.CreateTrade(t.Context(), req)
	if err != nil {
		t.Fatalf("first CreateTrade: %v", err)
	}
	if _, err := h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
		ActorId: buyerA, TradeId: first.GetTrade().GetId(), Reason: "取消",
	}); err != nil {
		t.Fatalf("CancelTrade: %v", err)
	}

	// Simulate Messaging becoming unavailable after the first success. The
	// stored result must still replay before any current dependency/state check.
	verifier.err = fmt.Errorf("messaging unavailable")
	replayed, err := h.client.CreateTrade(t.Context(), req)
	if err != nil {
		t.Fatalf("same-key replay: %v", err)
	}
	if !replayed.GetReplayed() || !replayed.GetCreated() ||
		replayed.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_PENDING {
		t.Fatalf("replayed first creation = %+v", replayed)
	}

	current, err := h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
		ActorId: buyerA, ProductId: product.GetId(),
	})
	if err != nil {
		t.Fatalf("fresh create-or-get: %v", err)
	}
	if current.GetCreated() || current.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED {
		t.Fatalf("fresh create-or-get = %+v", current)
	}
}

func TestTradeActionsHideResourcesFromNonParties(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())

	tests := []struct {
		name string
		call func() error
	}{
		{name: "reject", call: func() error {
			_, err := h.client.RejectTrade(t.Context(), &marketplacev1.RejectTradeRequest{
				ActorId: "u_stranger", TradeId: tradeID, Reason: "路过",
			})
			return err
		}},
		{name: "cancel", call: func() error {
			_, err := h.client.CancelTrade(t.Context(), &marketplacev1.CancelTradeRequest{
				ActorId: "u_stranger", TradeId: tradeID, Reason: "路过",
			})
			return err
		}},
		{name: "confirm", call: func() error {
			_, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
				ActorId: "u_stranger", TradeId: tradeID,
			})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCode(t, tt.call(), errs.CodeResourceNotFound)
		})
	}

	_, err := h.client.RejectTrade(t.Context(), &marketplacev1.RejectTradeRequest{
		ActorId: buyerA, TradeId: tradeID, Reason: "买家不能拒绝",
	})
	assertCode(t, err, errs.CodeForbidden)
}

func TestConfirmOnACompletedTradeIsIdempotentWithoutAKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "机械键盘", nil)
	tradeID := h.createTrade(t, buyerA, product.GetId())
	h.accept(t, tradeID)
	if _, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: buyerA, TradeId: tradeID,
	}); err != nil {
		t.Fatalf("buyer ConfirmTrade: %v", err)
	}
	completed, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: seller, TradeId: tradeID,
	})
	if err != nil {
		t.Fatalf("seller ConfirmTrade: %v", err)
	}

	repeated, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
		ActorId: buyerA, TradeId: tradeID,
	})
	if err != nil {
		t.Fatalf("repeated completed ConfirmTrade: %v", err)
	}
	if repeated.GetTrade().GetStatus() != marketplacev1.TradeStatus_TRADE_STATUS_COMPLETED ||
		repeated.GetTrade().GetCompletedAt().AsTime() != completed.GetTrade().GetCompletedAt().AsTime() {
		t.Fatalf("repeated confirmation changed the result: %+v", repeated.GetTrade())
	}
	h.assertOutboxCount(t, application.EventTradeCompleted, 1)
}

func TestCreateTradeAndOffShelfSerializeOnTheProductLock(t *testing.T) {
	t.Parallel()

	for attempt := range 4 {
		t.Run(fmt.Sprintf("attempt-%d", attempt), func(t *testing.T) {
			h := newHarness(t)
			product := h.create(t, seller, "机械键盘", nil)

			var createResp *marketplacev1.CreateTradeResponse
			var createErr, offShelfErr error
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				createResp, createErr = h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
					ActorId: buyerA, ProductId: product.GetId(),
				})
			}()
			go func() {
				defer wg.Done()
				<-start
				_, offShelfErr = h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
					ActorId: seller, ProductId: product.GetId(),
				})
			}()
			close(start)
			wg.Wait()

			switch {
			case createErr == nil:
				if offShelfErr == nil {
					t.Fatal("create and off-shelf both succeeded")
				}
				assertCode(t, offShelfErr, errs.CodeTradeStateConflict)
				if !createResp.GetCreated() {
					t.Fatal("winning create did not create the intent")
				}
				h.assertProductStatus(t, product.GetId(), "ON_SALE")
				h.assertTotalTrades(t, product.GetId(), 1)
			case offShelfErr == nil:
				assertCode(t, createErr, errs.CodeProductNotAvailable)
				h.assertProductStatus(t, product.GetId(), "OFF_SHELF")
				h.assertTotalTrades(t, product.GetId(), 0)
			default:
				t.Fatalf("neither command succeeded: create=%v off-shelf=%v", createErr, offShelfErr)
			}
		})
	}
}

func TestCreateTradeAndPriceEditUseOneCommittedProductVersion(t *testing.T) {
	t.Parallel()

	for attempt := range 4 {
		t.Run(fmt.Sprintf("attempt-%d", attempt), func(t *testing.T) {
			h := newHarness(t)
			product := h.create(t, seller, "机械键盘", nil)
			newPrice := product.GetPriceMinor() + 777

			var createResp *marketplacev1.CreateTradeResponse
			var createErr, editErr error
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				createResp, createErr = h.client.CreateTrade(t.Context(), &marketplacev1.CreateTradeRequest{
					ActorId: buyerA, ProductId: product.GetId(),
				})
			}()
			go func() {
				defer wg.Done()
				<-start
				_, editErr = h.client.UpdateProduct(t.Context(), &marketplacev1.UpdateProductRequest{
					ActorId: seller, ProductId: product.GetId(), PriceMinor: &newPrice,
				})
			}()
			close(start)
			wg.Wait()

			if createErr != nil {
				t.Fatalf("CreateTrade: %v", createErr)
			}
			switch {
			case editErr == nil:
				if createResp.GetTrade().GetPriceSnapshotMinor() != newPrice {
					t.Fatalf("edit committed first but snapshot = %d, want %d",
						createResp.GetTrade().GetPriceSnapshotMinor(), newPrice)
				}
			case errs.CodeOf(editErr) == errs.CodeTradeStateConflict:
				if createResp.GetTrade().GetPriceSnapshotMinor() != product.GetPriceMinor() {
					t.Fatalf("create committed first but snapshot = %d, want %d",
						createResp.GetTrade().GetPriceSnapshotMinor(), product.GetPriceMinor())
				}
			default:
				t.Fatalf("UpdateProduct: %v", editErr)
			}
			h.assertTotalTrades(t, product.GetId(), 1)
		})
	}
}

func stringPtr(value string) *string { return &value }
