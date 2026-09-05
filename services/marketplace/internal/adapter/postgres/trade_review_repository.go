package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// tradeReviewUniqueConstraint 是买家评价的唯一约束。它的名称属于行为契约：
// 代码会将该冲突转换为 ErrTradeReviewAlreadyExists。
const tradeReviewUniqueConstraint = "trade_reviews_trade_unique"

// tradeReviewColumns 是所有买家评价读取共享的字段投影。
const tradeReviewColumns = `id, trade_id, product_id, buyer_id, content, score, created_at`

// TradeReviewRepository 存储买家评价。
type TradeReviewRepository struct {
	pool *pgxpool.Pool
}

// NewTradeReviewRepository 基于已打开的连接池构造仓储。
func NewTradeReviewRepository(pool *pgxpool.Pool) *TradeReviewRepository {
	return &TradeReviewRepository{pool: pool}
}

var _ application.TradeReviewRepository = (*TradeReviewRepository)(nil)

// Insert 写入新评价。评价是单条不可变插入：交易存在性与唯一性由外键和唯一
// 约束兜底，COMPLETED 是交易终态，因此不需要事务包裹。
func (r *TradeReviewRepository) Insert(ctx context.Context, review *domain.TradeReview) error {
	const query = `
		INSERT INTO trade_reviews (id, trade_id, product_id, buyer_id, content, score, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	if _, err := r.pool.Exec(ctx, query,
		review.ID, review.TradeID, review.ProductID, review.BuyerID, review.Content, review.Score, review.CreatedAt,
	); err != nil {
		switch {
		case pg.IsUniqueViolation(err, tradeReviewUniqueConstraint):
			return application.ErrTradeReviewAlreadyExists
		case isForeignKeyViolation(err):
			return application.ErrNotFound
		default:
			return fmt.Errorf("postgres: insert trade review: %w", err)
		}
	}
	return nil
}

// ByProductIDs 一次读取多个商品各自的买家评价。缺失的商品不会出现在结果中。
func (r *TradeReviewRepository) ByProductIDs(ctx context.Context, productIDs []string) (map[string]*domain.TradeReview, error) {
	if len(productIDs) == 0 {
		return map[string]*domain.TradeReview{}, nil
	}

	const query = `SELECT ` + tradeReviewColumns + ` FROM trade_reviews WHERE product_id = ANY($1)`
	rows, err := r.pool.Query(ctx, query, productIDs)
	if err != nil {
		return nil, fmt.Errorf("postgres: select trade reviews: %w", err)
	}
	defer rows.Close()

	reviews := make(map[string]*domain.TradeReview, len(productIDs))
	for rows.Next() {
		var review domain.TradeReview
		if err := rows.Scan(&review.ID, &review.TradeID, &review.ProductID, &review.BuyerID, &review.Content, &review.Score, &review.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan trade review: %w", err)
		}
		reviews[review.ProductID] = &review
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate trade reviews: %w", err)
	}
	return reviews, nil
}

// AverageScoresByUserIDs 计算用户作为卖家收到的评分平均值。
// 平均值由数据库聚合得出，固定两位小数；没有评分的用户不出现在结果中。
func (r *TradeReviewRepository) AverageScoresByUserIDs(ctx context.Context, userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return map[string]string{}, nil
	}

	const query = `
		SELECT products.seller_id, ROUND(AVG(trade_reviews.score), 2)::text
		  FROM trade_reviews
		  JOIN products ON products.id = trade_reviews.product_id
		 WHERE trade_reviews.score IS NOT NULL
		   AND products.seller_id = ANY($1)
		 GROUP BY products.seller_id`

	rows, err := r.pool.Query(ctx, query, userIDs)
	if err != nil {
		return nil, fmt.Errorf("postgres: select average scores: %w", err)
	}
	defer rows.Close()

	scores := make(map[string]string, len(userIDs))
	for rows.Next() {
		var sellerID, average string
		if err := rows.Scan(&sellerID, &average); err != nil {
			return nil, fmt.Errorf("postgres: scan average score: %w", err)
		}
		scores[sellerID] = average
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate average scores: %w", err)
	}
	return scores, nil
}
