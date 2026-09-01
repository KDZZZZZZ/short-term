package dto

// FavoriteItem combines a relationship timestamp from Favorite Service with
// the current ProductSummary assembled by Gateway.
type FavoriteItem struct {
	Product     ProductSummary `json:"product"`
	FavoritedAt string         `json:"favorited_at"`
}

// FavoritePage is the public FavoritePage schema.
type FavoritePage struct {
	Items    []FavoriteItem `json:"items"`
	Page     int32          `json:"page"`
	PageSize int32          `json:"page_size"`
	Total    int64          `json:"total"`
}
