package grpc_test

import (
	"strings"
	"testing"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
)

const commentUser = "u_comment_user"
const commentObserver = "u_comment_observer"

// createComment 调用评论创建并返回结果与错误，供各用例断言。
func (h harness) createComment(t *testing.T, actorID, productID, content string) (*marketplacev1.ProductComment, error) {
	t.Helper()

	resp, err := h.client.CreateProductComment(t.Context(), &marketplacev1.CreateProductCommentRequest{
		ActorId: actorID, ProductId: productID, Content: content,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetComment(), nil
}

func TestAnyUserCanCommentOnAnyVisibleProduct(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "开放评论的机械键盘", nil)

	// 没有任何交易关系的旁观者。
	first, err := h.createComment(t, commentObserver, product.GetId(), "围观者的评论")
	if err != nil {
		t.Fatalf("CreateProductComment: %v", err)
	}
	// 卖家可以评论自己发布的商品。
	second, err := h.createComment(t, seller, product.GetId(), "卖家补充说明")
	if err != nil {
		t.Fatalf("CreateProductComment: %v", err)
	}
	// 已创建意向但未完成交易的买家同样可以评论。
	h.createTrade(t, commentUser, product.GetId())
	third, err := h.createComment(t, commentUser, product.GetId(), "意向买家的提问")
	if err != nil {
		t.Fatalf("CreateProductComment: %v", err)
	}
	if first.GetUserId() != commentObserver || second.GetUserId() != seller || third.GetUserId() != commentUser {
		t.Fatalf("comments = %+v/%+v/%+v, want the acting user on each", first, second, third)
	}

	page, err := h.client.ListProductComments(t.Context(), &marketplacev1.ListProductCommentsRequest{
		ProductId: product.GetId(), Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListProductComments: %v", err)
	}
	if page.GetPage().GetTotal() != 3 || len(page.GetPage().GetItems()) != 3 {
		t.Fatalf("comment page = %+v, want three rows", page.GetPage())
	}
	if page.GetPage().GetItems()[0].GetId() != third.GetId() {
		t.Fatalf("the newest comment is not listed first: %s", page.GetPage().GetItems()[0].GetId())
	}
}

func TestUserCanCommentOnAnyProductStatusAndMultipleTimes(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "下架后仍可评论的机械键盘", nil)
	if _, err := h.client.OffShelfProduct(t.Context(), &marketplacev1.OffShelfProductRequest{
		ActorId: seller, ProductId: product.GetId(),
	}); err != nil {
		t.Fatalf("OffShelfProduct: %v", err)
	}

	first, err := h.createComment(t, commentUser, product.GetId(), "第一条评论")
	if err != nil {
		t.Fatalf("CreateProductComment on OFF_SHELF product: %v", err)
	}
	second, err := h.createComment(t, commentUser, product.GetId(), "同一用户的第二条评论")
	if err != nil {
		t.Fatalf("CreateProductComment: %v", err)
	}
	if first.GetId() == second.GetId() {
		t.Fatal("two comments from the same user must be independent rows")
	}

	page, err := h.client.ListProductComments(t.Context(), &marketplacev1.ListProductCommentsRequest{
		ProductId: product.GetId(), Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListProductComments: %v", err)
	}
	if page.GetPage().GetTotal() != 2 {
		t.Fatalf("comment total = %d, want 2", page.GetPage().GetTotal())
	}
	if page.GetPage().GetItems()[0].GetId() != second.GetId() {
		t.Fatalf("the newest comment is not listed first: %s", page.GetPage().GetItems()[0].GetId())
	}
}

func TestCommentRejectsMissingProductAndInvalidContent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	product := h.create(t, seller, "评论校验机械键盘", nil)

	_, err := h.createComment(t, commentUser, "p_missing", "商品不存在")
	assertCode(t, err, errs.CodeResourceNotFound)
	_, err = h.client.ListProductComments(t.Context(), &marketplacev1.ListProductCommentsRequest{ProductId: "p_missing"})
	assertCode(t, err, errs.CodeResourceNotFound)

	_, err = h.createComment(t, commentUser, product.GetId(), "")
	assertCode(t, err, errs.CodeValidation)
	_, err = h.createComment(t, commentUser, product.GetId(), strings.Repeat("好", 501))
	assertCode(t, err, errs.CodeValidation)

	// 500 个字符仍然有效，且失败请求不能写入任何行。
	if _, err := h.createComment(t, commentUser, product.GetId(), strings.Repeat("好", 500)); err != nil {
		t.Fatalf("CreateProductComment with a 500 character content: %v", err)
	}
	page, err := h.client.ListProductComments(t.Context(), &marketplacev1.ListProductCommentsRequest{
		ProductId: product.GetId(), Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListProductComments: %v", err)
	}
	if page.GetPage().GetTotal() != 1 {
		t.Fatalf("comment total = %d, want 1 (invalid requests must not insert rows)", page.GetPage().GetTotal())
	}
}

func TestCommentListIsScopedToItsProduct(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	productOne := h.create(t, seller, "评论作用域机械键盘一", nil)
	productTwo := h.create(t, seller, "评论作用域机械键盘二", nil)

	commentOne, err := h.createComment(t, commentUser, productOne.GetId(), "第一件商品的评价")
	if err != nil {
		t.Fatalf("CreateProductComment: %v", err)
	}
	if _, err := h.createComment(t, commentUser, productTwo.GetId(), "第二件商品的评价"); err != nil {
		t.Fatalf("CreateProductComment: %v", err)
	}

	page, err := h.client.ListProductComments(t.Context(), &marketplacev1.ListProductCommentsRequest{
		ProductId: productOne.GetId(), Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListProductComments: %v", err)
	}
	if page.GetPage().GetTotal() != 1 || len(page.GetPage().GetItems()) != 1 {
		t.Fatalf("comment page = %+v, want only the product's own comment", page.GetPage())
	}
	if page.GetPage().GetItems()[0].GetId() != commentOne.GetId() {
		t.Fatalf("listed comment id = %s, want %s", page.GetPage().GetItems()[0].GetId(), commentOne.GetId())
	}
}
