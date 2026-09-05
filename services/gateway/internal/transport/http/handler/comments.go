package handler

import (
	"context"
	"net/http"
	"strconv"
	"unicode/utf8"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// maxCommentContentLength 与 openapi/components/schemas.yaml#/CommentContent 一致。
const maxCommentContentLength = 500

// Comments serves the product comment endpoints.
type Comments struct {
	marketplace marketplacev1.MarketplaceServiceClient
	aggregator  *aggregation.Aggregator
	responder   Responder
}

// NewComments builds the comment handler.
func NewComments(marketplace marketplacev1.MarketplaceServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Comments {
	return &Comments{marketplace: marketplace, aggregator: aggregator, responder: responder}
}

// Create handles POST /products/{productId}/comments.
//
// 评论对任意已认证用户开放：不要求购买，不限商品状态，同一用户可以发布
// 多条评论。商品存在性由下游校验，404 原样映射。
func (h *Comments) Create(w http.ResponseWriter, r *http.Request) {
	var body dto.CommentCreateRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}
	if length := utf8.RuneCountInString(body.Content); length < 1 || length > maxCommentContentLength {
		h.responder.Fail(w, r, errs.CodeValidation, "评论长度必须为 1 至 500 个字符")
		return
	}

	comment, err := h.marketplace.CreateProductComment(h.downstream(r), &marketplacev1.CreateProductCommentRequest{
		ActorId:   middleware.ActorID(r.Context()),
		ProductId: r.PathValue("productId"),
		Content:   body.Content,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithComment(w, r, http.StatusCreated, comment.GetComment())
}

// List handles GET /products/{productId}/comments.
func (h *Comments) List(w http.ResponseWriter, r *http.Request) {
	page, size, ok := h.pagination(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.ListProductComments(h.downstream(r), &marketplacev1.ListProductCommentsRequest{
		ProductId: r.PathValue("productId"),
		Page:      page,
		PageSize:  size,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	users, err := h.aggregator.Users(h.downstream(r), mapper.CommentUserIDs(resp.GetPage()))
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.CommentPage(resp.GetPage(), users))
}

// respondWithComment completes the commenter identity and writes the comment.
func (h *Comments) respondWithComment(w http.ResponseWriter, r *http.Request, status int, comment *marketplacev1.ProductComment) {
	users, err := h.aggregator.Users(h.downstream(r), []string{comment.GetUserId()})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Success(w, r, status, mapper.Comment(comment, users))
}

// pagination reads the page and page_size query parameters.
func (h *Comments) pagination(w http.ResponseWriter, r *http.Request) (page, size int32, ok bool) {
	page, ok = h.intQuery(w, r, "page", 1, 1, 0)
	if !ok {
		return 0, 0, false
	}
	size, ok = h.intQuery(w, r, "page_size", 20, 1, 100)
	if !ok {
		return 0, 0, false
	}
	return page, size, true
}

func (h *Comments) intQuery(w http.ResponseWriter, r *http.Request, name string, fallback, minimum, maximum int32) (int32, bool) {
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

// downstream builds the context for an internal call.
func (h *Comments) downstream(r *http.Request) context.Context {
	ctx := grpcx.WithActor(r.Context(), middleware.ActorID(r.Context()))
	return grpcx.WithRequestID(ctx, middleware.RequestID(r.Context()))
}
