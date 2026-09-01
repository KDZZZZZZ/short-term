package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/services/messaging/internal/application"
)

type OutboxRepository struct{ pool *pgxpool.Pool }

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

var _ application.OutboxRepository = (*OutboxRepository)(nil)

func (r *OutboxRepository) Pending(ctx context.Context, limit int32) ([]application.Event, error) {
	const query = `
		SELECT event_id, event_type, schema_version, aggregate_type, aggregate_id, occurred_at, trace_id, payload
		  FROM outbox_events
		 WHERE published_at IS NULL
		 ORDER BY occurred_at, event_id
		 LIMIT $1`
	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: select messaging outbox: %w", err)
	}
	defer rows.Close()
	events := make([]application.Event, 0)
	for rows.Next() {
		var event application.Event
		if err := rows.Scan(
			&event.ID, &event.Type, &event.SchemaVersion, &event.AggregateType,
			&event.AggregateID, &event.OccurredAt, &event.TraceID, &event.Payload,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan messaging outbox: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read messaging outbox: %w", err)
	}
	return events, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID string, at time.Time) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE outbox_events SET published_at = $2, last_error = NULL WHERE event_id = $1`, eventID, at,
	); err != nil {
		return fmt.Errorf("postgres: publish messaging outbox: %w", err)
	}
	return nil
}

func (r *OutboxRepository) MarkFailed(ctx context.Context, eventID, cause string) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE outbox_events SET attempts = attempts + 1, last_error = $2 WHERE event_id = $1`,
		eventID, truncate(cause, 500),
	); err != nil {
		return fmt.Errorf("postgres: fail messaging outbox: %w", err)
	}
	return nil
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
