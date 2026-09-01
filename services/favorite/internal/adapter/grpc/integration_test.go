package grpc_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	favoritev1 "github.com/KDZZZZZZ/short-term/gen/go/shortterm/favorite/v1"
	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/platform/grpcx"
	"github.com/KDZZZZZZ/short-term/platform/pgtest"
	grpcadapter "github.com/KDZZZZZZ/short-term/services/favorite/internal/adapter/grpc"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/adapter/postgres"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/application"
	"github.com/KDZZZZZZ/short-term/services/favorite/migrations"
)

type integrationCatalog struct {
	products map[string]application.Product
}

func (c integrationCatalog) Get(_ context.Context, _, productID string) (application.Product, error) {
	product, ok := c.products[productID]
	if !ok {
		return application.Product{}, errs.New(errs.CodeResourceNotFound, "商品不存在")
	}
	return product, nil
}

type integrationHarness struct {
	client favoritev1.FavoriteServiceClient
	pool   *pgxpool.Pool
}

func newIntegrationHarness(t *testing.T, products map[string]application.Product) integrationHarness {
	t.Helper()
	pool := pgtest.New(t, migrations.FS, migrations.Dir)
	app, err := application.NewService(
		postgres.NewRepository(pool), integrationCatalog{products: products},
		clock{at: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpcx.NewServer(grpcx.ServerOptions{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), HandlerTimeout: 10 * time.Second,
	})
	favoritev1.RegisterFavoriteServiceServer(server, grpcadapter.NewServer(app))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpcx.Dial(grpcx.ClientOptions{
		Target: listener.Addr().String(), Caller: "favorite-integration-test", DefaultTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return integrationHarness{client: favoritev1.NewFavoriteServiceClient(conn), pool: pool}
}

func TestFavoriteGRPCDatabaseFlow(t *testing.T) {
	t.Parallel()

	h := newIntegrationHarness(t, map[string]application.Product{
		"p_1": {ID: "p_1", SellerID: "u_seller"},
	})
	ctx := grpcx.WithActor(t.Context(), "u_buyer")
	for range 2 {
		if _, err := h.client.AddFavorite(ctx, &favoritev1.AddFavoriteRequest{ActorId: "u_buyer", ProductId: "p_1"}); err != nil {
			t.Fatalf("AddFavorite: %v", err)
		}
	}

	page, err := h.client.ListFavorites(ctx, &favoritev1.ListFavoritesRequest{ActorId: "u_buyer", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListFavorites: %v", err)
	}
	if page.GetPage().GetTotal() != 1 || len(page.GetPage().GetItems()) != 1 {
		t.Fatalf("page = %+v, want one relationship", page.GetPage())
	}

	for range 2 {
		if _, err := h.client.RemoveFavorite(ctx, &favoritev1.RemoveFavoriteRequest{ActorId: "u_buyer", ProductId: "p_1"}); err != nil {
			t.Fatalf("RemoveFavorite: %v", err)
		}
	}
	check, err := h.client.IsFavorited(ctx, &favoritev1.IsFavoritedRequest{ActorId: "u_buyer", ProductId: "p_1"})
	if err != nil || check.GetFavorited() {
		t.Fatalf("IsFavorited = %+v, %v; want false", check, err)
	}
}

func TestConcurrentFavoriteRPCAddsOneRow(t *testing.T) {
	t.Parallel()

	h := newIntegrationHarness(t, map[string]application.Product{
		"p_1": {ID: "p_1", SellerID: "u_seller"},
	})
	const attempts = 12
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = h.client.AddFavorite(grpcx.WithActor(t.Context(), "u_buyer"), &favoritev1.AddFavoriteRequest{
				ActorId: "u_buyer", ProductId: "p_1",
			})
		}()
	}
	wg.Wait()
	for _, err := range results {
		if err != nil {
			t.Fatalf("concurrent AddFavorite: %v", err)
		}
	}

	var count int
	if err := h.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM favorites WHERE user_id = $1 AND product_id = $2`, "u_buyer", "p_1",
	).Scan(&count); err != nil {
		t.Fatalf("count favorites: %v", err)
	}
	if count != 1 {
		t.Fatalf("stored rows = %d, want exactly 1", count)
	}
}

func TestSelfFavoriteRejectedOverGRPCWithoutDatabaseWrite(t *testing.T) {
	t.Parallel()

	h := newIntegrationHarness(t, map[string]application.Product{
		"p_own": {ID: "p_own", SellerID: "u_owner"},
	})
	_, err := h.client.AddFavorite(grpcx.WithActor(t.Context(), "u_owner"), &favoritev1.AddFavoriteRequest{
		ActorId: "u_owner", ProductId: "p_own",
	})
	if errs.CodeOf(err) != errs.CodeSelfActionNotAllowed {
		t.Fatalf("AddFavorite = %v, want SELF_ACTION_NOT_ALLOWED", err)
	}

	var count int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM favorites`).Scan(&count); err != nil {
		t.Fatalf("count favorites: %v", err)
	}
	if count != 0 {
		t.Fatalf("stored rows = %d, want zero", count)
	}
}
