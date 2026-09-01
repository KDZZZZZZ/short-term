package application_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/KDZZZZZZ/short-term/platform/errs"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/application"
	"github.com/KDZZZZZZ/short-term/services/favorite/internal/domain"
)

var fixedTime = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeCatalog struct {
	product application.Product
	err     error
	calls   int
}

func (f *fakeCatalog) Get(_ context.Context, _, _ string) (application.Product, error) {
	f.calls++
	return f.product, f.err
}

type memoryRepository struct {
	mu          sync.Mutex
	items       map[string]domain.Favorite
	addCalls    int
	removeCalls int
	err         error
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{items: make(map[string]domain.Favorite)}
}

func favoriteKey(userID, productID string) string { return userID + "\x00" + productID }

func (r *memoryRepository) Add(_ context.Context, favorite domain.Favorite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addCalls++
	if r.err != nil {
		return r.err
	}
	key := favoriteKey(favorite.UserID, favorite.ProductID)
	if _, exists := r.items[key]; !exists {
		r.items[key] = favorite
	}
	return nil
}

func (r *memoryRepository) Remove(_ context.Context, userID, productID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeCalls++
	if r.err != nil {
		return r.err
	}
	delete(r.items, favoriteKey(userID, productID))
	return nil
}

func (r *memoryRepository) List(_ context.Context, userID string, page application.Page) (application.FavoritePage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return application.FavoritePage{}, r.err
	}
	var items []domain.Favorite
	for _, item := range r.items {
		if item.UserID == userID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].FavoritedAt.Equal(items[j].FavoritedAt) {
			return items[i].ProductID > items[j].ProductID
		}
		return items[i].FavoritedAt.After(items[j].FavoritedAt)
	})
	return application.FavoritePage{Items: items, Page: page.Number, Size: page.Size, Total: int64(len(items))}, nil
}

func (r *memoryRepository) IsFavorited(_ context.Context, userID, productID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return false, r.err
	}
	_, exists := r.items[favoriteKey(userID, productID)]
	return exists, nil
}

func newService(t *testing.T, repo *memoryRepository, catalog *fakeCatalog) *application.Service {
	t.Helper()
	service, err := application.NewService(repo, catalog, fakeClock{now: fixedTime})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func TestAddIsIdempotentAndKeepsOriginalTimestamp(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepository()
	catalog := &fakeCatalog{product: application.Product{ID: "p_1", SellerID: "u_seller"}}
	service := newService(t, repo, catalog)
	cmd := application.FavoriteCommand{ActorID: "u_buyer", ProductID: "p_1"}

	if err := service.Add(t.Context(), cmd); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := service.Add(t.Context(), cmd); err != nil {
		t.Fatalf("second Add: %v", err)
	}

	if len(repo.items) != 1 || repo.addCalls != 2 {
		t.Fatalf("stored = %d, add calls = %d; want one relationship after two PUTs", len(repo.items), repo.addCalls)
	}
	if got := repo.items[favoriteKey("u_buyer", "p_1")].FavoritedAt; !got.Equal(fixedTime) {
		t.Fatalf("FavoritedAt = %s, want %s", got, fixedTime)
	}
	if catalog.calls != 2 {
		t.Fatalf("Marketplace calls = %d, want each add to validate the current product", catalog.calls)
	}
}

func TestAddAllowsEveryExistingProductStatusByNotDependingOnStatus(t *testing.T) {
	t.Parallel()

	// ProductCatalog intentionally exposes no status field. This prevents
	// Favorite application logic from rejecting OFF_SHELF, RESERVED or SOLD.
	repo := newMemoryRepository()
	service := newService(t, repo, &fakeCatalog{product: application.Product{ID: "p_1", SellerID: "u_seller"}})
	if err := service.Add(t.Context(), application.FavoriteCommand{ActorID: "u_buyer", ProductID: "p_1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
}

func TestAddRejectsSelfFavoriteBeforePersistence(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepository()
	service := newService(t, repo, &fakeCatalog{product: application.Product{ID: "p_1", SellerID: "u_owner"}})
	err := service.Add(t.Context(), application.FavoriteCommand{ActorID: "u_owner", ProductID: "p_1"})
	if errs.CodeOf(err) != errs.CodeSelfActionNotAllowed {
		t.Fatalf("Add = %v, want SELF_ACTION_NOT_ALLOWED", err)
	}
	if repo.addCalls != 0 {
		t.Fatal("self favorite reached persistence")
	}
}

func TestAddPreservesProductNotFoundAndHidesOtherDependencyErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want errs.Code
	}{
		{name: "missing", err: errs.New(errs.CodeResourceNotFound, "missing"), want: errs.CodeResourceNotFound},
		{name: "dependency", err: errors.New("dial secret"), want: errs.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service := newService(t, newMemoryRepository(), &fakeCatalog{err: tt.err})
			err := service.Add(t.Context(), application.FavoriteCommand{ActorID: "u_1", ProductID: "p_1"})
			if errs.CodeOf(err) != tt.want {
				t.Fatalf("Add = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestRemoveIsIdempotentAndDoesNotReadMarketplace(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepository()
	catalog := &fakeCatalog{}
	service := newService(t, repo, catalog)
	cmd := application.FavoriteCommand{ActorID: "u_1", ProductID: "p_missing"}
	if err := service.Remove(t.Context(), cmd); err != nil {
		t.Fatalf("first Remove: %v", err)
	}
	if err := service.Remove(t.Context(), cmd); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
	if repo.removeCalls != 2 || catalog.calls != 0 {
		t.Fatalf("remove calls = %d, catalog calls = %d", repo.removeCalls, catalog.calls)
	}
}

func TestListAppliesProtoDefaults(t *testing.T) {
	t.Parallel()

	service := newService(t, newMemoryRepository(), &fakeCatalog{})
	page, err := service.List(t.Context(), application.ListFavoritesQuery{ActorID: "u_1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Page != 1 || page.Size != application.DefaultPageSize {
		t.Fatalf("page = %+v, want defaults", page)
	}
}

func TestCommandsRequireActorAndValidProduct(t *testing.T) {
	t.Parallel()

	service := newService(t, newMemoryRepository(), &fakeCatalog{})
	if _, err := service.IsFavorited(t.Context(), application.FavoriteCommand{ProductID: "p_1"}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("missing actor = %v, want UNAUTHORIZED", err)
	}
	if err := service.Remove(t.Context(), application.FavoriteCommand{ActorID: "u_1"}); errs.CodeOf(err) != errs.CodeValidation {
		t.Fatalf("missing product = %v, want VALIDATION_ERROR", err)
	}
}
