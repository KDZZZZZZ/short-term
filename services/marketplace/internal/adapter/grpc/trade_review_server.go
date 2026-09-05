package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// maxBatchTradeReviews 限制一次 BatchGetProductTradeReviews 调用。公开卖家
// 商品列表和我的商品页一次补全一页，最大公开页面为 100 行。
const maxBatchTradeReviews = 200

// CreateTradeReview 在交易完成后由买家发布评价。授权与唯一性校验都在应用层完成。
func (s *Server) CreateTradeReview(ctx context.Context, req *marketplacev1.CreateTradeReviewRequest) (*marketplacev1.CreateTradeReviewResponse, error) {
	review, err := s.tradeReviews.Create(ctx, application.CreateTradeReviewCommand{
		ActorID: req.GetActorId(),
		TradeID: req.GetTradeId(),
		Content: req.GetContent(),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.CreateTradeReviewResponse{Review: tradeReviewProto(review)}, nil
}

// BatchGetProductTradeReviews 返回请求商品各自的买家评价（每个商品最多一条）。
func (s *Server) BatchGetProductTradeReviews(ctx context.Context, req *marketplacev1.BatchGetProductTradeReviewsRequest) (*marketplacev1.BatchGetProductTradeReviewsResponse, error) {
	if len(req.GetProductIds()) > maxBatchTradeReviews {
		return nil, errs.Newf(errs.CodeValidation, "单次最多查询 %d 个商品的评价", maxBatchTradeReviews)
	}

	reviews, err := s.tradeReviews.ByProductIDs(ctx, req.GetProductIds())
	if err != nil {
		return nil, err
	}

	mapped := make(map[string]*marketplacev1.TradeReview, len(reviews))
	for productID, review := range reviews {
		mapped[productID] = tradeReviewProto(review)
	}
	return &marketplacev1.BatchGetProductTradeReviewsResponse{Reviews: mapped}, nil
}

func tradeReviewProto(review *domain.TradeReview) *marketplacev1.TradeReview {
	return &marketplacev1.TradeReview{
		Id:        review.ID,
		TradeId:   review.TradeID,
		ProductId: review.ProductID,
		BuyerId:   review.BuyerID,
		Content:   review.Content,
		CreatedAt: timestamppb.New(review.CreatedAt),
	}
}
