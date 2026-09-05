package dto

// TradeReview is the TradeReview schema.
type TradeReview struct {
	ID        string     `json:"id"`
	TradeID   string     `json:"trade_id"`
	ProductID string     `json:"product_id"`
	Buyer     UserPublic `json:"buyer"`
	Score     int32      `json:"score"`
	Content   *string    `json:"content"`
	CreatedAt string     `json:"created_at"`
}

// TradeReviewCreateRequest is the TradeReviewCreateRequest schema.
type TradeReviewCreateRequest struct {
	Score int32 `json:"score"`
	// Content 是可选的评价文字；省略或 null 表示未填写文字，空字符串无效。
	Content *string `json:"content"`
}
