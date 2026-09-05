package application

// CreateTradeReviewCommand 发布一条买家评价。
type CreateTradeReviewCommand struct {
	ActorID string
	TradeID string
	Score   int32
	// Content 是可选的评价文字；空字符串表示买家未填写文字。
	Content string
}
