package dto

// Comment is the Comment schema.
type Comment struct {
	ID        string     `json:"id"`
	ProductID string     `json:"product_id"`
	User      UserPublic `json:"user"`
	Content   string     `json:"content"`
	CreatedAt string     `json:"created_at"`
}

// CommentPage is the CommentPage schema.
type CommentPage struct {
	Items    []Comment `json:"items"`
	Page     int32     `json:"page"`
	PageSize int32     `json:"page_size"`
	Total    int64     `json:"total"`
}

// CommentCreateRequest is the CommentCreateRequest schema.
type CommentCreateRequest struct {
	Content string `json:"content"`
}
