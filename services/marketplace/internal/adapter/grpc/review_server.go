package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// CreateReview 发布买家评论。授权与唯一性校验都在应用层事务中完成。
func (s *Server) CreateReview(ctx context.Context, req *marketplacev1.CreateReviewRequest) (*marketplacev1.CreateReviewResponse, error) {
	review, err := s.reviews.Create(ctx, application.CreateReviewCommand{
		ActorID:   req.GetActorId(),
		ProductID: req.GetProductId(),
		Comment:   req.GetComment(),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.CreateReviewResponse{Review: reviewProto(review)}, nil
}

// ListProductReviews 返回商品的全部买家评论。
func (s *Server) ListProductReviews(ctx context.Context, req *marketplacev1.ListProductReviewsRequest) (*marketplacev1.ListProductReviewsResponse, error) {
	page, err := s.reviews.List(ctx, application.ListReviewsQuery{
		ProductID: req.GetProductId(),
		Page:      application.Page{Number: req.GetPage(), Size: req.GetPageSize()},
	})
	if err != nil {
		return nil, err
	}

	items := make([]*marketplacev1.Review, 0, len(page.Items))
	for _, review := range page.Items {
		items = append(items, reviewProto(review))
	}
	return &marketplacev1.ListProductReviewsResponse{Page: &marketplacev1.ReviewPage{
		Items:    items,
		Page:     page.Page,
		PageSize: page.Size,
		Total:    page.Total,
	}}, nil
}

func reviewProto(review *domain.Review) *marketplacev1.Review {
	return &marketplacev1.Review{
		Id:        review.ID,
		ProductId: review.ProductID,
		BuyerId:   review.BuyerID,
		Comment:   review.Comment,
		CreatedAt: timestamppb.New(review.CreatedAt),
	}
}
