// Package application holds the Marketplace use cases: product publishing and
// browsing, images, and the trade state machine.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// Storage outcomes. They describe repository results, not HTTP or gRPC ones.
var (
	// ErrNotFound reports that no row matched.
	ErrNotFound = errors.New("not found")
	// ErrVersionConflict reports that the row changed since it was read.
	ErrVersionConflict = errors.New("product changed concurrently")
	// ErrImageSlotTaken reports that another writer took the image slot first.
	ErrImageSlotTaken = errors.New("image slot already used")
)

// ProductFilter selects products for the public list.
type ProductFilter struct {
	// Keyword matches a substring of the title. Nil means no keyword filter.
	Keyword *string
	// Category restricts the category. Nil means every category.
	Category *domain.Category
	// Status restricts the status. Nil means every status; the public list
	// always sets it to ON_SALE.
	Status *domain.Status
	// SellerID restricts to one seller. Empty means every seller.
	SellerID string
}

// Page describes a page request. The repository applies a deterministic order
// with the identifier as the tie-breaker, so paging cannot skip or repeat rows
// that share a timestamp.
type Page struct {
	Number int32
	Size   int32
}

// ProductPage is one page of results plus the total row count.
type ProductPage struct {
	Items []*domain.Product
	Page  int32
	Size  int32
	Total int64
}

// ProductRepository stores products and their images.
type ProductRepository interface {
	Create(ctx context.Context, product *domain.Product) error
	// ByID loads one product with its images.
	ByID(ctx context.Context, id string) (*domain.Product, error)
	// ByIDs loads several products with their images in one round trip.
	ByIDs(ctx context.Context, ids []string) ([]*domain.Product, error)
	List(ctx context.Context, filter ProductFilter, page Page) (ProductPage, error)
	// Update writes the product back, failing with ErrVersionConflict when the
	// stored version no longer matches the one that was read.
	Update(ctx context.Context, product *domain.Product, expectedVersion int64) error
	// AddImage inserts one image, failing with ErrImageSlotTaken when another
	// writer already used the slot.
	AddImage(ctx context.Context, image *domain.Image) error
	// DeleteImage removes one image and reports the object key that is now
	// unreferenced, so the caller can delete the stored object.
	DeleteImage(ctx context.Context, productID, imageID string) (string, error)
}

// ObjectStore holds product images.
//
// The interface is deliberately small so the deployed implementation can be
// Alibaba Cloud OSS (docs/software-design.md section 9.2) while local
// development and tests use the filesystem. No caller knows which is in use.
type ObjectStore interface {
	// Put stores data under key. Implementations must overwrite an existing
	// object so a retried upload is idempotent.
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Delete removes an object. Deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
	// URL returns the publicly reachable location of an object.
	URL(key string) string
}

// IDGenerator mints opaque identifiers for the Marketplace aggregates.
type IDGenerator interface {
	NewProductID() string
	NewImageID() string
	NewTradeID() string
	NewEventID() string
}

// Clock reads the current time.
type Clock interface {
	Now() time.Time
}
