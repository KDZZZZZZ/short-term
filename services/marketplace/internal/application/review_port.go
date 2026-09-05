package application

import (
	"context"
	"errors"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// ErrReviewAlreadyExists 表示同一买家已经评论过该商品，并被数据库唯一约束拒绝。
var ErrReviewAlreadyExists = errors.New("the buyer already reviewed this product")

// ReviewTx 是评论命令执行所使用的事务接口。
//
// 评论既不修改商品也不修改交易，因此不参与 Product -> Trade 锁序：
// 事务只读取商品存在性与已完成交易，再插入评论行。
type ReviewTx interface {
	// ProductExists 报告商品是否存在。商品只有下架没有删除，普通读取即可，
	// 不需要行锁。
	ProductExists(ctx context.Context, productID string) (bool, error)
	// CompletedTradeForBuyer 返回买家对该商品终生唯一交易中已完成的那个。
	// 没有交易或交易尚未完成都必须返回 ErrNotFound，由应用层映射为契约错误。
	CompletedTradeForBuyer(ctx context.Context, productID, buyerID string) (*domain.Trade, error)
	// InsertReview 写入新评论。唯一约束冲突必须返回 ErrReviewAlreadyExists。
	InsertReview(ctx context.Context, review *domain.Review) error
}

// ReviewPage 是一页评论及总行数。
type ReviewPage struct {
	Items []*domain.Review
	Page  int32
	Size  int32
	Total int64
}

// ReviewRepository 存储买家评论。
type ReviewRepository interface {
	// Execute 在单个事务中运行评论命令。
	Execute(ctx context.Context, fn func(ctx context.Context, tx ReviewTx) error) error
	// ProductExists 在事务外报告商品是否存在。
	ProductExists(ctx context.Context, productID string) (bool, error)
	// ListProductReviews 按确定性顺序返回一页商品评论。
	ListProductReviews(ctx context.Context, productID string, page Page) (ReviewPage, error)
}
