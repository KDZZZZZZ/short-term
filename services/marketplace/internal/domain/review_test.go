package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewReviewAcceptsValidComment(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	review, err := NewReview("rv_1", "p_1", "u_buyer", "很不错的卖家", created)
	if err != nil {
		t.Fatalf("NewReview: %v", err)
	}
	if review.ID != "rv_1" || review.ProductID != "p_1" || review.BuyerID != "u_buyer" {
		t.Fatalf("unexpected review: %+v", review)
	}
	if review.TradeID != "" {
		t.Fatal("trade binding belongs to the application transaction, not the constructor")
	}
}

func TestNewReviewRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		buyerID string
		comment string
		want    error
	}{
		{name: "missing id", id: "", buyerID: "u_buyer", comment: "不错", want: ErrReviewIDRequired},
		{name: "missing buyer", id: "rv_1", buyerID: "", comment: "不错", want: ErrReviewBuyerRequired},
		{name: "empty comment", id: "rv_1", buyerID: "u_buyer", comment: "", want: ErrReviewCommentLength},
		{name: "comment too long", id: "rv_1", buyerID: "u_buyer", comment: strings.Repeat("好", 501), want: ErrReviewCommentLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewReview(tt.id, "p_1", tt.buyerID, tt.comment, time.Now()); !errors.Is(err, tt.want) {
				t.Fatalf("NewReview error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateReviewCommentBoundaries(t *testing.T) {
	t.Parallel()

	if err := ValidateReviewComment(strings.Repeat("好", 500)); err != nil {
		t.Fatalf("500 characters must be valid: %v", err)
	}
	if err := ValidateReviewComment(strings.Repeat("好", 501)); !errors.Is(err, ErrReviewCommentLength) {
		t.Fatalf("501 characters must be rejected, got %v", err)
	}
}
