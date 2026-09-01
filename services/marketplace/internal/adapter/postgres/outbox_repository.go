package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
)

// OutboxRepository 是发布器访问 Outbox 表的仓储视图。
type OutboxRepository struct {
	pool *pgxpool.Pool
}

// NewOutboxRepository 基于已打开的连接池构造仓储。
func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

var _ application.OutboxRepository = (*OutboxRepository)(nil)

// Pending 按最早优先返回尚未发布的事件。
//
// 读取与发布之间没有跨进程 claim；当前日志发布器阶段只运行一个 worker。
// 如果误启动多个副本，它们可能同时读取并重复发布同一事件，但不会丢失事件，
// 仍符合至少一次语义。接入实际 Broker 时应随其交付模型增加租约/claim；无论
// 如何，消费者都必须按 event_id 去重。
func (r *OutboxRepository) Pending(ctx context.Context, limit int32) ([]application.Event, error) {
	const query = `
		SELECT event_id, event_type, schema_version, aggregate_type, aggregate_id, occurred_at, trace_id, payload
		  FROM outbox_events
		 WHERE published_at IS NULL
		 ORDER BY occurred_at, event_id
		 LIMIT $1`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: select pending events: %w", err)
	}
	defer rows.Close()

	var events []application.Event
	for rows.Next() {
		var event application.Event
		if err := rows.Scan(&event.ID, &event.Type, &event.SchemaVersion, &event.AggregateType,
			&event.AggregateID, &event.OccurredAt, &event.TraceID, &event.Payload); err != nil {
			return nil, fmt.Errorf("postgres: scan pending event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read pending events: %w", err)
	}
	return events, nil
}

// MarkPublished 记录已交付的事件。
func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID string, at time.Time) error {
	const query = `UPDATE outbox_events SET published_at = $2, last_error = NULL WHERE event_id = $1`

	if _, err := r.pool.Exec(ctx, query, eventID, at); err != nil {
		return fmt.Errorf("postgres: mark event published: %w", err)
	}
	return nil
}

// MarkFailed 记录失败尝试，并让事件保持待处理状态，以便重试且使运维人员能够看到失败。
func (r *OutboxRepository) MarkFailed(ctx context.Context, eventID, cause string) error {
	const query = `UPDATE outbox_events SET attempts = attempts + 1, last_error = $2 WHERE event_id = $1`

	if _, err := r.pool.Exec(ctx, query, eventID, truncate(cause, 500)); err != nil {
		return fmt.Errorf("postgres: mark event failed: %w", err)
	}
	return nil
}

// truncate 限制存储的错误消息长度。
func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
