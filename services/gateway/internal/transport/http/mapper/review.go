package mapper

import (
	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// Review maps one review, completing the buyer identity from the batch
// profile lookup the caller performed.
func Review(src *marketplacev1.Review, users map[string]*accountv1.UserPublic) dto.Review {
	return dto.Review{
		ID:        src.GetId(),
		ProductID: src.GetProductId(),
		Buyer:     UserPublic(src.GetBuyerId(), users[src.GetBuyerId()]),
		Comment:   src.GetComment(),
		CreatedAt: Timestamp(src.GetCreatedAt()),
	}
}

// ReviewPage maps a page of reviews.
func ReviewPage(src *marketplacev1.ReviewPage, users map[string]*accountv1.UserPublic) dto.ReviewPage {
	items := make([]dto.Review, 0, len(src.GetItems()))
	for _, item := range src.GetItems() {
		items = append(items, Review(item, users))
	}
	return dto.ReviewPage{
		Items:    items,
		Page:     src.GetPage(),
		PageSize: src.GetPageSize(),
		Total:    src.GetTotal(),
	}
}

// ReviewBuyerIDs collects the identities a page of reviews needs, so the
// caller can complete them in one batch call.
func ReviewBuyerIDs(page *marketplacev1.ReviewPage) []string {
	ids := make([]string, 0, len(page.GetItems()))
	for _, review := range page.GetItems() {
		ids = append(ids, review.GetBuyerId())
	}
	return ids
}
