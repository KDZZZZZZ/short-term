package application

import "github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"

// Pagination limits, matching openapi/components/parameters.yaml.
const (
	// DefaultPageSize is the page_size default.
	DefaultPageSize int32 = 20
	// MaxPageSize is the page_size maximum.
	MaxPageSize int32 = 100
)

// CreateProductCommand publishes a listing.
type CreateProductCommand struct {
	ActorID     string
	Title       string
	PriceMinor  int64
	Category    domain.Category
	Description string
	Images      []ImageUpload
}

// UpdateProductCommand edits a listing. A nil field is left unchanged.
type UpdateProductCommand struct {
	ActorID     string
	ProductID   string
	Title       *string
	PriceMinor  *int64
	Category    *domain.Category
	Description *string
}

// ListProductsQuery browses the public catalogue.
type ListProductsQuery struct {
	Keyword  *string
	Category *domain.Category
	Page     Page
}

// ListUserProductsQuery lists one seller's own listings.
type ListUserProductsQuery struct {
	SellerID string
	Status   *domain.Status
	Page     Page
}

// AddImagesCommand appends images to a listing.
type AddImagesCommand struct {
	ActorID   string
	ProductID string
	Images    []ImageUpload
}

// DeleteImageCommand removes one image from a listing.
type DeleteImageCommand struct {
	ActorID   string
	ProductID string
	ImageID   string
}

// ProductActionCommand is a seller action that only needs the product.
type ProductActionCommand struct {
	ActorID   string
	ProductID string
}

// normalize clamps a page request to the range the public contract allows, so
// a caller that omits the values still gets the documented defaults.
func (p Page) normalize() Page {
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

// Offset is the number of rows to skip for this page.
func (p Page) Offset() int32 { return (p.Number - 1) * p.Size }
