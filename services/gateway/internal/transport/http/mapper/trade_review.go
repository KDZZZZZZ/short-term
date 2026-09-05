package mapper

import (
	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// TradeReview maps one trade review, completing the buyer identity from the
// batch profile lookup the caller performed. 空文字映射为 null（买家未填写）。
func TradeReview(src *marketplacev1.TradeReview, users map[string]*accountv1.UserPublic) dto.TradeReview {
	var content *string
	if src.GetContent() != "" {
		value := src.GetContent()
		content = &value
	}
	return dto.TradeReview{
		ID:        src.GetId(),
		TradeID:   src.GetTradeId(),
		ProductID: src.GetProductId(),
		Buyer:     UserPublic(src.GetBuyerId(), users[src.GetBuyerId()]),
		Score:     src.GetScore(),
		Content:   content,
		CreatedAt: Timestamp(src.GetCreatedAt()),
	}
}

// TradeReviewBuyerIDs collects the identities a set of trade reviews needs,
// so the caller can complete them in one batch call.
func TradeReviewBuyerIDs(reviews map[string]*marketplacev1.TradeReview) []string {
	ids := make([]string, 0, len(reviews))
	for _, review := range reviews {
		ids = append(ids, review.GetBuyerId())
	}
	return ids
}
