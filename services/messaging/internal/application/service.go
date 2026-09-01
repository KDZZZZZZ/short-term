package application

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/observability"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/domain"
)

// Service implements product-context conversations, messages and read state.
type Service struct {
	repository Repository
	products   ProductReader
	ids        IDGenerator
	clock      Clock
	logger     *slog.Logger
}

func NewService(repository Repository, products ProductReader, ids IDGenerator, clock Clock, logger *slog.Logger) (*Service, error) {
	if repository == nil || products == nil || ids == nil || clock == nil {
		return nil, errors.New("application: every messaging dependency is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repository: repository, products: products, ids: ids, clock: clock, logger: logger}, nil
}

type ConversationResult struct {
	View     ConversationView
	Replayed bool
}

type MessageResult struct {
	Message  *domain.Message
	Replayed bool
}

// GetOrCreateConversation returns the one conversation for a product and buyer.
// The Marketplace lookup runs inside the command callback so an idempotency
// replay is returned before current product state or dependency availability is checked.
func (s *Service) GetOrCreateConversation(ctx context.Context, cmd GetOrCreateConversationCommand) (ConversationResult, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return ConversationResult{}, err
	}
	if err := validateIdentifier(cmd.ProductID, "商品标识"); err != nil {
		return ConversationResult{}, err
	}
	key, err := idempotencyKey(cmd.ActorID, OpGetOrCreateConversation, cmd.IdempotencyKey)
	if err != nil {
		return ConversationResult{}, err
	}

	result, replayed, err := s.repository.Execute(ctx, key, func(ctx context.Context, tx Tx) (*CommandResult, error) {
		product, err := s.products.Get(ctx, cmd.ProductID)
		if err != nil {
			if errs.CodeOf(err) == errs.CodeResourceNotFound || errors.Is(err, ErrNotFound) {
				return nil, errs.New(errs.CodeResourceNotFound, "商品不存在")
			}
			return nil, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
		}
		if product.ID != cmd.ProductID || product.SellerID == "" {
			return nil, errs.New(errs.CodeInternal, "服务暂时不可用")
		}
		if product.SellerID == cmd.ActorID {
			return nil, errs.New(errs.CodeSelfActionNotAllowed, "不能咨询自己发布的商品")
		}

		now := s.clock.Now()
		candidate, err := domain.NewConversation(
			s.ids.NewConversationID(), product.ID, cmd.ActorID, product.SellerID, now,
		)
		if err != nil {
			return nil, errs.Wrap(errs.CodeValidation, "会话参与者不合法", err)
		}
		conversation, _, err := tx.GetOrCreateConversation(ctx, candidate)
		if err != nil {
			return nil, err
		}
		view, err := tx.ConversationView(ctx, conversation.ID, cmd.ActorID)
		if err != nil {
			return nil, err
		}
		return &CommandResult{Code: "CONVERSATION_READY", ConversationView: &view}, nil
	})
	if err != nil {
		return ConversationResult{}, commandError(err)
	}
	if result.ConversationView == nil {
		return ConversationResult{}, errs.New(errs.CodeInternal, "服务暂时不可用")
	}
	return ConversationResult{View: *result.ConversationView, Replayed: replayed}, nil
}

// GetConversation returns a private conversation only to a participant. This
// is also the fact-source RPC used by Marketplace to validate trade binding.
func (s *Service) GetConversation(ctx context.Context, actorID, conversationID string) (*domain.Conversation, error) {
	if err := requireActor(actorID); err != nil {
		return nil, err
	}
	if err := validateIdentifier(conversationID, "会话标识"); err != nil {
		return nil, err
	}
	conversation, err := s.repository.ConversationByID(ctx, conversationID)
	if err != nil {
		return nil, readError(err, "会话不存在")
	}
	if !conversation.IsParticipant(actorID) {
		return nil, errs.New(errs.CodeResourceNotFound, "会话不存在")
	}
	return conversation, nil
}

func (s *Service) ListConversations(ctx context.Context, query ListConversationsQuery) (ConversationPage, error) {
	if err := requireActor(query.ActorID); err != nil {
		return ConversationPage{}, err
	}
	page, err := s.repository.ListConversations(ctx, query.ActorID, query.Page.normalize())
	if err != nil {
		return ConversationPage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return page, nil
}

func (s *Service) UnreadCount(ctx context.Context, actorID string) (int64, error) {
	if err := requireActor(actorID); err != nil {
		return 0, err
	}
	count, err := s.repository.UnreadCount(ctx, actorID)
	if err != nil {
		return 0, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}
	return count, nil
}

func (s *Service) ListMessages(ctx context.Context, query ListMessagesQuery) (MessagePage, error) {
	if err := requireActor(query.ActorID); err != nil {
		return MessagePage{}, err
	}
	if err := validateIdentifier(query.ConversationID, "会话标识"); err != nil {
		return MessagePage{}, err
	}
	conversation, err := s.repository.ConversationByID(ctx, query.ConversationID)
	if err != nil {
		return MessagePage{}, readError(err, "会话不存在")
	}
	if !conversation.IsParticipant(query.ActorID) {
		return MessagePage{}, errs.New(errs.CodeResourceNotFound, "会话不存在")
	}

	var cursor *MessageCursor
	if query.Before != nil {
		decoded, err := decodeCursor(*query.Before)
		if err != nil {
			return MessagePage{}, errs.New(errs.CodeValidation, "消息游标不合法")
		}
		cursor = &decoded
	}
	limit := normalizeMessageLimit(query.Limit)
	messages, err := s.repository.ListMessages(ctx, query.ConversationID, cursor, limit+1)
	if err != nil {
		return MessagePage{}, errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
	}

	page := MessagePage{Items: messages}
	if len(messages) > int(limit) {
		page.Items = messages[:limit]
		last := page.Items[len(page.Items)-1]
		next := encodeCursor(MessageCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		page.NextBefore = &next
	}
	if page.Items == nil {
		page.Items = []*domain.Message{}
	}
	return page, nil
}

func (s *Service) SendMessage(ctx context.Context, cmd SendMessageCommand) (MessageResult, error) {
	if err := requireActor(cmd.ActorID); err != nil {
		return MessageResult{}, err
	}
	if err := validateIdentifier(cmd.ConversationID, "会话标识"); err != nil {
		return MessageResult{}, err
	}
	if err := domain.ValidateContent(cmd.Content); err != nil {
		return MessageResult{}, errs.New(errs.CodeValidation, "消息长度必须为 1 至 1000 个字符")
	}
	key, err := idempotencyKey(cmd.ActorID, OpSendMessage, cmd.IdempotencyKey)
	if err != nil {
		return MessageResult{}, err
	}

	result, replayed, err := s.repository.Execute(ctx, key, func(ctx context.Context, tx Tx) (*CommandResult, error) {
		conversation, err := tx.LockConversation(ctx, cmd.ConversationID)
		if err != nil {
			return nil, readError(err, "会话不存在")
		}
		if !conversation.IsParticipant(cmd.ActorID) {
			return nil, errs.New(errs.CodeResourceNotFound, "会话不存在")
		}

		now := s.clock.Now()
		message, err := domain.NewMessage(s.ids.NewMessageID(), conversation, cmd.ActorID, cmd.Content, now)
		if err != nil {
			return nil, errs.Wrap(errs.CodeValidation, "消息不合法", err)
		}
		if err := tx.InsertMessage(ctx, message); err != nil {
			return nil, err
		}
		if err := tx.TouchConversation(ctx, conversation.ID, now); err != nil {
			return nil, err
		}
		event, err := (eventFactory{
			newID: s.ids.NewEventID, now: now, traceID: observability.TraceID(ctx),
		}).messageSent(message)
		if err != nil {
			return nil, err
		}
		if err := tx.AppendEvent(ctx, event); err != nil {
			return nil, err
		}
		return &CommandResult{Code: "MESSAGE_SENT", Message: message}, nil
	})
	if err != nil {
		return MessageResult{}, commandError(err)
	}
	return MessageResult{Message: result.Message, Replayed: replayed}, nil
}

// MarkConversationRead marks only counterpart messages through the supplied
// message. The update and ConversationRead event are one local transaction.
func (s *Service) MarkConversationRead(ctx context.Context, cmd MarkReadCommand) error {
	if err := requireActor(cmd.ActorID); err != nil {
		return err
	}
	if err := validateIdentifier(cmd.ConversationID, "会话标识"); err != nil {
		return err
	}
	if err := validateIdentifier(cmd.LastMessageID, "消息标识"); err != nil {
		return err
	}

	err := s.repository.Transact(ctx, func(ctx context.Context, tx Tx) error {
		conversation, err := tx.LockConversation(ctx, cmd.ConversationID)
		if err != nil {
			return readError(err, "会话不存在")
		}
		if !conversation.IsParticipant(cmd.ActorID) {
			return errs.New(errs.CodeResourceNotFound, "会话不存在")
		}
		anchor, err := tx.LockMessage(ctx, conversation.ID, cmd.LastMessageID)
		if err != nil {
			return readError(err, "消息不存在")
		}
		if anchor.SenderID == cmd.ActorID {
			return errs.New(errs.CodeValidation, "只能将对方发送的消息标记为已读")
		}

		now := s.clock.Now()
		changed, err := tx.MarkOpponentMessagesRead(
			ctx, conversation.ID, cmd.ActorID, anchor.CreatedAt, anchor.ID, now,
		)
		if err != nil {
			return err
		}
		if changed == 0 {
			return nil
		}
		event, err := (eventFactory{
			newID: s.ids.NewEventID, now: now, traceID: observability.TraceID(ctx),
		}).conversationRead(conversation.ID, cmd.ActorID, anchor.ID)
		if err != nil {
			return err
		}
		return tx.AppendEvent(ctx, event)
	})
	if err != nil {
		return commandError(err)
	}
	return nil
}

func requireActor(actorID string) error {
	if actorID == "" {
		return errs.New(errs.CodeUnauthorized, "请先登录")
	}
	return nil
}

func validateIdentifier(value, label string) error {
	length := utf8.RuneCountInString(value)
	if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 || length < 1 || length > 64 {
		return errs.Newf(errs.CodeValidation, "%s不合法", label)
	}
	return nil
}

func idempotencyKey(actorID, operation string, value *string) (*IdempotencyKey, error) {
	if value == nil {
		return nil, nil
	}
	length := utf8.RuneCountInString(*value)
	if !utf8.ValidString(*value) || strings.IndexByte(*value, 0) >= 0 || length < MinIdempotencyKeyLength || length > MaxIdempotencyKeyLength {
		return nil, errs.New(errs.CodeValidation, "Idempotency-Key 长度必须为 16 至 128 个字符")
	}
	return &IdempotencyKey{ActorID: actorID, Operation: operation, Key: *value}, nil
}

func readError(err error, message string) error {
	if errors.Is(err, ErrNotFound) {
		return errs.Wrap(errs.CodeResourceNotFound, message, err)
	}
	if errs.CodeOf(err) != errs.CodeInternal {
		return err
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}

func commandError(err error) error {
	if errs.CodeOf(err) != errs.CodeInternal {
		return err
	}
	if errors.Is(err, ErrIdempotencyRace) {
		return errs.Wrap(errs.CodeInternal, "请求幂等结果发生竞争，请重试", err)
	}
	return errs.Wrap(errs.CodeInternal, "服务暂时不可用", err)
}
