package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// reviewBuyerUniqueConstraint 是评论的唯一约束。它的名称属于行为契约：
// 代码会将该冲突转换为 ErrReviewAlreadyExists。
const reviewBuyerUniqueConstraint = "product_reviews_buyer_unique"

// reviewColumns 是所有评论读取共享的字段投影。
const reviewColumns = `id, product_id, trade_id, buyer_id, comment, created_at`

// ReviewRepository 存储买家评论。
type ReviewRepository struct {
	pool *pgxpool.Pool
}

// NewReviewRepository 基于已打开的连接池构造仓储。
func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{pool: pool}
}

var _ application.ReviewRepository = (*ReviewRepository)(nil)

// Execute 在单个事务中运行评论命令。
func (r *ReviewRepository) Execute(ctx context.Context, fn func(ctx context.Context, tx application.ReviewTx) error) error {
	return pg.InTx(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return fn(ctx, &reviewTx{tx: tx})
	})
}

// ProductExists 在事务外报告商品是否存在。
func (r *ReviewRepository) ProductExists(ctx context.Context, productID string) (bool, error) {
	exists, err := productExists(ctx, r.pool, productID)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ListProductReviews 返回商品的一页评论，按最新优先，并以标识作为确定性的平局裁决。
func (r *ReviewRepository) ListProductReviews(ctx context.Context, productID string, page application.Page) (application.ReviewPage, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM product_reviews WHERE product_id = $1`, productID,
	).Scan(&total); err != nil {
		return application.ReviewPage{}, fmt.Errorf("postgres: count reviews: %w", err)
	}

	const query = `SELECT ` + reviewColumns + ` FROM product_reviews WHERE product_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, productID, page.Size, page.Offset())
	if err != nil {
		return application.ReviewPage{}, fmt.Errorf("postgres: select reviews: %w", err)
	}
	defer rows.Close()

	var reviews []*domain.Review
	for rows.Next() {
		review, err := scanReview(rows)
		if err != nil {
			return application.ReviewPage{}, fmt.Errorf("postgres: scan review: %w", err)
		}
		reviews = append(reviews, review)
	}
	if err := rows.Err(); err != nil {
		return application.ReviewPage{}, fmt.Errorf("postgres: iterate reviews: %w", err)
	}

	return application.ReviewPage{Items: reviews, Page: page.Number, Size: page.Size, Total: total}, nil
}

// reviewTx 是事务内的评论访问实现。
type reviewTx struct {
	tx pgx.Tx
}

// ProductExists 报告商品是否存在。评论创建不写入商品行，因此读取无需加锁：
// 商品没有任何删除路径，事务内读取的结果不会失效。
func (t *reviewTx) ProductExists(ctx context.Context, productID string) (bool, error) {
	return productExists(ctx, t.tx, productID)
}

// CompletedTradeForBuyer 返回买家对该商品终生唯一交易中已完成的那个。
// 交易处于 PENDING、ACCEPTED、CANCELLED 或不存在时都返回 ErrNotFound：
// 对评论而言它们是同一个事实——买家还没有可评论的已完成交易。
func (t *reviewTx) CompletedTradeForBuyer(ctx context.Context, productID, buyerID string) (*domain.Trade, error) {
	const query = `SELECT ` + tradeColumns + ` FROM trades
		WHERE product_id = $1 AND buyer_id = $2 AND status = 'COMPLETED'`

	trade, err := scanTrade(t.tx.QueryRow(ctx, query, productID, buyerID))
	if pg.IsNoRows(err) {
		return nil, application.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: select completed trade: %w", err)
	}
	return trade, nil
}

// InsertReview 写入新评论。
func (t *reviewTx) InsertReview(ctx context.Context, review *domain.Review) error {
	const query = `
		INSERT INTO product_reviews (id, product_id, trade_id, buyer_id, comment, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := t.tx.Exec(ctx, query,
		review.ID, review.ProductID, review.TradeID, review.BuyerID, review.Comment, review.CreatedAt,
	)
	switch {
	case pg.IsUniqueViolation(err, reviewBuyerUniqueConstraint):
		return application.ErrReviewAlreadyExists
	case isForeignKeyViolation(err):
		return application.ErrNotFound
	case err != nil:
		return fmt.Errorf("postgres: insert review: %w", err)
	}
	return nil
}

// querier 是连接池与事务共享的最小查询接口。
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// productExists 是事务内外的商品存在性共享查询。
func productExists(ctx context.Context, db querier, productID string) (bool, error) {
	var exists bool
	if err := db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM products WHERE id = $1)`, productID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres: check product existence: %w", err)
	}
	return exists, nil
}

// scanReview 将一行评论投影为领域对象。
func scanReview(row pgx.Row) (*domain.Review, error) {
	var review domain.Review
	err := row.Scan(&review.ID, &review.ProductID, &review.TradeID, &review.BuyerID, &review.Comment, &review.CreatedAt)
	if pg.IsNoRows(err) {
		return nil, application.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &review, nil
}
