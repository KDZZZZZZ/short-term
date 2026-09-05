package dto

// TradeReview is the TradeReview schema.
type TradeReview struct {
	ID        string     `json:"id"`
	TradeID   string     `json:"trade_id"`
	ProductID string     `json:"product_id"`
	Buyer     UserPublic `json:"buyer"`
	Content   string     `json:"content"`
	CreatedAt string     `json:"created_at"`
}

// TradeReviewCreateRequest is the TradeReviewCreateRequest schema.
type TradeReviewCreateRequest struct {
	Content string `json:"content"`
}
