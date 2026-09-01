package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KDZZZZZZ/short-term/services/favorite/internal/domain"
)

func TestNewFavorite(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	favorite, err := domain.New("u_1", "p_1", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if favorite.UserID != "u_1" || favorite.ProductID != "p_1" {
		t.Fatalf("favorite = %+v", favorite)
	}
	if favorite.FavoritedAt.Location() != time.UTC || !favorite.FavoritedAt.Equal(now) {
		t.Fatalf("FavoritedAt = %s, want UTC representation of %s", favorite.FavoritedAt, now)
	}
}

func TestNewFavoriteRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name      string
		userID    string
		productID string
		at        time.Time
		want      error
	}{
		{name: "missing user", productID: "p_1", at: now, want: domain.ErrInvalidUserID},
		{name: "long user", userID: strings.Repeat("u", 65), productID: "p_1", at: now, want: domain.ErrInvalidUserID},
		{name: "missing product", userID: "u_1", at: now, want: domain.ErrInvalidProductID},
		{name: "long product", userID: "u_1", productID: strings.Repeat("p", 65), at: now, want: domain.ErrInvalidProductID},
		{name: "missing time", userID: "u_1", productID: "p_1", want: domain.ErrInvalidTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.New(tt.userID, tt.productID, tt.at)
			if !errors.Is(err, tt.want) {
				t.Fatalf("New = %v, want %v", err, tt.want)
			}
		})
	}
}
