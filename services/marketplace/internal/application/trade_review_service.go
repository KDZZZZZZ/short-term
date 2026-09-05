package application

import (
	"context"
	"errors"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// TradeReviewService 实现买家评价用例。
type TradeReviewService struct {
	trades  TradeRepository
	reviews TradeReviewRepository
	ids     IDGenerator
	clock   Clock
}

// NewTradeReviewService 组装买家评价用例。
func NewTradeReviewService(trades TradeRepository, reviews TradeReviewRepository, ids IDGenerator, clock Clock) (*TradeReviewService, error) {
	if trades == nil || reviews == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every trade review dependency is required")
	}
	return &TradeReviewService{trades: trades, reviews: reviews, ids: ids, clock: clock}, nil
}

// Create 在交易完成后由买家发布一条评价。
//
// docs/state-machines.md：每笔交易最多一条评价，发布后不可变，评价不改变
// 商品或交易状态。交易不存在或当前用户不是交易方时按交易不可见处理
// （与 GetTrade 一致，不泄露交易标识的存在性）；交易未完成或操作者是卖家
// 返回 FORBIDDEN；并发重复提交由 trade_id 唯一约束仲裁。
func (s *TradeReviewService) Create(ctx context.Context, cmd CreateTradeReviewCommand) (*domain.TradeReview, error) {
	if cmd.ActorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if err := domain.ValidateTradeReviewScore(cmd.Score); err != nil {
		return nil, errs.Wrap(errs.CodeValidation, "评分必须为 1 至 5 的整数", err)
	}
	if err := domain.ValidateTradeReviewContent(cmd.Content); err != nil {
		return nil, errs.Wrap(errs.CodeValidation, "评价文字长度必须为 0 至 500 个字符", err)
	}

	trade, err := s.trades.ByID(ctx, cmd.TradeID)
	if err != nil {
		return nil, s.notFound(err)
	}
	if !trade.IsParty(cmd.ActorID) {
		return nil, errs.New(errs.CodeResourceNotFound, "交易不存在")
	}
	if !trade.IsBuyer(cmd.ActorID) {
		return nil, errs.New(errs.CodeForbidden, "只有买家可以评价该交易")
	}
	if trade.Status != domain.TradeCompleted {
		return nil, errs.New(errs.CodeForbidden, "交易完成后才能发布评价")
	}

	review, err := domain.NewTradeReview(s.ids.NewTradeReviewID(), trade, cmd.Score, cmd.Content, s.clock.Now())
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "评价初始化失败", err)
	}
	if err := s.reviews.Insert(ctx, review); err != nil {
		return nil, insertTradeReviewError(err)
	}
	return review, nil
}

// ByProductIDs 一次读取多个商品各自的买家评价（每个商品最多一条）。
// 评价随已售出商品详情和卖家自己的商品列表公开。
func (s *TradeReviewService) ByProductIDs(ctx context.Context, productIDs []string) (map[string]*domain.TradeReview, error) {
	reviews, err := s.reviews.ByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return reviews, nil
}

// AverageScoresByUserIDs 一次计算多个用户作为卖家收到的评分平均值。
// 平均值是展示投影，公开于卖家信息和本人资料中。
func (s *TradeReviewService) AverageScoresByUserIDs(ctx context.Context, userIDs []string) (map[string]string, error) {
	scores, err := s.reviews.AverageScoresByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return scores, nil
}

// notFound 将仓储未找到结果映射为 RESOURCE_NOT_FOUND。
func (s *TradeReviewService) notFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, "交易不存在", err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

// insertTradeReviewError 映射评价写入失败。唯一冲突是预期并发结果。
func insertTradeReviewError(err error) error {
	switch {
	case errors.Is(err, ErrTradeReviewAlreadyExists):
		return errs.Wrap(errs.CodeTradeReviewExists, "该交易已经收到买家评价", err)
	case errors.Is(err, ErrNotFound):
		return errs.Wrap(errs.CodeResourceNotFound, "交易不存在", err)
	default:
		return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
}
