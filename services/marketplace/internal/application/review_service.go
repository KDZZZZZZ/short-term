package application

import (
	"context"
	"errors"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// ReviewService 实现买家评论用例。
type ReviewService struct {
	reviews ReviewRepository
	ids     IDGenerator
	clock   Clock
}

// NewReviewService 组装评论用例。
func NewReviewService(reviews ReviewRepository, ids IDGenerator, clock Clock) (*ReviewService, error) {
	if reviews == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every review dependency is required")
	}
	return &ReviewService{reviews: reviews, ids: ids, clock: clock}, nil
}

// Create 在买家交易完成后为其发布一条商品评论。
//
// 评论不改变商品或交易状态，因此命令在一个小事务中完成：读取商品存在性、
// 读取买家已完成的终生唯一交易、插入评论。同一买家的并发重复提交由
// (product_id, buyer_id) 唯一约束仲裁，只有一个事务能提交并返回评论，
// 其余返回 REVIEW_ALREADY_EXISTS（docs/state-machines.md，买家评论）。
func (s *ReviewService) Create(ctx context.Context, cmd CreateReviewCommand) (*domain.Review, error) {
	if cmd.ActorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if err := domain.ValidateReviewComment(cmd.Comment); err != nil {
		return nil, errs.Wrap(errs.CodeValidation, "评论长度必须为 1 至 500 个字符", err)
	}

	review, err := domain.NewReview(s.ids.NewReviewID(), cmd.ProductID, cmd.ActorID, cmd.Comment, s.clock.Now())
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "评论初始化失败", err)
	}

	err = s.reviews.Execute(ctx, func(ctx context.Context, tx ReviewTx) error {
		exists, err := tx.ProductExists(ctx, cmd.ProductID)
		if err != nil {
			return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
		}
		if !exists {
			return errs.New(errs.CodeResourceNotFound, "商品不存在")
		}

		trade, err := tx.CompletedTradeForBuyer(ctx, cmd.ProductID, cmd.ActorID)
		if errors.Is(err, ErrNotFound) {
			return errs.New(errs.CodeForbidden, "只有完成交易的买家可以评论该商品")
		}
		if err != nil {
			return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
		}

		review.TradeID = trade.ID
		if err := tx.InsertReview(ctx, review); err != nil {
			return insertReviewError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return review, nil
}

// List 返回商品收到的全部评论。评论与商品状态无关，
// 但商品本身必须存在。
func (s *ReviewService) List(ctx context.Context, query ListReviewsQuery) (ReviewPage, error) {
	exists, err := s.reviews.ProductExists(ctx, query.ProductID)
	if err != nil {
		return ReviewPage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	if !exists {
		return ReviewPage{}, errs.New(errs.CodeResourceNotFound, "商品不存在")
	}

	page, err := s.reviews.ListProductReviews(ctx, query.ProductID, query.Page.normalize())
	if err != nil {
		return ReviewPage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return page, nil
}

// insertReviewError 映射评论写入失败。唯一冲突是预期并发结果，
// 外键冲突只可能表示存储不变量被绕过。
func insertReviewError(err error) error {
	switch {
	case errors.Is(err, ErrReviewAlreadyExists):
		return errs.Wrap(errs.CodeReviewAlreadyExists, "已经评论过该商品", err)
	case errors.Is(err, ErrNotFound):
		return errs.Wrap(errs.CodeResourceNotFound, "商品不存在", err)
	default:
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
}
