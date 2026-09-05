package application

// CreateReviewCommand 发布一条买家评论。
type CreateReviewCommand struct {
	ActorID   string
	ProductID string
	Comment   string
}

// ListReviewsQuery 查看商品的买家评论。
type ListReviewsQuery struct {
	ProductID string
	Page      Page
}
