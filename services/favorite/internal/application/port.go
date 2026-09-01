// Package application implements Favorite Service use cases without depending
// on transport or persistence details.
package application

import (
	"context"
	"time"

	"github.com/KDZZZZZZ/short-term/services/favorite/internal/domain"
)

const (
	// DefaultPageSize matches the public OpenAPI pagination default.
	DefaultPageSize int32 = 20
	// MaxPageSize matches the public OpenAPI pagination maximum.
	MaxPageSize int32 = 100
)

// Repository owns only favorite relationships.
type Repository interface {
	Add(ctx context.Context, favorite domain.Favorite) error
	Remove(ctx context.Context, userID, productID string) error
	List(ctx context.Context, userID string, page Page) (FavoritePage, error)
	IsFavorited(ctx context.Context, userID, productID string) (bool, error)
}

// ProductCatalog reads the minimum current product fact needed when adding a
// favorite. No product fact is copied into Favorite storage.
type ProductCatalog interface {
	Get(ctx context.Context, actorID, productID string) (Product, error)
}

// Clock supplies persisted timestamps.
type Clock interface {
	Now() time.Time
}

// Product is the minimal Marketplace projection used to reject self-favorites.
type Product struct {
	ID       string
	SellerID string
}

// Page describes a non-snapshot page request.
type Page struct {
	Number int32
	Size   int32
}

// Normalize applies internal defaults. Gateway rejects explicitly invalid REST
// query values before constructing this request.
func (p Page) Normalize() Page {
	if p.Number < 1 {
		p.Number = 1
	}
	if p.Size < 1 {
		p.Size = DefaultPageSize
	}
	if p.Size > MaxPageSize {
		p.Size = MaxPageSize
	}
	return p
}

// Offset returns the number of rows preceding this page.
func (p Page) Offset() int32 { return (p.Number - 1) * p.Size }

// FavoritePage is a deterministically ordered page of relationships.
type FavoritePage struct {
	Items []domain.Favorite
	Page  int32
	Size  int32
	Total int64
}

// FavoriteCommand identifies a relationship mutation.
type FavoriteCommand struct {
	ActorID   string
	ProductID string
}

// ListFavoritesQuery identifies a user's requested page.
type ListFavoritesQuery struct {
	ActorID string
	Page    Page
}
