package grpc_test

import (
	"strings"
	"sync"
	"testing"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
)

const reviewBuyer = "u_review_buyer"
const reviewObserver = "u_review_observer"

// completeTrade 走完 PENDING -> ACCEPTED -> COMPLETED 的全部确认，
// 使买家获得评论该商品的资格。
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

// createReview 调用评论创建并返回结果与错误，供各用例断言。
func (h harness) createReview(t *testing.T, actorID, productID, comment string) (*marketplacev1.Review, error) {
	t.Helper()

	resp, err := h.client.CreateReview(t.Context(), &marketplacev1.CreateReviewRequest{
		ActorId: actorID, ProductId: productID, Comment: comment,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetReview(), nil
}

func TestCompletedBuyerCanReviewOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "可评论的机械键盘", nil)
	h.completeTrade(t, reviewBuyer, product.GetId())

	review, err := h.createReview(t, reviewBuyer, product.GetId(), "很划算，成色如描述")
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if review.GetProductId() != product.GetId() || review.GetBuyerId() != reviewBuyer {
		t.Fatalf("review = %+v, want product %s and buyer %s", review, product.GetId(), reviewBuyer)
	}
	if review.GetComment() == "" || review.GetCreatedAt() == nil {
		t.Fatalf("review is missing comment or timestamps: %+v", review)
	}

	_, err = h.createReview(t, reviewBuyer, product.GetId(), "重复评论")
	assertCode(t, err, errs.CodeReviewAlreadyExists)

	page, err := h.client.ListProductReviews(t.Context(), &marketplacev1.ListProductReviewsRequest{
		ProductId: product.GetId(), Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListProductReviews: %v", err)
	}
	if page.GetPage().GetTotal() != 1 || len(page.GetPage().GetItems()) != 1 {
		t.Fatalf("review page = %+v, want exactly one row", page.GetPage())
	}
	if got := page.GetPage().GetItems()[0].GetId(); got != review.GetId() {
		t.Fatalf("listed review id = %s, want %s", got, review.GetId())
	}
}

func TestReviewIsDeniedWithoutACompletedBuyerTrade(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "未完成交易的机械键盘", nil)

	// 没有任何交易记录的旁观者。
	_, err := h.createReview(t, reviewObserver, product.GetId(), "旁观者评论")
	assertCode(t, err, errs.CodeForbidden)

	// 卖家自己不是该商品的买家。
	_, err = h.createReview(t, seller, product.GetId(), "卖家自评")
	assertCode(t, err, errs.CodeForbidden)

	// 已创建意向但交易尚未完成。
	h.createTrade(t, reviewBuyer, product.GetId())
	_, err = h.createReview(t, reviewBuyer, product.GetId(), "交易未完成就想评论")
	assertCode(t, err, errs.CodeForbidden)

	// 商品不存在时创建和列表都必须是 404。
	_, err = h.createReview(t, reviewBuyer, "p_missing", "商品不存在")
	assertCode(t, err, errs.CodeResourceNotFound)
	_, err = h.client.ListProductReviews(t.Context(), &marketplacev1.ListProductReviewsRequest{ProductId: "p_missing"})
	assertCode(t, err, errs.CodeResourceNotFound)
}

func TestReviewRejectsInvalidComments(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "评论校验机械键盘", nil)
	h.completeTrade(t, reviewBuyer, product.GetId())

	_, err := h.createReview(t, reviewBuyer, product.GetId(), "")
	assertCode(t, err, errs.CodeValidation)
	_, err = h.createReview(t, reviewBuyer, product.GetId(), strings.Repeat("好", 501))
	assertCode(t, err, errs.CodeValidation)

	// 500 个字符仍然有效，且失败请求不能消耗唯一名额。
	_, err = h.createReview(t, reviewBuyer, product.GetId(), strings.Repeat("好", 500))
	if err != nil {
		t.Fatalf("CreateReview with a 500 character comment: %v", err)
	}
}

func TestConcurrentDuplicateReviewsProduceExactlyOneSuccess(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "并发评论机械键盘", nil)
	h.completeTrade(t, reviewBuyer, product.GetId())

	const attempts = 8
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.client.CreateReview(t.Context(), &marketplacev1.CreateReviewRequest{
				ActorId: reviewBuyer, ProductId: product.GetId(), Comment: "并发评论",
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	succeeded := 0
	duplicated := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errs.CodeOf(err) == errs.CodeReviewAlreadyExists:
			duplicated++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || duplicated != attempts-1 {
		t.Fatalf("succeeded = %d, duplicated = %d, want exactly one success", succeeded, duplicated)
	}
}

func TestReviewListIsScopedToItsProduct(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	productOne := h.create(t, seller, "评论作用域机械键盘一", nil)
	productTwo := h.create(t, seller, "评论作用域机械键盘二", nil)
	h.completeTrade(t, reviewBuyer, productOne.GetId())
	h.completeTrade(t, reviewBuyer, productTwo.GetId())

	reviewOne, err := h.createReview(t, reviewBuyer, productOne.GetId(), "第一件商品的评价")
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if _, err := h.createReview(t, reviewBuyer, productTwo.GetId(), "第二件商品的评价"); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}

	page, err := h.client.ListProductReviews(t.Context(), &marketplacev1.ListProductReviewsRequest{
		ProductId: productOne.GetId(), Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListProductReviews: %v", err)
	}
	if page.GetPage().GetTotal() != 1 || len(page.GetPage().GetItems()) != 1 {
		t.Fatalf("review page = %+v, want only the product's own review", page.GetPage())
	}
	if page.GetPage().GetItems()[0].GetId() != reviewOne.GetId() {
		t.Fatalf("listed review id = %s, want %s", page.GetPage().GetItems()[0].GetId(), reviewOne.GetId())
	}
}
