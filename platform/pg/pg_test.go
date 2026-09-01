package pg_test

import (
	"context"
	"embed"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/KDZZZZZZ/short-term/platform/pg"
	"github.com/KDZZZZZZ/short-term/platform/pgtest"
)

//go:embed testdata/migrations/*.sql
var migrations embed.FS

func TestMigrateAppliesEveryVersion(t *testing.T) {
	t.Parallel()

	pool := pgtest.New(t, migrations, "testdata/migrations")

	var version int
	if err := pool.QueryRow(t.Context(), `SELECT version FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 2 {
		t.Fatalf("schema version = %d, want 2", version)
	}

	// 第二个迁移会添加此列，因此查询该列可以证明完整迁移序列已运行，
	// 而不只是运行了第一个文件。
	if _, err := pool.Exec(t.Context(), `INSERT INTO widgets (id, name, notes) VALUES ($1, $2, $3)`, "w1", "first", "note"); err != nil {
		t.Fatalf("insert into migrated table: %v", err)
	}
}

func TestInTxCommitsOnSuccess(t *testing.T) {
	t.Parallel()

	pool := pgtest.New(t, migrations, "testdata/migrations")

	err := pg.InTx(t.Context(), pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO widgets (id, name) VALUES ($1, $2)`, "w1", "kept")
		return err
	})
	if err != nil {
		t.Fatalf("InTx = %v, want nil", err)
	}

	if got := countWidgets(t, pool); got != 1 {
		t.Fatalf("row count = %d, want 1", got)
	}
}

func TestInTxRollsBackEveryWriteOnFailure(t *testing.T) {
	t.Parallel()

	pool := pgtest.New(t, migrations, "testdata/migrations")
	sentinel := errors.New("second step failed")

	err := pg.InTx(t.Context(), pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO widgets (id, name) VALUES ($1, $2)`, "w1", "first"); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("InTx = %v, want the sentinel error", err)
	}

	if got := countWidgets(t, pool); got != 0 {
		t.Fatalf("row count = %d, want 0: the first write was not rolled back", got)
	}
}

func TestInTxRollsBackWhenTheRequestIsCancelled(t *testing.T) {
	t.Parallel()

	pool := pgtest.New(t, migrations, "testdata/migrations")
	ctx, cancel := context.WithCancel(t.Context())

	err := pg.InTx(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO widgets (id, name) VALUES ($1, $2)`, "w1", "first"); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InTx = %v, want context.Canceled", err)
	}

	if got := countWidgets(t, pool); got != 0 {
		t.Fatalf("row count = %d, want 0: a cancelled request left its write behind", got)
	}
}

func TestIsUniqueViolationIdentifiesTheConstraint(t *testing.T) {
	t.Parallel()

	pool := pgtest.New(t, migrations, "testdata/migrations")

	if _, err := pool.Exec(t.Context(), `INSERT INTO widgets (id, name) VALUES ($1, $2)`, "w1", "same"); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO widgets (id, name) VALUES ($1, $2)`, "w2", "same")

	if !pg.IsUniqueViolation(err, "widgets_name_key") {
		t.Fatalf("IsUniqueViolation(err, widgets_name_key) = false for %v", err)
	}
	if pg.IsUniqueViolation(err, "widgets_pkey") {
		t.Fatal("IsUniqueViolation matched an unrelated constraint")
	}
	if pg.IsUniqueViolation(errors.New("boom"), "") {
		t.Fatal("IsUniqueViolation matched a non-database error")
	}
}

func TestConcurrentTransactionsSerialiseOnTheUniqueIndex(t *testing.T) {
	t.Parallel()

	pool := pgtest.New(t, migrations, "testdata/migrations")

	const attempts = 8
	var wg sync.WaitGroup
	results := make([]error, attempts)

	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = pg.InTx(t.Context(), pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
				_, err := tx.Exec(t.Context(), `INSERT INTO widgets (id, name) VALUES ($1, $2)`, "w"+string(rune('a'+i)), "contended")
				return err
			})
		}()
	}
	wg.Wait()

	var succeeded int
	for _, err := range results {
		switch {
		case err == nil:
			succeeded++
		case pg.IsUniqueViolation(err, "widgets_name_key"):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d transactions committed, want exactly 1", succeeded)
	}
	if got := countWidgets(t, pool); got != 1 {
		t.Fatalf("row count = %d, want 1", got)
	}
}

func countWidgets(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
},
) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM widgets`).Scan(&count); err != nil {
		t.Fatalf("count widgets: %v", err)
	}
	return count
}
