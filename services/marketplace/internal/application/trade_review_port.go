package application

import (
	"context"
	"errors"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// ErrTradeReviewAlreadyExists 表示该交易已经收到买家评价，并被数据库唯一
// 约束拒绝。
var ErrTradeReviewAlreadyExists = errors.New("the trade already has a buyer review")

// TradeReviewRepository 存储买家评价。
//
// 评价不改变商品或交易状态：插入是单条不可变语句，COMPLETED 是交易终态，
// 因此不需要事务接口，也不与商品/交易发生锁交互。
type TradeReviewRepository interface {
	// Insert 写入新评价。交易不存在必须返回 ErrNotFound；
	// 交易已收到评价必须返回 ErrTradeReviewAlreadyExists。
	Insert(ctx context.Context, review *domain.TradeReview) error
	// ByProductIDs 一次读取多个商品的买家评价（每个商品最多一条）。
	ByProductIDs(ctx context.Context, productIDs []string) (map[string]*domain.TradeReview, error)
	// AverageScoresByUserIDs 一次计算多个用户作为卖家收到的评分平均值，
	// 格式为固定两位小数的字符串；没有评分的用户不出现在结果中。
	AverageScoresByUserIDs(ctx context.Context, userIDs []string) (map[string]string, error)
}
