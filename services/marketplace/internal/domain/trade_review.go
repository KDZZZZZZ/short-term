package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// MaxTradeReviewContentLength 是公开 TradeReviewContent 的长度上限。
const MaxTradeReviewContentLength = 500

// 买家评价错误。
var (
	ErrTradeReviewIDRequired    = errors.New("trade review id is required")
	ErrTradeReviewBuyerRequired = errors.New("trade review buyer is required")
	ErrTradeReviewContentLength = errors.New("content must be 1-500 characters")
)

// TradeReview 是买家在交易完成后发布的不可变文字评价。
//
// docs/state-machines.md：评价与商品用户评论相互独立，不产生商品或交易
// 副作用；每笔交易最多一条，发布后没有更新或删除路径。
type TradeReview struct {
	ID        string
	TradeID   string
	ProductID string
	BuyerID   string
	Content   string
	CreatedAt time.Time
}

// NewTradeReview 构造一条待插入的评价。交易授权由应用层校验，
// 这里只保证评价自身字段合法。
func NewTradeReview(id string, trade *Trade, content string, now time.Time) (*TradeReview, error) {
	if id == "" {
		return nil, ErrTradeReviewIDRequired
	}
	if trade.BuyerID == "" {
		return nil, ErrTradeReviewBuyerRequired
	}
	if err := ValidateTradeReviewContent(content); err != nil {
		return nil, err
	}
	return &TradeReview{
		ID:        id,
		TradeID:   trade.ID,
		ProductID: trade.ProductID,
		BuyerID:   trade.BuyerID,
		Content:   content,
		CreatedAt: now,
	}, nil
}

// ValidateTradeReviewContent 强制执行公开评价正文限制。
func ValidateTradeReviewContent(content string) error {
	length := utf8.RuneCountInString(content)
	if length < 1 || length > MaxTradeReviewContentLength {
		return ErrTradeReviewContentLength
	}
	return nil
}
