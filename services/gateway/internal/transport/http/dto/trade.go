package dto

// TradeProduct is the TradeProduct schema: the product projection carried by a
// trade, always with the product's current status.
type TradeProduct struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	CoverURL *string `json:"cover_url"`
	Status   string  `json:"status"`
}

// Trade is the Trade schema.
type Trade struct {
	ID              string       `json:"id"`
	Product         TradeProduct `json:"product"`
	Buyer           UserPublic   `json:"buyer"`
	Seller          UserPublic   `json:"seller"`
	ConversationID  *string      `json:"conversation_id"`
	PriceSnapshot   string       `json:"price_snapshot"`
	Status          string       `json:"status"`
	BuyerConfirmed  bool         `json:"buyer_confirmed"`
	SellerConfirmed bool         `json:"seller_confirmed"`
	CancelReason    *string      `json:"cancel_reason"`
	CreatedAt       string       `json:"created_at"`
	AcceptedAt      *string      `json:"accepted_at"`
	CompletedAt     *string      `json:"completed_at"`
	CancelledAt     *string      `json:"cancelled_at"`
	UpdatedAt       string       `json:"updated_at"`
}

// TradePage is the TradePage schema.
type TradePage struct {
	Items    []Trade `json:"items"`
	Page     int32   `json:"page"`
	PageSize int32   `json:"page_size"`
	Total    int64   `json:"total"`
}

// TradeCreateRequest is the TradeCreateRequest schema. The body is optional
// and its only property is nullable.
type TradeCreateRequest struct {
	ConversationID RawField `json:"conversation_id"`
}

// ReasonRequest is the ReasonRequest schema used by reject and cancel.
type ReasonRequest struct {
	Reason string `json:"reason"`
}
