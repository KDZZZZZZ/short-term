package application

// CreateTradeReviewCommand 发布一条买家评价。
type CreateTradeReviewCommand struct {
	ActorID string
	TradeID string
	Content string
}
