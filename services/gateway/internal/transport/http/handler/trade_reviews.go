package handler

import (
	"context"
	"net/http"
	"unicode/utf8"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/application/aggregation"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/mapper"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/middleware"
)

// maxTradeReviewContentLength 与 openapi/components/schemas.yaml#/TradeReviewContent 一致。
const maxTradeReviewContentLength = 500

// TradeReviews serves POST /trades/{tradeId}/review.
type TradeReviews struct {
	marketplace marketplacev1.MarketplaceServiceClient
	aggregator  *aggregation.Aggregator
	responder   Responder
}

// NewTradeReviews builds the trade review handler.
func NewTradeReviews(marketplace marketplacev1.MarketplaceServiceClient, aggregator *aggregation.Aggregator, responder Responder) *TradeReviews {
	return &TradeReviews{marketplace: marketplace, aggregator: aggregator, responder: responder}
}

// Create handles POST /trades/{tradeId}/review.
//
// 评价是一次性的不可变命令：每笔交易最多一条，重复提交返回
// 409 TRADE_REVIEW_ALREADY_EXISTS。交易可见性（对非交易方隐藏存在性）、
// 买家身份和 COMPLETED 状态都由下游校验。
func (h *TradeReviews) Create(w http.ResponseWriter, r *http.Request) {
	var body dto.TradeReviewCreateRequest
	if !decodeJSON(w, r, h.responder, &body) {
		return
	}
	if length := utf8.RuneCountInString(body.Content); length < 1 || length > maxTradeReviewContentLength {
		h.responder.Fail(w, r, errs.CodeValidation, "评价长度必须为 1 至 500 个字符")
		return
	}

	review, err := h.marketplace.CreateTradeReview(h.downstream(r), &marketplacev1.CreateTradeReviewRequest{
		ActorId: middleware.ActorID(r.Context()),
		TradeId: r.PathValue("tradeId"),
		Content: body.Content,
	})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	users, err := h.aggregator.Users(h.downstream(r), []string{review.GetReview().GetBuyerId()})
	if err != nil {
		h.responder.Error(w, r, err)
		return
	}

	h.responder.Created(w, r, mapper.TradeReview(review.GetReview(), users))
}

// downstream builds the context for an internal call.
func (h *TradeReviews) downstream(r *http.Request) context.Context {
	ctx := grpcx.WithActor(r.Context(), middleware.ActorID(r.Context()))
	return grpcx.WithRequestID(ctx, middleware.RequestID(r.Context()))
}
