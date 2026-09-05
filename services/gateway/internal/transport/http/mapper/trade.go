package mapper

import (
	accountv1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/account/v1"
	marketplacev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/marketplace/v1"
	"github.com/KDZZZZZZ/short-term/services/gateway/internal/transport/http/dto"
)

// tradeStatuses maps between the wire enum and the public string enum.
var tradeStatuses = map[marketplacev1.TradeStatus]string{
	marketplacev1.TradeStatus_TRADE_STATUS_PENDING:   "PENDING",
	marketplacev1.TradeStatus_TRADE_STATUS_ACCEPTED:  "ACCEPTED",
	marketplacev1.TradeStatus_TRADE_STATUS_COMPLETED: "COMPLETED",
	marketplacev1.TradeStatus_TRADE_STATUS_CANCELLED: "CANCELLED",
}

// TradeStatus renders the public trade status string.
func TradeStatus(value marketplacev1.TradeStatus) string { return tradeStatuses[value] }

// ParseTradeStatus converts a public status string to the wire enum.
func ParseTradeStatus(value string) (marketplacev1.TradeStatus, bool) {
	for enum, name := range tradeStatuses {
		if name == value {
			return enum, true
		}
	}
	return marketplacev1.TradeStatus_TRADE_STATUS_UNSPECIFIED, false
}

// Trade maps one trade, completing the two parties' identities and contacts
// from the batch contact lookup the caller performed.
func Trade(src *marketplacev1.Trade, contacts map[string]*accountv1.UserContact) dto.Trade {
	return dto.Trade{
		ID: src.GetId(),
		Product: dto.TradeProduct{
			ID:       src.GetProduct().GetId(),
			Title:    src.GetProduct().GetTitle(),
			CoverURL: src.GetProduct().CoverUrl,
			Status:   ProductStatus(src.GetProduct().GetStatus()),
		},
		Buyer:           TradeParty(src.GetBuyerId(), contacts[src.GetBuyerId()]),
		Seller:          TradeParty(src.GetSellerId(), contacts[src.GetSellerId()]),
		ConversationID:  src.ConversationId,
		PriceSnapshot:   FormatPrice(src.GetPriceSnapshotMinor()),
		Status:          TradeStatus(src.GetStatus()),
		BuyerConfirmed:  src.GetBuyerConfirmed(),
		SellerConfirmed: src.GetSellerConfirmed(),
		CancelReason:    src.CancelReason,
		CreatedAt:       Timestamp(src.GetCreatedAt()),
		AcceptedAt:      OptionalTimestamp(src.AcceptedAt),
		CompletedAt:     OptionalTimestamp(src.CompletedAt),
		CancelledAt:     OptionalTimestamp(src.CancelledAt),
		UpdatedAt:       Timestamp(src.GetUpdatedAt()),
	}
}

// TradePage maps a page of trades.
func TradePage(src *marketplacev1.TradePage, contacts map[string]*accountv1.UserContact) dto.TradePage {
	items := make([]dto.Trade, 0, len(src.GetItems()))
	for _, item := range src.GetItems() {
		items = append(items, Trade(item, contacts))
	}
	return dto.TradePage{
		Items:    items,
		Page:     src.GetPage(),
		PageSize: src.GetPageSize(),
		Total:    src.GetTotal(),
	}
}

// TradeParticipantIDs collects the identities a page of trades needs, so the
// caller can complete them in one batch call.
func TradeParticipantIDs(page *marketplacev1.TradePage) []string {
	ids := make([]string, 0, len(page.GetItems())*2)
	for _, trade := range page.GetItems() {
		ids = append(ids, trade.GetBuyerId(), trade.GetSellerId())
	}
	return ids
}
