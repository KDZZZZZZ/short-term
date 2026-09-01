// Package domain defines the Favorite aggregate and its local invariants.
package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

const maxIdentifierLength = 64

var (
	// ErrInvalidUserID indicates that a favorite has no valid owner.
	ErrInvalidUserID = errors.New("favorite user id must contain 1 to 64 characters")
	// ErrInvalidProductID indicates that the referenced product id is malformed.
	ErrInvalidProductID = errors.New("favorite product id must contain 1 to 64 characters")
	// ErrInvalidTime indicates that the relationship timestamp is absent.
	ErrInvalidTime = errors.New("favorite time is required")
)

// Favorite is the relationship owned by Favorite Service. Product facts such
// as title, seller and status deliberately remain in Marketplace Service.
type Favorite struct {
	UserID      string
	ProductID   string
	FavoritedAt time.Time
}

// New creates a favorite relationship after validating its local fields.
func New(userID, productID string, favoritedAt time.Time) (Favorite, error) {
	if !validIdentifier(userID) {
		return Favorite{}, ErrInvalidUserID
	}
	if !validIdentifier(productID) {
		return Favorite{}, ErrInvalidProductID
	}
	if favoritedAt.IsZero() {
		return Favorite{}, ErrInvalidTime
	}
	return Favorite{UserID: userID, ProductID: productID, FavoritedAt: favoritedAt.UTC()}, nil
}

// ValidateProductID validates a product reference before an external lookup.
func ValidateProductID(productID string) error {
	if !validIdentifier(productID) {
		return ErrInvalidProductID
	}
	return nil
}

func validIdentifier(value string) bool {
	length := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && length >= 1 && length <= maxIdentifierLength
}
