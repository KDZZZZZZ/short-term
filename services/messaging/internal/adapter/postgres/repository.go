// Package postgres persists Messaging Service state in PostgreSQL.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
	"github.com/KDZZZZZZ/short-term/services/messaging/internal/domain"
)

const (
	idempotencyPK            = "idempotency_records_pkey"
	idempotencyLockNamespace = int32(0x4d53)
)

const conversationColumns = `id, product_id, buyer_id, seller_id, created_at, last_message_at`
const messageColumns = `id, conversation_id, sender_id, content, read_at, created_at`

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var _ application.Repository = (*Repository)(nil)

func (r *Repository) Execute(
	ctx context.Context,
	key *application.IdempotencyKey,
	fn func(context.Context, application.Tx) (*application.CommandResult, error),
) (*application.CommandResult, bool, error) {
	var (
		result   *application.CommandResult
		replayed bool
	)
	err := pg.InTx(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if key != nil {
			if err := lockIdempotencyKey(ctx, tx, *key); err != nil {
				return err
			}
			stored, found, err := readIdempotencyRecord(ctx, tx, *key)
			if err != nil {
				return err
			}
			if found {
				result, replayed = stored, true
				return nil
			}
		}

		commandResult, err := fn(ctx, &messagingTx{tx: tx})
		if err != nil {
			return err
		}
		if key != nil {
			if err := writeIdempotencyRecord(ctx, tx, *key, commandResult); err != nil {
				return err
			}
		}
		result = commandResult
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, replayed, nil
}

func (r *Repository) Transact(ctx context.Context, fn func(context.Context, application.Tx) error) error {
	return pg.InTx(ctx, r.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return fn(ctx, &messagingTx{tx: tx})
	})
}

func (r *Repository) ConversationByID(ctx context.Context, conversationID string) (*domain.Conversation, error) {
	conversation, err := scanConversation(r.pool.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM conversations WHERE id = $1`, conversationID,
	))
	if err != nil {
		return nil, fmt.Errorf("postgres: select conversation: %w", err)
	}
	return conversation, nil
}

func (r *Repository) ListConversations(ctx context.Context, actorID string, page application.Page) (application.ConversationPage, error) {
	const countQuery = `
		SELECT count(*)
		  FROM conversations
		 WHERE buyer_id = $1 OR seller_id = $1`
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, actorID).Scan(&total); err != nil {
		return application.ConversationPage{}, fmt.Errorf("postgres: count conversations: %w", err)
	}

	const listQuery = `
		SELECT c.id, c.product_id, c.buyer_id, c.seller_id, c.created_at, c.last_message_at,
		       lm.id, lm.sender_id, lm.content, lm.read_at, lm.created_at,
		       (SELECT count(*)
		          FROM messages unread
		         WHERE unread.conversation_id = c.id
		           AND unread.sender_id <> $1
		           AND unread.read_at IS NULL) AS unread_count
		  FROM conversations c
		  LEFT JOIN LATERAL (
		       SELECT id, sender_id, content, read_at, created_at
		         FROM messages
		        WHERE conversation_id = c.id
		        ORDER BY created_at DESC, id DESC
		        LIMIT 1
		  ) lm ON true
		 WHERE c.buyer_id = $1 OR c.seller_id = $1
		 ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC
		 LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, listQuery, actorID, page.Size, page.Offset())
	if err != nil {
		return application.ConversationPage{}, fmt.Errorf("postgres: select conversations: %w", err)
	}
	defer rows.Close()

	items := make([]application.ConversationView, 0)
	for rows.Next() {
		var (
			conversation domain.Conversation
			lastID       *string
			lastSenderID *string
			lastContent  *string
			lastReadAt   *time.Time
			lastCreated  *time.Time
			unread       int64
		)
		if err := rows.Scan(
			&conversation.ID, &conversation.ProductID, &conversation.BuyerID, &conversation.SellerID,
			&conversation.CreatedAt, &conversation.LastMessageAt,
			&lastID, &lastSenderID, &lastContent, &lastReadAt, &lastCreated, &unread,
		); err != nil {
			return application.ConversationPage{}, fmt.Errorf("postgres: scan conversation: %w", err)
		}
		var last *domain.Message
		if lastID != nil {
			last = &domain.Message{
				ID: *lastID, ConversationID: conversation.ID, SenderID: *lastSenderID,
				Content: *lastContent, ReadAt: lastReadAt, CreatedAt: *lastCreated,
			}
		}
		items = append(items, application.ConversationView{
			Conversation: &conversation, LastMessage: last, UnreadCount: unread,
		})
	}
	if err := rows.Err(); err != nil {
		return application.ConversationPage{}, fmt.Errorf("postgres: read conversations: %w", err)
	}
	return application.ConversationPage{Items: items, Page: page.Number, Size: page.Size, Total: total}, nil
}

func (r *Repository) ListMessages(ctx context.Context, conversationID string, before *application.MessageCursor, limit int32) ([]*domain.Message, error) {
	query := `SELECT ` + messageColumns + ` FROM messages WHERE conversation_id = $1`
	args := []any{conversationID}
	if before != nil {
		query += ` AND (created_at < $2 OR (created_at = $2 AND id < $3))`
		args = append(args, before.CreatedAt, before.ID)
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: select messages: %w", err)
	}
	defer rows.Close()
	messages := make([]*domain.Message, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read messages: %w", err)
	}
	return messages, nil
}

func (r *Repository) UnreadCount(ctx context.Context, actorID string) (int64, error) {
	const query = `
		SELECT count(*)
		  FROM messages m
		  JOIN conversations c ON c.id = m.conversation_id
		 WHERE (c.buyer_id = $1 OR c.seller_id = $1)
		   AND m.sender_id <> $1
		   AND m.read_at IS NULL`
	var count int64
	if err := r.pool.QueryRow(ctx, query, actorID).Scan(&count); err != nil {
		return 0, fmt.Errorf("postgres: count unread messages: %w", err)
	}
	return count, nil
}

type messagingTx struct{ tx pgx.Tx }

func (t *messagingTx) GetOrCreateConversation(ctx context.Context, candidate *domain.Conversation) (*domain.Conversation, bool, error) {
	const insert = `
		INSERT INTO conversations (` + conversationColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (product_id, buyer_id, seller_id) DO NOTHING
		RETURNING ` + conversationColumns
	conversation, err := scanConversation(t.tx.QueryRow(ctx, insert,
		candidate.ID, candidate.ProductID, candidate.BuyerID, candidate.SellerID,
		candidate.CreatedAt, candidate.LastMessageAt,
	))
	if err == nil {
		return conversation, true, nil
	}
	if !isNotFound(err) {
		return nil, false, fmt.Errorf("postgres: insert conversation: %w", err)
	}

	const existing = `
		SELECT ` + conversationColumns + `
		  FROM conversations
		 WHERE product_id = $1 AND buyer_id = $2 AND seller_id = $3
		 FOR UPDATE`
	conversation, err = scanConversation(t.tx.QueryRow(ctx, existing,
		candidate.ProductID, candidate.BuyerID, candidate.SellerID,
	))
	if err != nil {
		return nil, false, fmt.Errorf("postgres: select existing conversation: %w", err)
	}
	return conversation, false, nil
}

func (t *messagingTx) ConversationView(ctx context.Context, conversationID, actorID string) (application.ConversationView, error) {
	const query = `
		SELECT c.id, c.product_id, c.buyer_id, c.seller_id, c.created_at, c.last_message_at,
		       lm.id, lm.sender_id, lm.content, lm.read_at, lm.created_at,
		       (SELECT count(*)
		          FROM messages unread
		         WHERE unread.conversation_id = c.id
		           AND unread.sender_id <> $2
		           AND unread.read_at IS NULL) AS unread_count
		  FROM conversations c
		  LEFT JOIN LATERAL (
		       SELECT id, sender_id, content, read_at, created_at
		         FROM messages
		        WHERE conversation_id = c.id
		        ORDER BY created_at DESC, id DESC
		        LIMIT 1
		  ) lm ON true
		 WHERE c.id = $1`
	var (
		conversation domain.Conversation
		lastID       *string
		lastSenderID *string
		lastContent  *string
		lastReadAt   *time.Time
		lastCreated  *time.Time
		unread       int64
	)
	if err := t.tx.QueryRow(ctx, query, conversationID, actorID).Scan(
		&conversation.ID, &conversation.ProductID, &conversation.BuyerID, &conversation.SellerID,
		&conversation.CreatedAt, &conversation.LastMessageAt,
		&lastID, &lastSenderID, &lastContent, &lastReadAt, &lastCreated, &unread,
	); err != nil {
		if pg.IsNoRows(err) {
			return application.ConversationView{}, application.ErrNotFound
		}
		return application.ConversationView{}, fmt.Errorf("postgres: read conversation view: %w", err)
	}
	var last *domain.Message
	if lastID != nil {
		last = &domain.Message{
			ID: *lastID, ConversationID: conversation.ID, SenderID: *lastSenderID,
			Content: *lastContent, ReadAt: lastReadAt, CreatedAt: *lastCreated,
		}
	}
	return application.ConversationView{
		Conversation: &conversation, LastMessage: last, UnreadCount: unread,
	}, nil
}

func (t *messagingTx) LockConversation(ctx context.Context, conversationID string) (*domain.Conversation, error) {
	conversation, err := scanConversation(t.tx.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM conversations WHERE id = $1 FOR UPDATE`, conversationID,
	))
	if err != nil {
		return nil, fmt.Errorf("postgres: lock conversation: %w", err)
	}
	return conversation, nil
}

func (t *messagingTx) LockMessage(ctx context.Context, conversationID, messageID string) (*domain.Message, error) {
	message, err := scanMessage(t.tx.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE conversation_id = $1 AND id = $2 FOR UPDATE`,
		conversationID, messageID,
	))
	if err != nil {
		return nil, fmt.Errorf("postgres: lock message: %w", err)
	}
	return message, nil
}

func (t *messagingTx) InsertMessage(ctx context.Context, message *domain.Message) error {
	const query = `
		INSERT INTO messages (` + messageColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := t.tx.Exec(ctx, query,
		message.ID, message.ConversationID, message.SenderID,
		message.Content, message.ReadAt, message.CreatedAt,
	); err != nil {
		return fmt.Errorf("postgres: insert message: %w", err)
	}
	return nil
}

func (t *messagingTx) TouchConversation(ctx context.Context, conversationID string, at time.Time) error {
	const query = `
		UPDATE conversations
		   SET last_message_at = CASE
		       WHEN last_message_at IS NULL OR last_message_at < $2 THEN $2
		       ELSE last_message_at END
		 WHERE id = $1`
	tag, err := t.tx.Exec(ctx, query, conversationID, at)
	if err != nil {
		return fmt.Errorf("postgres: touch conversation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (t *messagingTx) MarkOpponentMessagesRead(
	ctx context.Context,
	conversationID, actorID string,
	through time.Time,
	throughID string,
	at time.Time,
) (int64, error) {
	const query = `
		UPDATE messages
		   SET read_at = $5
		 WHERE conversation_id = $1
		   AND sender_id <> $2
		   AND read_at IS NULL
		   AND (created_at < $3 OR (created_at = $3 AND id <= $4))`
	tag, err := t.tx.Exec(ctx, query, conversationID, actorID, through, throughID, at)
	if err != nil {
		return 0, fmt.Errorf("postgres: mark messages read: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (t *messagingTx) AppendEvent(ctx context.Context, event application.Event) error {
	const query = `
		INSERT INTO outbox_events
		       (event_id, event_type, schema_version, aggregate_type, aggregate_id, occurred_at, trace_id, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	if _, err := t.tx.Exec(ctx, query,
		event.ID, event.Type, event.SchemaVersion, event.AggregateType,
		event.AggregateID, event.OccurredAt, event.TraceID, event.Payload,
	); err != nil {
		return fmt.Errorf("postgres: append outbox event: %w", err)
	}
	return nil
}

type storedResult struct {
	Code                    string              `json:"code"`
	Conversation            *storedConversation `json:"conversation,omitempty"`
	ConversationLastMessage *storedMessage      `json:"conversation_last_message,omitempty"`
	ConversationUnreadCount int64               `json:"conversation_unread_count,omitempty"`
	Message                 *storedMessage      `json:"message,omitempty"`
}

type storedConversation struct {
	ID            string     `json:"id"`
	ProductID     string     `json:"product_id"`
	BuyerID       string     `json:"buyer_id"`
	SellerID      string     `json:"seller_id"`
	CreatedAt     time.Time  `json:"created_at"`
	LastMessageAt *time.Time `json:"last_message_at"`
}

type storedMessage struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	SenderID       string     `json:"sender_id"`
	Content        string     `json:"content"`
	ReadAt         *time.Time `json:"read_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

func lockIdempotencyKey(ctx context.Context, tx pgx.Tx, key application.IdempotencyKey) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, $2)`,
		idempotencyLockNamespace, advisoryLockID(key),
	); err != nil {
		return fmt.Errorf("postgres: lock idempotency key: %w", err)
	}
	return nil
}

func advisoryLockID(key application.IdempotencyKey) int32 {
	digest := sha256.New()
	for _, part := range []string{key.ActorID, key.Operation, key.Key} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(part)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(part))
	}
	return int32(binary.BigEndian.Uint32(digest.Sum(nil)[:4]))
}

func readIdempotencyRecord(ctx context.Context, tx pgx.Tx, key application.IdempotencyKey) (*application.CommandResult, bool, error) {
	const query = `
		SELECT result_code, schema_version, result
		  FROM idempotency_records
		 WHERE actor_id = $1 AND operation = $2 AND idempotency_key = $3`
	var (
		code    string
		version int32
		payload []byte
	)
	if err := tx.QueryRow(ctx, query, key.ActorID, key.Operation, key.Key).Scan(&code, &version, &payload); err != nil {
		if pg.IsNoRows(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("postgres: read idempotency record: %w", err)
	}
	if version != application.SnapshotSchemaVersion {
		return nil, false, fmt.Errorf("postgres: unreadable idempotency snapshot version %d", version)
	}
	var stored storedResult
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, false, fmt.Errorf("postgres: decode idempotency snapshot: %w", err)
	}
	result := &application.CommandResult{Code: code, Message: stored.Message.toDomain()}
	if stored.Conversation != nil {
		result.ConversationView = &application.ConversationView{
			Conversation: stored.Conversation.toDomain(),
			LastMessage:  stored.ConversationLastMessage.toDomain(),
			UnreadCount:  stored.ConversationUnreadCount,
		}
	}
	return result, true, nil
}

func writeIdempotencyRecord(ctx context.Context, tx pgx.Tx, key application.IdempotencyKey, result *application.CommandResult) error {
	stored := storedResult{Code: result.Code, Message: fromDomainMessage(result.Message)}
	if result.ConversationView != nil {
		stored.Conversation = fromDomainConversation(result.ConversationView.Conversation)
		stored.ConversationLastMessage = fromDomainMessage(result.ConversationView.LastMessage)
		stored.ConversationUnreadCount = result.ConversationView.UnreadCount
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("postgres: encode idempotency snapshot: %w", err)
	}
	const query = `
		INSERT INTO idempotency_records
		       (actor_id, operation, idempotency_key, result_code, schema_version, result)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = tx.Exec(ctx, query,
		key.ActorID, key.Operation, key.Key, result.Code, application.SnapshotSchemaVersion, payload,
	)
	if pg.IsUniqueViolation(err, idempotencyPK) {
		return application.ErrIdempotencyRace
	}
	if err != nil {
		return fmt.Errorf("postgres: write idempotency record: %w", err)
	}
	return nil
}

func scanConversation(row pgx.Row) (*domain.Conversation, error) {
	var conversation domain.Conversation
	if err := row.Scan(
		&conversation.ID, &conversation.ProductID, &conversation.BuyerID, &conversation.SellerID,
		&conversation.CreatedAt, &conversation.LastMessageAt,
	); err != nil {
		if pg.IsNoRows(err) {
			return nil, application.ErrNotFound
		}
		return nil, err
	}
	return &conversation, nil
}

func scanMessage(row pgx.Row) (*domain.Message, error) {
	var message domain.Message
	if err := row.Scan(
		&message.ID, &message.ConversationID, &message.SenderID,
		&message.Content, &message.ReadAt, &message.CreatedAt,
	); err != nil {
		if pg.IsNoRows(err) {
			return nil, application.ErrNotFound
		}
		return nil, err
	}
	return &message, nil
}

func isNotFound(err error) bool { return err == application.ErrNotFound }

func fromDomainConversation(value *domain.Conversation) *storedConversation {
	if value == nil {
		return nil
	}
	return &storedConversation{
		ID: value.ID, ProductID: value.ProductID, BuyerID: value.BuyerID, SellerID: value.SellerID,
		CreatedAt: value.CreatedAt, LastMessageAt: value.LastMessageAt,
	}
}

func (s *storedConversation) toDomain() *domain.Conversation {
	if s == nil {
		return nil
	}
	return &domain.Conversation{
		ID: s.ID, ProductID: s.ProductID, BuyerID: s.BuyerID, SellerID: s.SellerID,
		CreatedAt: s.CreatedAt, LastMessageAt: s.LastMessageAt,
	}
}

func fromDomainMessage(value *domain.Message) *storedMessage {
	if value == nil {
		return nil
	}
	return &storedMessage{
		ID: value.ID, ConversationID: value.ConversationID, SenderID: value.SenderID,
		Content: value.Content, ReadAt: value.ReadAt, CreatedAt: value.CreatedAt,
	}
}

func (s *storedMessage) toDomain() *domain.Message {
	if s == nil {
		return nil
	}
	return &domain.Message{
		ID: s.ID, ConversationID: s.ConversationID, SenderID: s.SenderID,
		Content: s.Content, ReadAt: s.ReadAt, CreatedAt: s.CreatedAt,
	}
}
