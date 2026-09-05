package application

// CreateCommentCommand 发布一条商品用户评论。
type CreateCommentCommand struct {
	ActorID   string
	ProductID string
	Content   string
}

// ListCommentsQuery 查看商品的用户评论。
type ListCommentsQuery struct {
	ProductID string
	Page      Page
}
