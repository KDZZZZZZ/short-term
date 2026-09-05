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

// maxReviewCommentLength 与 openapi/components/schemas.yaml#/ReviewComment 一致。
const maxReviewCommentLength = 500

// Reviews serves the product review endpoints.
type Reviews struct {
	marketplace marketplacev1.MarketplaceServiceClient
	aggregator  *aggregation.Aggregator
	responder   Responder
}

// NewReviews builds the review handler.
func NewReviews(marketplace marketplacev1.MarketplaceServiceClient, aggregator *aggregation.Aggregator, responder Responder) *Reviews {
	return &Reviews{marketplace: marketplace, aggregator: aggregator, responder: responder}
}

// Create handles POST /products/{productId}/reviews.
//
// 评论是一次性的不可变命令：服务端以 (product, buyer) 唯一约束仲裁重复提交，
// 重复请求得到 409 REVIEW_ALREADY_EXISTS 而不是第一份响应。客户端重试后可以
// 用列表接口确认自己的评论是否已经入库。
func (h *Reviews) Create(w http.ResponseWriter, r *http.Request) {
	var body dto.ReviewCreateRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}
	if length := utf8.RuneCountInString(body.Comment); length < 1 || length > maxReviewCommentLength {
		h.responder.Fail(w, r, errs.CodeValidation, "评论长度必须为 1 至 500 个字符")
		return
	}

	review, err := h.marketplace.CreateReview(h.downstream(r), &marketplacev1.CreateReviewRequest{
		ActorId:   middleware.ActorID(r.Context()),
		ProductId: r.PathValue("productId"),
		Comment:   body.Comment,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.respondWithReview(w, r, http.StatusCreated, review.GetReview())
}

// List handles GET /products/{productId}/reviews.
func (h *Reviews) List(w http.ResponseWriter, r *http.Request) {
	page, size, ok := h.pagination(w, r)
	if !ok {
		return
	}

	resp, err := h.marketplace.ListProductReviews(h.downstream(r), &marketplacev1.ListProductReviewsRequest{
		ProductId: r.PathValue("productId"),
		Page:      page,
		PageSize:  size,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	users, err := h.aggregator.Users(h.downstream(r), mapper.ReviewBuyerIDs(resp.GetPage()))
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.OK(w, r, mapper.ReviewPage(resp.GetPage(), users))
}

// respondWithReview completes the buyer identity and writes the review.
func (h *Reviews) respondWithReview(w http.ResponseWriter, r *http.Request, status int, review *marketplacev1.Review) {
	users, err := h.aggregator.Users(h.downstream(r), []string{review.GetBuyerId()})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Success(w, r, status, mapper.Review(review, users))
}

// pagination reads the page and page_size query parameters.
func (h *Reviews) pagination(w http.ResponseWriter, r *http.Request) (page, size int32, ok bool) {
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

func (h *Reviews) intQuery(w http.ResponseWriter, r *http.Request, name string, fallback, minimum, maximum int32) (int32, bool) {
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
func (h *Reviews) downstream(r *http.Request) context.Context {
	ctx := grpcx.WithActor(r.Context(), middleware.ActorID(r.Context()))
	return grpcx.WithRequestID(ctx, middleware.RequestID(r.Context()))
}
