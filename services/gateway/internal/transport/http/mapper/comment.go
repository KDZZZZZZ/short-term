package mapper

import (
	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// Comment maps one comment, completing the commenter identity from the batch
// profile lookup the caller performed.
func Comment(src *marketplacev1.ProductComment, users map[string]*accountv1.UserPublic) dto.Comment {
	return dto.Comment{
		ID:        src.GetId(),
		ProductID: src.GetProductId(),
		User:      UserPublic(src.GetUserId(), users[src.GetUserId()]),
		Content:   src.GetContent(),
		CreatedAt: Timestamp(src.GetCreatedAt()),
	}
}

// CommentPage maps a page of comments.
func CommentPage(src *marketplacev1.ProductCommentPage, users map[string]*accountv1.UserPublic) dto.CommentPage {
	items := make([]dto.Comment, 0, len(src.GetItems()))
	for _, item := range src.GetItems() {
		items = append(items, Comment(item, users))
	}
	return dto.CommentPage{
		Items:    items,
		Page:     src.GetPage(),
		PageSize: src.GetPageSize(),
		Total:    src.GetTotal(),
	}
}

// CommentUserIDs collects the identities a page of comments needs, so the
// caller can complete them in one batch call.
func CommentUserIDs(page *marketplacev1.ProductCommentPage) []string {
	ids := make([]string, 0, len(page.GetItems()))
	for _, comment := range page.GetItems() {
		ids = append(ids, comment.GetUserId())
	}
	return ids
}
