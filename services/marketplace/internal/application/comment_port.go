package application

import (
	"context"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// CommentPage 是一页评论及总行数。
type CommentPage struct {
	Items []*domain.ProductComment
	Page  int32
	Size  int32
	Total int64
}

// CommentRepository 存储商品用户评论。
//
// 评论不改变商品或交易状态，因此不需要事务接口：插入是单条语句，商品存在性
// 由外键约束兜底（商品没有任何删除路径），不存在与商品或交易的锁交互。
type CommentRepository interface {
	// Insert 写入新评论。商品不存在必须返回 ErrNotFound。
	Insert(ctx context.Context, comment *domain.ProductComment) error
	// ProductExists 报告商品是否存在。
	ProductExists(ctx context.Context, productID string) (bool, error)
	// ListProductComments 按确定性顺序返回一页商品评论。
	ListProductComments(ctx context.Context, productID string, page Page) (CommentPage, error)
}
