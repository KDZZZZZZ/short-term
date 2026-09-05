package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// MaxTradeReviewContentLength 是公开评价文字的长度上限。
const MaxTradeReviewContentLength = 500

// MinTradeReviewScore 和 MaxTradeReviewScore 是公开评分的取值范围。
const (
	MinTradeReviewScore = 1
	MaxTradeReviewScore = 5
)

// 买家评价错误。
var (
	ErrTradeReviewIDRequired    = errors.New("trade review id is required")
	ErrTradeReviewBuyerRequired = errors.New("trade review buyer is required")
	ErrTradeReviewContentLength = errors.New("content must be empty or 1-500 characters")
	ErrTradeReviewScoreRange    = errors.New("score must be between 1 and 5")
)

// TradeReview 是买家在交易完成后发布的不可变评价。
//
// docs/state-machines.md：评价与商品用户评论相互独立，不产生商品或交易
// 副作用；每笔交易最多一条，发布后没有更新或删除路径。评分必填，文字可选
// （空字符串表示未填写）。
type TradeReview struct {
	ID        string
	TradeID   string
	ProductID string
	BuyerID   string
	Content   string
	Score     int32
	CreatedAt time.Time
}

// NewTradeReview 构造一条待插入的评价。交易授权由应用层校验，
// 这里只保证评价自身字段合法。
func NewTradeReview(id string, trade *Trade, score int32, content string, now time.Time) (*TradeReview, error) {
	if id == "" {
		return nil, ErrTradeReviewIDRequired
	}
	if trade.BuyerID == "" {
		return nil, ErrTradeReviewBuyerRequired
	}
	if err := ValidateTradeReviewScore(score); err != nil {
		return nil, err
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
		Score:     score,
		CreatedAt: now,
	}, nil
}

// ValidateTradeReviewScore 强制执行公开评分取值范围。
func ValidateTradeReviewScore(score int32) error {
	if score < MinTradeReviewScore || score > MaxTradeReviewScore {
		return ErrTradeReviewScoreRange
	}
	return nil
}

// ValidateTradeReviewContent 强制执行公开评价文字限制。文字可选：
// 空字符串表示未填写。
func ValidateTradeReviewContent(content string) error {
	if utf8.RuneCountInString(content) > MaxTradeReviewContentLength {
		return ErrTradeReviewContentLength
	}
	return nil
}
