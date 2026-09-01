// Package postgres implements Favorite Service persistence in its own database.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/services/favorite/internal/application"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/domain"
)

// Repository stores favorite relationships in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repository over an opened Favorite database pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ application.Repository = (*Repository)(nil)

// Add is idempotent. ON CONFLICT deliberately performs no UPDATE so a repeated
// PUT cannot move the original favorited_at timestamp and reorder the list.
func (r *Repository) Add(ctx context.Context, favorite domain.Favorite) error {
	const query = `
		INSERT INTO favorites (user_id, product_id, favorited_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, product_id) DO NOTHING`

	if _, err := r.pool.Exec(ctx, query, favorite.UserID, favorite.ProductID, favorite.FavoritedAt); err != nil {
		return fmt.Errorf("postgres: add favorite: %w", err)
	}
	return nil
}

// Remove is idempotent: deleting an absent relationship succeeds.
func (r *Repository) Remove(ctx context.Context, userID, productID string) error {
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM favorites WHERE user_id = $1 AND product_id = $2`, userID, productID,
	); err != nil {
		return fmt.Errorf("postgres: remove favorite: %w", err)
	}
	return nil
}

// List returns a page ordered by favorited_at DESC, product_id DESC. Page
// pagination is intentionally not a cross-request snapshot, as documented.
func (r *Repository) List(ctx context.Context, userID string, page application.Page) (application.FavoritePage, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM favorites WHERE user_id = $1`, userID,
	).Scan(&total); err != nil {
		return application.FavoritePage{}, fmt.Errorf("postgres: count favorites: %w", err)
	}

	const query = `
		SELECT user_id, product_id, favorited_at
		  FROM favorites
		 WHERE user_id = $1
		 ORDER BY favorited_at DESC, product_id DESC
		 LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, userID, page.Size, page.Offset())
	if err != nil {
		return application.FavoritePage{}, fmt.Errorf("postgres: list favorites: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Favorite, 0, page.Size)
	for rows.Next() {
		var item domain.Favorite
		if err := rows.Scan(&item.UserID, &item.ProductID, &item.FavoritedAt); err != nil {
			return application.FavoritePage{}, fmt.Errorf("postgres: scan favorite: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return application.FavoritePage{}, fmt.Errorf("postgres: read favorites: %w", err)
	}

	return application.FavoritePage{
		Items: items,
		Page:  page.Number,
		Size:  page.Size,
		Total: total,
	}, nil
}

// IsFavorited checks the composite key without loading list data.
func (r *Repository) IsFavorited(ctx context.Context, userID, productID string) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM favorites WHERE user_id = $1 AND product_id = $2)`,
		userID, productID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres: check favorite: %w", err)
	}
	return exists, nil
}
