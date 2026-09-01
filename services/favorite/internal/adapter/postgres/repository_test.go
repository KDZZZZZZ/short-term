package postgres_test

import (
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/application"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/domain"
	"github.com/KDZZZZZZ/short-term/services/favorite/migrations"
)

func newRepository(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	return postgres.NewRepository(pool), pool
}

func favorite(t *testing.T, userID, productID string, at time.Time) domain.Favorite {
	t.Helper()
	item, err := domain.New(userID, productID, at)
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	return item
}

func TestAddAndRemoveAreIdempotent(t *testing.T) {
	t.Parallel()

	repo, pool := newRepository(t)
	first := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)

	if err := repo.Add(t.Context(), favorite(t, "u_1", "p_1", first)); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := repo.Add(t.Context(), favorite(t, "u_1", "p_1", second)); err != nil {
		t.Fatalf("second Add: %v", err)
	}

	var storedAt time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT favorited_at FROM favorites WHERE user_id = $1 AND product_id = $2`, "u_1", "p_1",
	).Scan(&storedAt); err != nil {
		t.Fatalf("read favorite: %v", err)
	}
	if !storedAt.Equal(first) {
		t.Fatalf("favorited_at = %s, want original %s", storedAt, first)
	}

	for range 2 {
		if err := repo.Remove(t.Context(), "u_1", "p_1"); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	}
	favorited, err := repo.IsFavorited(t.Context(), "u_1", "p_1")
	if err != nil {
		t.Fatalf("IsFavorited: %v", err)
	}
	if favorited {
		t.Fatal("relationship still exists after removal")
	}
}

func TestConcurrentAddCreatesExactlyOneRelationship(t *testing.T) {
	t.Parallel()

	repo, pool := newRepository(t)
	const attempts = 12
	errors := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors[i] = repo.Add(t.Context(), favorite(t, "u_1", "p_1", time.Now().Add(time.Duration(i)*time.Millisecond)))
		}()
	}
	wg.Wait()
	for _, err := range errors {
		if err != nil {
			t.Fatalf("concurrent Add: %v", err)
		}
	}

	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM favorites WHERE user_id = $1 AND product_id = $2`, "u_1", "p_1",
	).Scan(&count); err != nil {
		t.Fatalf("count favorites: %v", err)
	}
	if count != 1 {
		t.Fatalf("stored rows = %d, want exactly 1", count)
	}
}

func TestListUsesStableOrderPaginationAndUserIsolation(t *testing.T) {
	t.Parallel()

	repo, _ := newRepository(t)
	base := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	for _, item := range []domain.Favorite{
		favorite(t, "u_1", "p_a", base),
		favorite(t, "u_1", "p_b", base),
		favorite(t, "u_1", "p_c", base.Add(time.Hour)),
		favorite(t, "u_2", "p_hidden", base.Add(2*time.Hour)),
	} {
		if err := repo.Add(t.Context(), item); err != nil {
			t.Fatalf("Add(%s): %v", item.ProductID, err)
		}
	}

	page, err := repo.List(t.Context(), "u_1", application.Page{Number: 1, Size: 2})
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if page.Total != 3 || page.Page != 1 || page.Size != 2 {
		t.Fatalf("page metadata = %+v", page)
	}
	if got := []string{page.Items[0].ProductID, page.Items[1].ProductID}; got[0] != "p_c" || got[1] != "p_b" {
		t.Fatalf("page 1 order = %v, want [p_c p_b]", got)
	}

	page, err = repo.List(t.Context(), "u_1", application.Page{Number: 2, Size: 2})
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ProductID != "p_a" {
		t.Fatalf("page 2 = %+v, want p_a", page.Items)
	}
}

func TestDatabaseRejectsMalformedCrossServiceReferences(t *testing.T) {
	t.Parallel()

	_, pool := newRepository(t)
	tests := []struct {
		name      string
		userID    string
		productID string
	}{
		{name: "empty user", productID: "p_1"},
		{name: "empty product", userID: "u_1"},
		{name: "long product", userID: "u_1", productID: "12345678901234567890123456789012345678901234567890123456789012345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := pool.Exec(t.Context(),
				`INSERT INTO favorites (user_id, product_id, favorited_at) VALUES ($1, $2, now())`,
				tt.userID, tt.productID,
			)
			if err == nil {
				t.Fatal("database accepted malformed reference")
			}
		})
	}
}
