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

// commentColumns 是所有评论读取共享的字段投影。
const commentColumns = `id, product_id, user_id, content, created_at`

// CommentRepository 存储商品用户评论。
type CommentRepository struct {
	pool *pgxpool.Pool
}

// NewCommentRepository 基于已打开的连接池构造仓储。
func NewCommentRepository(pool *pgxpool.Pool) *CommentRepository {
	return &CommentRepository{pool: pool}
}

var _ application.CommentRepository = (*CommentRepository)(nil)

// Insert 写入新评论。评论是单条不可变插入：商品存在性由外键约束兜底，
// 商品没有任何删除路径，因此不需要事务包裹。
func (r *CommentRepository) Insert(ctx context.Context, comment *domain.ProductComment) error {
	const query = `
		INSERT INTO product_comments (id, product_id, user_id, content, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	if _, err := r.pool.Exec(ctx, query,
		comment.ID, comment.ProductID, comment.UserID, comment.Content, comment.CreatedAt,
	); err != nil {
		if isForeignKeyViolation(err) {
			return application.ErrNotFound
		}
		return fmt.Errorf("postgres: insert comment: %w", err)
	}
	return nil
}

// ProductExists 报告商品是否存在。
func (r *CommentRepository) ProductExists(ctx context.Context, productID string) (bool, error) {
	return productExists(ctx, r.pool, productID)
}

// ListProductComments 返回商品的一页评论，按最新优先，并以标识作为确定性的平局裁决。
func (r *CommentRepository) ListProductComments(ctx context.Context, productID string, page application.Page) (application.CommentPage, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM product_comments WHERE product_id = $1`, productID,
	).Scan(&total); err != nil {
		return application.CommentPage{}, fmt.Errorf("postgres: count comments: %w", err)
	}

	const query = `SELECT ` + commentColumns + ` FROM product_comments WHERE product_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, productID, page.Size, page.Offset())
	if err != nil {
		return application.CommentPage{}, fmt.Errorf("postgres: select comments: %w", err)
	}
	defer rows.Close()

	var comments []*domain.ProductComment
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return application.CommentPage{}, fmt.Errorf("postgres: scan comment: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return application.CommentPage{}, fmt.Errorf("postgres: iterate comments: %w", err)
	}

	return application.CommentPage{Items: comments, Page: page.Number, Size: page.Size, Total: total}, nil
}

// querier 是连接池与事务共享的最小查询接口。
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// productExists 是商品存在性的共享查询。
func productExists(ctx context.Context, db querier, productID string) (bool, error) {
	var exists bool
	if err := db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM products WHERE id = $1)`, productID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres: check product existence: %w", err)
	}
	return exists, nil
}

// scanComment 将一行评论投影为领域对象。
func scanComment(row pgx.Row) (*domain.ProductComment, error) {
	var comment domain.ProductComment
	err := row.Scan(&comment.ID, &comment.ProductID, &comment.UserID, &comment.Content, &comment.CreatedAt)
	if pg.IsNoRows(err) {
		return nil, application.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &comment, nil
}
