package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// MaxReviewCommentLength 是公开 ReviewComment 的长度上限。
const MaxReviewCommentLength = 500

// 评论错误。
var (
	ErrReviewIDRequired    = errors.New("review id is required")
	ErrReviewBuyerRequired = errors.New("review buyer is required")
	ErrReviewCommentLength = errors.New("comment must be 1-500 characters")
)

// Review 是买家在交易完成后就商品发布的不可变文字评论。
//
// docs/state-machines.md：评论发布后没有更新或删除路径，也不产生商品或交易
// 副作用。TradeID 记录产生评论的已完成交易，用于审计追溯；由于一个买家对
// 一个商品终生只有一笔交易，它同时是 (product_id, buyer_id) 唯一性的来源。
type Review struct {
	ID        string
	ProductID string
	TradeID   string
	BuyerID   string
	Comment   string
	CreatedAt time.Time
}

// NewReview 构造一条待插入的评论。交易授权由应用层在事务中校验，
// 这里只保证评论自身字段合法。
func NewReview(id, productID, buyerID, comment string, now time.Time) (*Review, error) {
	if id == "" {
		return nil, ErrReviewIDRequired
	}
	if buyerID == "" {
		return nil, ErrReviewBuyerRequired
	}
	if err := ValidateReviewComment(comment); err != nil {
		return nil, err
	}
	return &Review{
		ID:        id,
		ProductID: productID,
		BuyerID:   buyerID,
		Comment:   comment,
		CreatedAt: now,
	}, nil
}

// ValidateReviewComment 强制执行公开评论正文限制。
func ValidateReviewComment(comment string) error {
	length := utf8.RuneCountInString(comment)
	if length < 1 || length > MaxReviewCommentLength {
		return ErrReviewCommentLength
	}
	return nil
}
