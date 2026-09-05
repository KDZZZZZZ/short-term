package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewProductCommentAcceptsValidContent(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
	comment, err := NewProductComment("cm_1", "p_1", "u_user", "很不错的商品", created)
	if err != nil {
		t.Fatalf("NewProductComment: %v", err)
	}
	if comment.ID != "cm_1" || comment.ProductID != "p_1" || comment.UserID != "u_user" {
		t.Fatalf("unexpected comment: %+v", comment)
	}
}

func TestNewProductCommentRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		userID  string
		content string
		want    error
	}{
		{name: "missing id", id: "", userID: "u_user", content: "不错", want: ErrCommentIDRequired},
		{name: "missing user", id: "cm_1", userID: "", content: "不错", want: ErrCommentUserRequired},
		{name: "empty content", id: "cm_1", userID: "u_user", content: "", want: ErrCommentContentLength},
		{name: "content too long", id: "cm_1", userID: "u_user", content: strings.Repeat("好", 501), want: ErrCommentContentLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewProductComment(tt.id, "p_1", tt.userID, tt.content, time.Now()); !errors.Is(err, tt.want) {
				t.Fatalf("NewProductComment error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateCommentContentBoundaries(t *testing.T) {
	t.Parallel()

	if err := ValidateCommentContent(strings.Repeat("好", 500)); err != nil {
		t.Fatalf("500 characters must be valid: %v", err)
	}
	if err := ValidateCommentContent(strings.Repeat("好", 501)); !errors.Is(err, ErrCommentContentLength) {
		t.Fatalf("501 characters must be rejected, got %v", err)
	}
}
