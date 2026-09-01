package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func newTestProduct(t *testing.T) *Product {
	t.Helper()

	product, err := NewProduct("p_1", "u_seller", "机械键盘", 12000, CategoryDigital, "九成新", now)
	if err != nil {
		t.Fatalf("NewProduct: %v", err)
	}
	return product
}

func TestNewProductStartsOnSale(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)

	if product.Status != StatusOnSale {
		t.Fatalf("Status = %s, want ON_SALE", product.Status)
	}
	if product.Version != 1 {
		t.Fatalf("Version = %d, want 1", product.Version)
	}
	if !product.Tradable() {
		t.Fatal("a newly published product must be tradable")
	}
}

func TestNewProductRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          string
		sellerID    string
		title       string
		price       int64
		category    Category
		description string
		wantErr     error
	}{
		{name: "no id", sellerID: "u_1", title: "t", price: 1, category: CategoryOther, description: "d", wantErr: ErrIDRequired},
		{name: "no seller", id: "p_1", title: "t", price: 1, category: CategoryOther, description: "d", wantErr: ErrSellerRequired},
		{name: "empty title", id: "p_1", sellerID: "u_1", title: "", price: 1, category: CategoryOther, description: "d", wantErr: ErrTitleLength},
		{name: "title too long", id: "p_1", sellerID: "u_1", title: strings.Repeat("字", 101), price: 1, category: CategoryOther, description: "d", wantErr: ErrTitleLength},
		{name: "negative price", id: "p_1", sellerID: "u_1", title: "t", price: -1, category: CategoryOther, description: "d", wantErr: ErrPriceRange},
		{name: "price above the contract maximum", id: "p_1", sellerID: "u_1", title: "t", price: MaxPriceMinor + 1, category: CategoryOther, description: "d", wantErr: ErrPriceRange},
		{name: "empty description", id: "p_1", sellerID: "u_1", title: "t", price: 1, category: CategoryOther, description: "", wantErr: ErrDescriptionLength},
		{name: "description too long", id: "p_1", sellerID: "u_1", title: "t", price: 1, category: CategoryOther, description: strings.Repeat("字", 2001), wantErr: ErrDescriptionLength},
		{name: "unknown category", id: "p_1", sellerID: "u_1", title: "t", price: 1, category: "BOOKS", description: "d", wantErr: ErrCategoryUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewProduct(tt.id, tt.sellerID, tt.title, tt.price, tt.category, tt.description, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewProduct error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTitleAndDescriptionLimitsCountRunesNotBytes(t *testing.T) {
	t.Parallel()

	// 100 个中文字符的标题占 300 字节。按字节计数会错误拒绝公开契约允许的标题。
	if err := ValidateTitle(strings.Repeat("字", 100)); err != nil {
		t.Fatalf("ValidateTitle(100 runes) = %v, want nil", err)
	}
	if err := ValidateDescription(strings.Repeat("字", 2000)); err != nil {
		t.Fatalf("ValidateDescription(2000 runes) = %v, want nil", err)
	}
}

func TestOnlyTheSellerMayActOnAProduct(t *testing.T) {
	t.Parallel()

	title := "新标题"

	tests := []struct {
		name string
		act  func(*Product) error
	}{
		{name: "edit", act: func(p *Product) error { return p.Edit("u_other", &title, nil, nil, nil, now) }},
		{name: "off shelf", act: func(p *Product) error { return p.OffShelf("u_other", now) }},
		{name: "relist", act: func(p *Product) error { return p.Relist("u_other", now) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			product := newTestProduct(t)
			if err := tt.act(product); !errors.Is(err, ErrNotSeller) {
				t.Fatalf("%s by a non-seller = %v, want ErrNotSeller", tt.name, err)
			}
		})
	}
}

func TestOffShelfAndRelistRoundTrip(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)

	if err := product.OffShelf("u_seller", now); err != nil {
		t.Fatalf("OffShelf: %v", err)
	}
	if product.Status != StatusOffShelf {
		t.Fatalf("Status = %s, want OFF_SHELF", product.Status)
	}
	if product.Tradable() {
		t.Fatal("an off-shelf product must not be tradable")
	}
	if err := product.OffShelf("u_seller", now); !errors.Is(err, ErrNotOnSale) {
		t.Fatalf("a second OffShelf = %v, want ErrNotOnSale", err)
	}

	if err := product.Relist("u_seller", now); err != nil {
		t.Fatalf("Relist: %v", err)
	}
	if product.Status != StatusOnSale {
		t.Fatalf("Status = %s, want ON_SALE", product.Status)
	}
	if err := product.Relist("u_seller", now); !errors.Is(err, ErrNotOffShelf) {
		t.Fatalf("a second Relist = %v, want ErrNotOffShelf", err)
	}
}

func TestTradeDrivenTransitions(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)

	if err := product.Reserve(now); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if product.Status != StatusReserved {
		t.Fatalf("Status = %s, want RESERVED", product.Status)
	}
	if product.Tradable() {
		t.Fatal("a reserved product must not accept a new trade")
	}

	if err := product.Release(now); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if product.Status != StatusOnSale {
		t.Fatalf("Status = %s, want ON_SALE after release", product.Status)
	}

	if err := product.Reserve(now); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := product.MarkSold(now); err != nil {
		t.Fatalf("MarkSold: %v", err)
	}
	if product.Status != StatusSold {
		t.Fatalf("Status = %s, want SOLD", product.Status)
	}
}

func TestForbiddenTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		from    Status
		act     func(*Product) error
		wantErr error
	}{
		{name: "reserve a sold product", from: StatusSold, act: func(p *Product) error { return p.Reserve(now) }, wantErr: ErrNotOnSale},
		{name: "reserve an off-shelf product", from: StatusOffShelf, act: func(p *Product) error { return p.Reserve(now) }, wantErr: ErrNotOnSale},
		{name: "sell an on-sale product directly", from: StatusOnSale, act: func(p *Product) error { return p.MarkSold(now) }, wantErr: ErrNotReserved},
		{name: "sell a sold product again", from: StatusSold, act: func(p *Product) error { return p.MarkSold(now) }, wantErr: ErrNotReserved},
		{name: "release an on-sale product", from: StatusOnSale, act: func(p *Product) error { return p.Release(now) }, wantErr: ErrNotReserved},
		{name: "off shelf a reserved product", from: StatusReserved, act: func(p *Product) error { return p.OffShelf("u_seller", now) }, wantErr: ErrNotOnSale},
		{name: "off shelf a sold product", from: StatusSold, act: func(p *Product) error { return p.OffShelf("u_seller", now) }, wantErr: ErrNotOnSale},
		{name: "relist a sold product", from: StatusSold, act: func(p *Product) error { return p.Relist("u_seller", now) }, wantErr: ErrNotOffShelf},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			product := newTestProduct(t)
			product.Status = tt.from
			before := product.Status

			if err := tt.act(product); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if product.Status != before {
				t.Fatalf("a rejected transition changed the status to %s", product.Status)
			}
		})
	}
}

func TestEditIsRejectedWhileAProductIsInATrade(t *testing.T) {
	t.Parallel()

	title := "改个标题"

	for _, status := range []Status{StatusReserved, StatusSold} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			product := newTestProduct(t)
			product.Status = status

			if err := product.Edit("u_seller", &title, nil, nil, nil, now); !errors.Is(err, ErrNotEditable) {
				t.Fatalf("Edit on a %s product = %v, want ErrNotEditable", status, err)
			}
			if product.Title == title {
				t.Fatal("a rejected edit still changed the title")
			}
		})
	}
}

func TestEditAppliesOnlyTheSuppliedFieldsAndBumpsVersion(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)
	price := int64(9900)
	before := product.Version

	if err := product.Edit("u_seller", nil, &price, nil, nil, now.Add(time.Hour)); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	if product.PriceMinor != price {
		t.Fatalf("PriceMinor = %d, want %d", product.PriceMinor, price)
	}
	if product.Title != "机械键盘" {
		t.Fatalf("an absent title changed to %q", product.Title)
	}
	if product.Version != before+1 {
		t.Fatalf("Version = %d, want %d", product.Version, before+1)
	}
	if !product.UpdatedAt.After(product.CreatedAt) {
		t.Fatal("UpdatedAt did not move")
	}
}

func TestEditRejectsInvalidValuesWithoutPartialWrites(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)
	goodTitle := "新标题"
	badPrice := int64(-1)

	err := product.Edit("u_seller", &goodTitle, &badPrice, nil, nil, now)
	if !errors.Is(err, ErrPriceRange) {
		t.Fatalf("Edit = %v, want ErrPriceRange", err)
	}
	if product.Title == goodTitle {
		t.Fatal("a rejected edit applied the valid field anyway")
	}
	if product.Version != 1 {
		t.Fatalf("Version = %d, want it unchanged after a rejected edit", product.Version)
	}
}

func TestImageSlotAllocation(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)

	for want := 1; want <= MaxImages; want++ {
		slot, err := product.NextImageSlot()
		if err != nil {
			t.Fatalf("NextImageSlot: %v", err)
		}
		if slot != want {
			t.Fatalf("slot = %d, want %d", slot, want)
		}
		product.Images = append(product.Images, Image{ID: "img", SortOrder: slot})
	}

	if _, err := product.NextImageSlot(); !errors.Is(err, ErrImageLimitExceeded) {
		t.Fatalf("NextImageSlot on a full product = %v, want ErrImageLimitExceeded", err)
	}
}

func TestNextImageSlotReusesAFreedSlot(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)
	product.Images = []Image{{SortOrder: 1}, {SortOrder: 3}}

	slot, err := product.NextImageSlot()
	if err != nil {
		t.Fatalf("NextImageSlot: %v", err)
	}
	if slot != 2 {
		t.Fatalf("slot = %d, want the freed slot 2", slot)
	}
}

func TestCoverIsTheLowestOrderedImage(t *testing.T) {
	t.Parallel()

	product := newTestProduct(t)
	if product.CoverObjectKey() != "" {
		t.Fatal("a product without images must have no cover")
	}

	product.Images = []Image{
		{ObjectKey: "third", SortOrder: 3},
		{ObjectKey: "first", SortOrder: 1},
	}
	if got := product.CoverObjectKey(); got != "first" {
		t.Fatalf("CoverObjectKey = %q, want first", got)
	}
}
