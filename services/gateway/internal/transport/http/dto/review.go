package dto

// Review is the Review schema.
type Review struct {
	ID        string     `json:"id"`
	ProductID string     `json:"product_id"`
	Buyer     UserPublic `json:"buyer"`
	Comment   string     `json:"comment"`
	CreatedAt string     `json:"created_at"`
}

// ReviewPage is the ReviewPage schema.
type ReviewPage struct {
	Items    []Review `json:"items"`
	Page     int32    `json:"page"`
	PageSize int32    `json:"page_size"`
	Total    int64    `json:"total"`
}

// ReviewCreateRequest is the ReviewCreateRequest schema.
type ReviewCreateRequest struct {
	Comment string `json:"comment"`
}
