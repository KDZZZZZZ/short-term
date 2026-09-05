package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// MaxCommentContentLength 是公开 CommentContent 的长度上限。
const MaxCommentContentLength = 500

// 评论错误。
var (
	ErrCommentIDRequired    = errors.New("comment id is required")
	ErrCommentUserRequired  = errors.New("comment user is required")
	ErrCommentContentLength = errors.New("content must be 1-500 characters")
)

// ProductComment 是用户就商品发布的不可变文字评论。
//
// docs/state-machines.md：评论不依赖交易、不产生商品或交易副作用，发布后
// 没有更新或删除路径。同一用户可以对同一商品发布任意多条评论。
type ProductComment struct {
	ID        string
	ProductID string
	UserID    string
	Content   string
	CreatedAt time.Time
}

// NewProductComment 构造一条待插入的评论。这里只保证评论自身字段合法；
// 商品存在性由仓储层外键约束兜底。
func NewProductComment(id, productID, userID, content string, now time.Time) (*ProductComment, error) {
	if id == "" {
		return nil, ErrCommentIDRequired
	}
	if userID == "" {
		return nil, ErrCommentUserRequired
	}
	if err := ValidateCommentContent(content); err != nil {
		return nil, err
	}
	return &ProductComment{
		ID:        id,
		ProductID: productID,
		UserID:    userID,
		Content:   content,
		CreatedAt: now,
	}, nil
}

// ValidateCommentContent 强制执行公开评论正文限制。
func ValidateCommentContent(content string) error {
	length := utf8.RuneCountInString(content)
	if length < 1 || length > MaxCommentContentLength {
		return ErrCommentContentLength
	}
	return nil
}
