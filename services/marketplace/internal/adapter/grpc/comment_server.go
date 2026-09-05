package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// CreateProductComment 为任意已认证用户发布商品评论。商品存在性等校验
// 都在应用层完成。
func (s *Server) CreateProductComment(ctx context.Context, req *marketplacev1.CreateProductCommentRequest) (*marketplacev1.CreateProductCommentResponse, error) {
	comment, err := s.comments.Create(ctx, application.CreateCommentCommand{
		ActorID:   req.GetActorId(),
		ProductID: req.GetProductId(),
		Content:   req.GetContent(),
	})
	if err != nil {
		return nil, err
	}
	return &marketplacev1.CreateProductCommentResponse{Comment: commentProto(comment)}, nil
}

// ListProductComments 返回商品的全部用户评论。
func (s *Server) ListProductComments(ctx context.Context, req *marketplacev1.ListProductCommentsRequest) (*marketplacev1.ListProductCommentsResponse, error) {
	page, err := s.comments.List(ctx, application.ListCommentsQuery{
		ProductID: req.GetProductId(),
		Page:      application.Page{Number: req.GetPage(), Size: req.GetPageSize()},
	})
	if err != nil {
		return nil, err
	}

	items := make([]*marketplacev1.ProductComment, 0, len(page.Items))
	for _, comment := range page.Items {
		items = append(items, commentProto(comment))
	}
	return &marketplacev1.ListProductCommentsResponse{Page: &marketplacev1.ProductCommentPage{
		Items:    items,
		Page:     page.Page,
		PageSize: page.Size,
		Total:    page.Total,
	}}, nil
}

func commentProto(comment *domain.ProductComment) *marketplacev1.ProductComment {
	return &marketplacev1.ProductComment{
		Id:        comment.ID,
		ProductId: comment.ProductID,
		UserId:    comment.UserID,
		Content:   comment.Content,
		CreatedAt: timestamppb.New(comment.CreatedAt),
	}
}
