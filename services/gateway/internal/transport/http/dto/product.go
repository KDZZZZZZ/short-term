package dto

// ProductImage is the ProductImage schema.
type ProductImage struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	SortOrder int32  `json:"sort_order"`
	CreatedAt string `json:"created_at"`
}

// ProductSummary is the ProductSummary schema used by every list.
type ProductSummary struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Price     string     `json:"price"`
	Category  string     `json:"category"`
	CoverURL  *string    `json:"cover_url"`
	Status    string     `json:"status"`
	Seller    UserPublic `json:"seller"`
	CreatedAt string     `json:"created_at"`
}

// ProductDetail is the ProductDetail schema. The seller is a SellerContact,
// which has no student number field.
type ProductDetail struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Price       string         `json:"price"`
	Category    string         `json:"category"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Images      []ProductImage `json:"images"`
	Seller      SellerContact  `json:"seller"`
	IsFavorited bool           `json:"is_favorited"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// ProductPage is the ProductPage schema.
type ProductPage struct {
	Items    []ProductSummary `json:"items"`
	Page     int32            `json:"page"`
	PageSize int32            `json:"page_size"`
	Total    int64            `json:"total"`
}

// ProductImageList is the ProductImageListData schema.
type ProductImageList struct {
	Images []ProductImage `json:"images"`
}

// ProductUpdateRequest is the ProductUpdateRequest schema. Every property is
// optional but at least one must be present, and none of them is nullable.
type ProductUpdateRequest struct {
	Title       *string `json:"title"`
	Price       *string `json:"price"`
	Category    *string `json:"category"`
	Description *string `json:"description"`
}
