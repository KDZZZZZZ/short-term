package postgres_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/marketplace/internal/application"
	"github.com/KDZZZZZZ/short-term/services/marketplace/migrations"
)

func newOutbox(t *testing.T) (*postgres.OutboxRepository, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	return postgres.NewOutboxRepository(pool), pool
}

func seedEvent(t *testing.T, pool *pgxpool.Pool, id, eventType string, at time.Time) {
	t.Helper()

	_, err := pool.Exec(t.Context(), `
		INSERT INTO outbox_events (event_id, event_type, schema_version, aggregate_type, aggregate_id, occurred_at, trace_id, payload)
		VALUES ($1, $2, 1, 'trade', 't_1', $3, '', $4)`,
		id, eventType, at, []byte(`{"trade_id":"t_1"}`))
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
}

func TestPendingReturnsUnpublishedEventsOldestFirst(t *testing.T) {
	t.Parallel()

	repo, pool := newOutbox(t)
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	seedEvent(t, pool, "evt_3", "c", base.Add(2*time.Minute))
	seedEvent(t, pool, "evt_1", "a", base)
	seedEvent(t, pool, "evt_2", "b", base.Add(time.Minute))

	events, err := repo.Pending(t.Context(), 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	for i, want := range []string{"evt_1", "evt_2", "evt_3"} {
		if events[i].ID != want {
			t.Fatalf("event %d = %s, want %s", i, events[i].ID, want)
		}
	}
	if events[0].SchemaVersion != 1 || len(events[0].Payload) == 0 {
		t.Fatalf("event fields were not read back: %+v", events[0])
	}
}

func TestPublishedEventsAreNotReturnedAgain(t *testing.T) {
	t.Parallel()

	repo, pool := newOutbox(t)
	seedEvent(t, pool, "evt_1", "a", time.Now().UTC())

	if err := repo.MarkPublished(t.Context(), "evt_1", time.Now().UTC()); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	events, err := repo.Pending(t.Context(), 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("a published event was returned again: %+v", events)
	}
}

func TestFailedEventsStayPendingAndRecordTheCause(t *testing.T) {
	t.Parallel()

	repo, pool := newOutbox(t)
	seedEvent(t, pool, "evt_1", "a", time.Now().UTC())

	if err := repo.MarkFailed(t.Context(), "evt_1", "broker unreachable"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	events, err := repo.Pending(t.Context(), 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("a failed event was dropped from the backlog: %+v", events)
	}

	var (
		attempts  int
		lastError string
	)
	if err := pool.QueryRow(t.Context(),
		`SELECT attempts, last_error FROM outbox_events WHERE event_id = $1`, "evt_1").Scan(&attempts, &lastError); err != nil {
		t.Fatalf("read attempt state: %v", err)
	}
	if attempts != 1 || lastError != "broker unreachable" {
		t.Fatalf("attempts/last_error = %d/%q", attempts, lastError)
	}
}

func TestDrainPublishesEveryPendingEvent(t *testing.T) {
	t.Parallel()

	repo, pool := newOutbox(t)
	for _, id := range []string{"evt_1", "evt_2", "evt_3"} {
		seedEvent(t, pool, id, "a", time.Now().UTC())
	}

	publisher := &recordingPublisher{}
	service, err := application.NewOutboxService(repo, publisher, 10, discardLogger())
	if err != nil {
		t.Fatalf("NewOutboxService: %v", err)
	}

	published, err := service.DrainOnce(t.Context())
	if err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	if published != 3 {
		t.Fatalf("published = %d, want 3", published)
	}

	// A second drain has nothing left, so a running worker does not republish
	// what it already delivered.
	again, err := service.DrainOnce(t.Context())
	if err != nil {
		t.Fatalf("second DrainOnce: %v", err)
	}
	if again != 0 {
		t.Fatalf("second drain published %d events, want 0", again)
	}
	if len(publisher.delivered) != 3 {
		t.Fatalf("publisher saw %d events, want 3", len(publisher.delivered))
	}
}

func TestOneUndeliverableEventDoesNotBlockTheRest(t *testing.T) {
	t.Parallel()

	repo, pool := newOutbox(t)
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	seedEvent(t, pool, "evt_bad", "a", base)
	seedEvent(t, pool, "evt_good", "b", base.Add(time.Minute))

	publisher := &recordingPublisher{failFor: "evt_bad"}
	service, err := application.NewOutboxService(repo, publisher, 10, discardLogger())
	if err != nil {
		t.Fatalf("NewOutboxService: %v", err)
	}

	published, err := service.DrainOnce(t.Context())
	if err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1", published)
	}

	// The failed event is still pending, so it is retried rather than lost.
	pending, err := repo.Pending(t.Context(), 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "evt_bad" {
		t.Fatalf("backlog = %+v, want only the failed event", pending)
	}
}

func TestRedeliveryIsPossibleBecauseThePublishHappensBeforeTheMark(t *testing.T) {
	t.Parallel()

	repo, pool := newOutbox(t)
	seedEvent(t, pool, "evt_1", "a", time.Now().UTC())

	publisher := &recordingPublisher{}
	service, err := application.NewOutboxService(repo, publisher, 10, discardLogger())
	if err != nil {
		t.Fatalf("NewOutboxService: %v", err)
	}
	if _, err := service.DrainOnce(t.Context()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}

	// Simulate a crash between the successful publish and the mark: the event
	// becomes pending again and is delivered a second time. Delivery is at
	// least once, which is why consumers deduplicate on event_id.
	if _, err := pool.Exec(t.Context(),
		`UPDATE outbox_events SET published_at = NULL WHERE event_id = $1`, "evt_1"); err != nil {
		t.Fatalf("reset published_at: %v", err)
	}
	if _, err := service.DrainOnce(t.Context()); err != nil {
		t.Fatalf("second DrainOnce: %v", err)
	}

	if len(publisher.delivered) != 2 {
		t.Fatalf("deliveries = %d, want 2", len(publisher.delivered))
	}
	if publisher.delivered[0] != publisher.delivered[1] {
		t.Fatal("the redelivery carried a different event id")
	}
}

// recordingPublisher records deliveries and can fail one event on purpose.
type recordingPublisher struct {
	delivered []string
	failFor   string
}

func (p *recordingPublisher) Publish(_ context.Context, event application.Event) error {
	if event.ID == p.failFor {
		return errors.New("broker unreachable")
	}
	p.delivered = append(p.delivered, event.ID)
	return nil
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
