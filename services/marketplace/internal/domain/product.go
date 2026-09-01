// Package domain holds the Marketplace entities and the rules that govern
// them. Product status may only change through the transitions declared here,
// which is what docs/state-machines.md requires: no generic setter, and no
// PATCH endpoint, may assign a status directly.
package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// Status is the product lifecycle state.
type Status string

// The product states from docs/state-machines.md.
const (
	StatusOnSale   Status = "ON_SALE"
	StatusReserved Status = "RESERVED"
	StatusSold     Status = "SOLD"
	StatusOffShelf Status = "OFF_SHELF"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusOnSale, StatusReserved, StatusSold, StatusOffShelf:
		return true
	default:
		return false
	}
}

// Category is the product category.
type Category string

// The categories declared by the public ProductCategory schema.
const (
	CategoryTextbook Category = "TEXTBOOK"
	CategoryDigital  Category = "DIGITAL"
	CategoryLife     Category = "LIFE"
	CategoryOther    Category = "OTHER"
)

// Valid reports whether c is a known category.
func (c Category) Valid() bool {
	switch c {
	case CategoryTextbook, CategoryDigital, CategoryLife, CategoryOther:
		return true
	default:
		return false
	}
}

// Field limits, mirroring openapi/components/schemas.yaml.
const (
	// MaxTitleLength is the ProductSummary/ProductDetail title limit.
	MaxTitleLength = 100
	// MaxDescriptionLength is the ProductDetail description limit.
	MaxDescriptionLength = 2000
	// MaxPriceMinor is the largest amount the Price pattern can express:
	// 99999999.99 in minor units.
	MaxPriceMinor = 9999999999
	// MaxImages is the number of images a product may carry.
	MaxImages = 3
)

// Validation and transition errors.
var (
	ErrIDRequired         = errors.New("product id is required")
	ErrSellerRequired     = errors.New("seller id is required")
	ErrTitleLength        = errors.New("title must be 1-100 characters")
	ErrDescriptionLength  = errors.New("description must be 1-2000 characters")
	ErrPriceRange         = errors.New("price must be between 0 and 99999999.99")
	ErrCategoryUnknown    = errors.New("category is not one of TEXTBOOK, DIGITAL, LIFE, OTHER")
	ErrStatusUnknown      = errors.New("status is not a known product status")
	ErrNotSeller          = errors.New("only the seller may perform this action")
	ErrNotEditable        = errors.New("a product may only be edited while it is on sale or off shelf")
	ErrNotOnSale          = errors.New("the product is not on sale")
	ErrNotOffShelf        = errors.New("the product is not off shelf")
	ErrNotReserved        = errors.New("the product is not reserved")
	ErrImageLimitExceeded = errors.New("a product may hold at most three images")
	ErrImageSlotTaken     = errors.New("the image slot is already used")
)

// Image is a stored product photo. The object key is internal; the public URL
// is derived by the transport layer, so moving to another object store does
// not change the domain.
type Image struct {
	ID        string
	ProductID string
	ObjectKey string
	SortOrder int
	CreatedAt time.Time
}

// Product is the aggregate root for a listing.
type Product struct {
	ID          string
	SellerID    string
	Title       string
	PriceMinor  int64
	Category    Category
	Description string
	Status      Status
	// Version increases on every change and is used for optimistic concurrency
	// detection on the product row.
	Version   int64
	Images    []Image
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewProduct creates a listing in the ON_SALE state.
func NewProduct(id, sellerID, title string, priceMinor int64, category Category, description string, now time.Time) (*Product, error) {
	if id == "" {
		return nil, ErrIDRequired
	}
	if sellerID == "" {
		return nil, ErrSellerRequired
	}
	if err := ValidateTitle(title); err != nil {
		return nil, err
	}
	if err := ValidatePrice(priceMinor); err != nil {
		return nil, err
	}
	if err := ValidateDescription(description); err != nil {
		return nil, err
	}
	if !category.Valid() {
		return nil, ErrCategoryUnknown
	}

	return &Product{
		ID:          id,
		SellerID:    sellerID,
		Title:       title,
		PriceMinor:  priceMinor,
		Category:    category,
		Description: description,
		Status:      StatusOnSale,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// ValidateTitle enforces the public title limits.
func ValidateTitle(title string) error {
	length := utf8.RuneCountInString(title)
	if length < 1 || length > MaxTitleLength {
		return ErrTitleLength
	}
	return nil
}

// ValidateDescription enforces the public description limits.
func ValidateDescription(description string) error {
	length := utf8.RuneCountInString(description)
	if length < 1 || length > MaxDescriptionLength {
		return ErrDescriptionLength
	}
	return nil
}

// ValidatePrice enforces the range the public Price pattern can express.
func ValidatePrice(priceMinor int64) error {
	if priceMinor < 0 || priceMinor > MaxPriceMinor {
		return ErrPriceRange
	}
	return nil
}

// IsSeller reports whether actorID published this product.
func (p *Product) IsSeller(actorID string) bool { return actorID != "" && actorID == p.SellerID }

// Edit changes the descriptive fields. Only fields with a non-nil argument
// change, and status is never one of them: a listing that is reserved or sold
// belongs to a trade in progress and must not move under the counterparty.
func (p *Product) Edit(actorID string, title *string, priceMinor *int64, category *Category, description *string, now time.Time) error {
	if !p.IsSeller(actorID) {
		return ErrNotSeller
	}
	if p.Status != StatusOnSale && p.Status != StatusOffShelf {
		return ErrNotEditable
	}

	if title != nil {
		if err := ValidateTitle(*title); err != nil {
			return err
		}
	}
	if priceMinor != nil {
		if err := ValidatePrice(*priceMinor); err != nil {
			return err
		}
	}
	if description != nil {
		if err := ValidateDescription(*description); err != nil {
			return err
		}
	}
	if category != nil && !category.Valid() {
		return ErrCategoryUnknown
	}

	if title != nil {
		p.Title = *title
	}
	if priceMinor != nil {
		p.PriceMinor = *priceMinor
	}
	if category != nil {
		p.Category = *category
	}
	if description != nil {
		p.Description = *description
	}
	p.touch(now)
	return nil
}

// OffShelf performs ON_SALE -> OFF_SHELF.
func (p *Product) OffShelf(actorID string, now time.Time) error {
	if !p.IsSeller(actorID) {
		return ErrNotSeller
	}
	if p.Status != StatusOnSale {
		return ErrNotOnSale
	}
	p.Status = StatusOffShelf
	p.touch(now)
	return nil
}

// Relist performs OFF_SHELF -> ON_SALE.
func (p *Product) Relist(actorID string, now time.Time) error {
	if !p.IsSeller(actorID) {
		return ErrNotSeller
	}
	if p.Status != StatusOffShelf {
		return ErrNotOffShelf
	}
	p.Status = StatusOnSale
	p.touch(now)
	return nil
}

// Reserve performs ON_SALE -> RESERVED when a seller accepts a trade. It is
// driven by the trade state machine, never by a product endpoint.
func (p *Product) Reserve(now time.Time) error {
	if p.Status != StatusOnSale {
		return ErrNotOnSale
	}
	p.Status = StatusReserved
	p.touch(now)
	return nil
}

// Release performs RESERVED -> ON_SALE when an accepted trade is cancelled.
func (p *Product) Release(now time.Time) error {
	if p.Status != StatusReserved {
		return ErrNotReserved
	}
	p.Status = StatusOnSale
	p.touch(now)
	return nil
}

// MarkSold performs RESERVED -> SOLD when both parties confirm. SOLD is final.
func (p *Product) MarkSold(now time.Time) error {
	if p.Status != StatusReserved {
		return ErrNotReserved
	}
	p.Status = StatusSold
	p.touch(now)
	return nil
}

// Tradable reports whether a new trade may be created or accepted for this
// product. RESERVED, SOLD and OFF_SHELF products are not tradable.
func (p *Product) Tradable() bool { return p.Status == StatusOnSale }

// NextImageSlot returns the lowest free sort_order, or an error when the
// product already holds the maximum number of images.
func (p *Product) NextImageSlot() (int, error) {
	used := make(map[int]bool, len(p.Images))
	for _, image := range p.Images {
		used[image.SortOrder] = true
	}
	for slot := 1; slot <= MaxImages; slot++ {
		if !used[slot] {
			return slot, nil
		}
	}
	return 0, ErrImageLimitExceeded
}

// CoverObjectKey returns the object key of the lowest-ordered image, which is
// the product cover, or an empty string when the product has no images.
func (p *Product) CoverObjectKey() string {
	cover := ""
	lowest := MaxImages + 1
	for _, image := range p.Images {
		if image.SortOrder < lowest {
			lowest = image.SortOrder
			cover = image.ObjectKey
		}
	}
	return cover
}

// touch records a change: the timestamp moves and the version increments, so a
// concurrent writer that read an older row can be detected.
func (p *Product) touch(now time.Time) {
	p.Version++
	p.UpdatedAt = now
}
