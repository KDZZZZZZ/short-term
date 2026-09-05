package grpc_test

import (
	"strings"
	"testing"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
)

const tradeReviewBuyer = "u_trade_review_buyer"
const tradeReviewObserver = "u_trade_review_observer"

// completeTrade 走完 PENDING -> ACCEPTED -> COMPLETED 的全部确认。
func (h harness) completeTrade(t *testing.T, buyerID, productID string) string {
	t.Helper()

	tradeID := h.createTrade(t, buyerID, productID)
	h.accept(t, tradeID)
	for _, actor := range []string{seller, buyerID} {
		if _, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
			ActorId: actor, TradeId: tradeID,
		}); err != nil {
			t.Fatalf("ConfirmTrade(%s): %v", actor, err)
		}
	}
	return tradeID
}

// createTradeReview 调用买家评价创建并返回结果与错误，供各用例断言。
func (h harness) createTradeReview(t *testing.T, actorID, tradeID string, score int32, content string) (*marketplacev1.TradeReview, error) {
	t.Helper()

	resp, err := h.client.CreateTradeReview(t.Context(), &marketplacev1.CreateTradeReviewRequest{
		ActorId: actorID, TradeId: tradeID, Score: score, Content: content,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetReview(), nil
}

func TestCompletedBuyerCanReviewTheTradeOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "可评价的机械键盘", nil)
	tradeID := h.completeTrade(t, tradeReviewBuyer, product.GetId())

	review, err := h.createTradeReview(t, tradeReviewBuyer, tradeID, 5, "交易愉快，成色如描述")
	if err != nil {
		t.Fatalf("CreateTradeReview: %v", err)
	}
	if review.GetTradeId() != tradeID || review.GetProductId() != product.GetId() || review.GetBuyerId() != tradeReviewBuyer {
		t.Fatalf("review = %+v, want trade %s, product %s and buyer %s", review, tradeID, product.GetId(), tradeReviewBuyer)
	}
	if review.GetScore() != 5 {
		t.Fatalf("review score = %d, want 5", review.GetScore())
	}

	_, err = h.createTradeReview(t, tradeReviewBuyer, tradeID, 5, "重复评价")
	assertCode(t, err, errs.CodeTradeReviewExists)

	batch, err := h.client.BatchGetProductTradeReviews(t.Context(), &marketplacev1.BatchGetProductTradeReviewsRequest{
		ProductIds: []string{product.GetId()},
	})
	if err != nil {
		t.Fatalf("BatchGetProductTradeReviews: %v", err)
	}
	if got := batch.GetReviews()[product.GetId()]; got == nil || got.GetId() != review.GetId() {
		t.Fatalf("batch review = %v, want the created review", got)
	}
}

func TestTradeReviewIsDeniedBeforeCompletionAndToNonBuyers(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "未完成交易的机械键盘", nil)
	tradeID := h.createTrade(t, tradeReviewBuyer, product.GetId())

	// 交易尚未完成。
	_, err := h.createTradeReview(t, tradeReviewBuyer, tradeID, 5, "交易未完成就想评价")
	assertCode(t, err, errs.CodeForbidden)

	// 卖家是交易方但不是买家。
	h.accept(t, tradeID)
	_, err = h.createTradeReview(t, seller, tradeID, 4, "卖家不能评价自己的交易")
	assertCode(t, err, errs.CodeForbidden)

	// 交易完成后卖家仍不能评价。
	for _, actor := range []string{seller, tradeReviewBuyer} {
		if _, err := h.client.ConfirmTrade(t.Context(), &marketplacev1.ConfirmTradeRequest{
			ActorId: actor, TradeId: tradeID,
		}); err != nil {
			t.Fatalf("ConfirmTrade(%s): %v", actor, err)
		}
	}
	_, err = h.createTradeReview(t, seller, tradeID, 4, "交易完成后卖家仍不能评价")
	assertCode(t, err, errs.CodeForbidden)

	// 非交易方看不到交易的存在性。
	_, err = h.createTradeReview(t, tradeReviewObserver, tradeID, 5, "旁观者评价")
	assertCode(t, err, errs.CodeResourceNotFound)

	// 交易不存在。
	_, err = h.createTradeReview(t, tradeReviewBuyer, "t_missing", 5, "交易不存在")
	assertCode(t, err, errs.CodeResourceNotFound)
}

func TestTradeReviewRejectsInvalidContent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "评价校验机械键盘", nil)
	tradeID := h.completeTrade(t, tradeReviewBuyer, product.GetId())

	_, err := h.createTradeReview(t, tradeReviewBuyer, tradeID, 0, "")
	assertCode(t, err, errs.CodeValidation)
	_, err = h.createTradeReview(t, tradeReviewBuyer, tradeID, 5, strings.Repeat("好", 501))
	assertCode(t, err, errs.CodeValidation)

	// 500 个字符仍然有效，且失败请求不能消耗唯一名额。
	if _, err := h.createTradeReview(t, tradeReviewBuyer, tradeID, 5, strings.Repeat("好", 500)); err != nil {
		t.Fatalf("CreateTradeReview with a 500 character content: %v", err)
	}
}

func TestSellerProductListFiltersPublicStatuses(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	onSale := h.create(t, seller, "在售机械键盘", nil)
	sold := h.create(t, seller, "已售出机械键盘", nil)
	tradeID := h.completeTrade(t, tradeReviewBuyer, sold.GetId())
	offShelf := h.create(t, seller, "已下架机械键盘", nil)
	if _, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: offShelf.GetId(),
	}); err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}

	page, err := h.client.ListUserProducts(t.Context(), &marketplacev1.ListUserProductsRequest{
		SellerId: seller,
		Statuses: []marketplacev1.ProductStatus{
			marketplacev1.ProductStatus_PRODUCT_STATUS_ON_SALE,
			marketplacev1.ProductStatus_PRODUCT_STATUS_SOLD,
		},
		Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListUserProducts: %v", err)
	}
	items := page.GetPage().GetItems()
	if page.GetPage().GetTotal() != 2 || len(items) != 2 {
		t.Fatalf("public seller page = %+v, want exactly the on-sale and sold products", page.GetPage())
	}
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.GetId()] = true
	}
	if !seen[onSale.GetId()] || !seen[sold.GetId()] || seen[offShelf.GetId()] {
		t.Fatalf("public seller page contains unexpected products: %v", seen)
	}

	// 卖家自己的管理视图不受影响：仍能看到全部状态。
	mine, err := h.client.ListUserProducts(t.Context(), &marketplacev1.ListUserProductsRequest{
		SellerId: seller, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListUserProducts without filter: %v", err)
	}
	if mine.GetPage().GetTotal() != 3 {
		t.Fatalf("seller page total = %d, want 3", mine.GetPage().GetTotal())
	}
	_ = tradeID
}

func TestUserStatsCountsSellerActivity(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	sold := h.create(t, seller, "已售出统计机械键盘", nil)
	tradeID := h.completeTrade(t, tradeReviewBuyer, sold.GetId())
	if _, err := h.createTradeReview(t, tradeReviewBuyer, tradeID, 5, "统计用评价"); err != nil {
		t.Fatalf("CreateTradeReview: %v", err)
	}
	h.create(t, seller, "在售统计机械键盘", nil)
	offShelf := h.create(t, seller, "已下架统计机械键盘", nil)
	if _, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: offShelf.GetId(),
	}); err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}

	stats, err := h.client.GetUserStats(t.Context(), &marketplacev1.GetUserStatsRequest{UserId: seller})
	if err != nil {
		t.Fatalf("GetUserStats: %v", err)
	}
	if stats.GetCompletedTrades() != 1 {
		t.Fatalf("completed trades = %d, want 1", stats.GetCompletedTrades())
	}
	if stats.GetOnSaleProducts() != 1 {
		t.Fatalf("on-sale products = %d, want 1 (OFF_SHELF and SOLD must not count)", stats.GetOnSaleProducts())
	}

	idle, err := h.client.GetUserStats(t.Context(), &marketplacev1.GetUserStatsRequest{UserId: tradeReviewBuyer})
	if err != nil {
		t.Fatalf("GetUserStats: %v", err)
	}
	if idle.GetCompletedTrades() != 0 || idle.GetOnSaleProducts() != 0 {
		t.Fatalf("a user without selling activity must have zero stats: %+v", idle)
	}
}
