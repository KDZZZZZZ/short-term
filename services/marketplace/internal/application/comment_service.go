package application

import (
	"context"
	"errors"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/domain"
)

// CommentService 实现商品用户评论用例。
type CommentService struct {
	comments CommentRepository
	ids      IDGenerator
	clock    Clock
}

// NewCommentService 组装评论用例。
func NewCommentService(comments CommentRepository, ids IDGenerator, clock Clock) (*CommentService, error) {
	if comments == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every comment dependency is required")
	}
	return &CommentService{comments: comments, ids: ids, clock: clock}, nil
}

// Create 为任意已认证用户在任意现存商品上发布一条评论。
//
// docs/state-machines.md：不要求存在交易，不限商品状态，同一用户可以对同一
// 商品发布多条评论；发布评论不改变商品或交易状态。商品不存在时由外键约束
// 报告为 RESOURCE_NOT_FOUND。
func (s *CommentService) Create(ctx context.Context, cmd CreateCommentCommand) (*domain.ProductComment, error) {
	if cmd.ActorID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "请先登录")
	}
	if err := domain.ValidateCommentContent(cmd.Content); err != nil {
		return nil, errs.Wrap(errs.CodeValidation, "评论长度必须为 1 至 500 个字符", err)
	}

	comment, err := domain.NewProductComment(s.ids.NewCommentID(), cmd.ProductID, cmd.ActorID, cmd.Content, s.clock.Now())
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "评论初始化失败", err)
	}
	if err := s.comments.Insert(ctx, comment); err != nil {
		return nil, insertCommentError(err)
	}
	return comment, nil
}

// List 返回商品收到的全部评论。评论与商品状态无关，
// 但商品本身必须存在。
func (s *CommentService) List(ctx context.Context, query ListCommentsQuery) (CommentPage, error) {
	exists, err := s.comments.ProductExists(ctx, query.ProductID)
	if err != nil {
		return CommentPage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	if !exists {
		return CommentPage{}, errs.New(errs.CodeResourceNotFound, "商品不存在")
	}

	page, err := s.comments.ListProductComments(ctx, query.ProductID, query.Page.normalize())
	if err != nil {
		return CommentPage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return page, nil
}

// insertCommentError 映射评论写入失败。外键冲突表示商品不存在；
// 其余失败一律按内部错误处理。
func insertCommentError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, "商品不存在", err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}
