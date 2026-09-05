package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewTradeReviewCopiesTheCompletedTrade(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	trade := &Trade{ID: "t_1", ProductID: "p_1", BuyerID: "u_buyer", SellerID: "u_seller"}
	review, err := NewTradeReview("tr_1", trade, 5, "交易愉快", created)
	if err != nil {
		t.Fatalf("NewTradeReview: %v", err)
	}
	if review.ID != "tr_1" || review.TradeID != "t_1" || review.ProductID != "p_1" || review.BuyerID != "u_buyer" {
		t.Fatalf("unexpected review: %+v", review)
	}
	if review.Score != 5 || review.Content != "交易愉快" {
		t.Fatalf("score/content lost: %+v", review)
	}
}

func TestNewTradeReviewAllowsEmptyContent(t *testing.T) {
	t.Parallel()

	trade := &Trade{ID: "t_1", ProductID: "p_1", BuyerID: "u_buyer", SellerID: "u_seller"}
	review, err := NewTradeReview("tr_1", trade, 4, "", time.Now())
	if err != nil {
		t.Fatalf("NewTradeReview without content: %v", err)
	}
	if review.Content != "" || review.Score != 4 {
		t.Fatalf("unexpected review: %+v", review)
	}
}

func TestNewTradeReviewRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	empty := &Trade{ID: "t_1", ProductID: "p_1", SellerID: "u_seller"}
	valid := &Trade{ID: "t_1", ProductID: "p_1", BuyerID: "u_buyer"}

	tests := []struct {
		name    string
		id      string
		trade   *Trade
		score   int32
		content string
		want    error
	}{
		{name: "missing id", id: "", trade: valid, score: 5, content: "不错", want: ErrTradeReviewIDRequired},
		{name: "missing buyer", id: "tr_1", trade: empty, score: 5, content: "不错", want: ErrTradeReviewBuyerRequired},
		{name: "score too low", id: "tr_1", trade: valid, score: 0, content: "不错", want: ErrTradeReviewScoreRange},
		{name: "score too high", id: "tr_1", trade: valid, score: 6, content: "不错", want: ErrTradeReviewScoreRange},
		{name: "content too long", id: "tr_1", trade: valid, score: 5, content: strings.Repeat("好", 501), want: ErrTradeReviewContentLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewTradeReview(tt.id, tt.trade, tt.score, tt.content, time.Now()); !errors.Is(err, tt.want) {
				t.Fatalf("NewTradeReview error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateTradeReviewBoundaries(t *testing.T) {
	t.Parallel()

	if err := ValidateTradeReviewScore(1); err != nil {
		t.Fatalf("score 1 must be valid: %v", err)
	}
	if err := ValidateTradeReviewScore(5); err != nil {
		t.Fatalf("score 5 must be valid: %v", err)
	}
	if err := ValidateTradeReviewContent(strings.Repeat("好", 500)); err != nil {
		t.Fatalf("500 characters must be valid: %v", err)
	}
	if err := ValidateTradeReviewContent(strings.Repeat("好", 501)); !errors.Is(err, ErrTradeReviewContentLength) {
		t.Fatalf("501 characters must be rejected, got %v", err)
	}
	if err := ValidateTradeReviewContent(""); err != nil {
		t.Fatalf("empty content is the optional no-text form: %v", err)
	}
}
