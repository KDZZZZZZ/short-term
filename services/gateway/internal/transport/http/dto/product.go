package dto

// ProductImage 是 ProductImage schema。
type ProductImage struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	SortOrder int32  `json:"sort_order"`
	CreatedAt string `json:"created_at"`
}

// ProductSummary 是所有列表使用的 ProductSummary schema。
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

// ProductDetail 是 ProductDetail schema。卖家使用 SellerContact，
// 其中不包含学号字段。
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

// ProductPage 是 ProductPage schema。
type ProductPage struct {
	Items    []ProductSummary `json:"items"`
	Page     int32            `json:"page"`
	PageSize int32            `json:"page_size"`
	Total    int64            `json:"total"`
}

// ProductImageList 是 ProductImageListData schema。
type ProductImageList struct {
	Images []ProductImage `json:"images"`
}

// ProductUpdateRequest 是 ProductUpdateRequest schema。每个属性都是可选的，
// 但至少要出现一个，且这些属性都不可为 null。
type ProductUpdateRequest struct {
	Title       *string `json:"title"`
	Price       *string `json:"price"`
	Category    *string `json:"category"`
	Description *string `json:"description"`
}
