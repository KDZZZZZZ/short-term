package id

import (
	"sort"
	"sync"
	"testing"
	"time"
)

func TestNewIsPrefixedAndParsable(t *testing.T) {
	t.Parallel()

	g := NewGenerator(nil)
	value := g.New(PrefixProduct)

	body, err := Parse(PrefixProduct, value)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", value, err)
	}
	if len(body) != encodedLen {
		t.Fatalf("body length = %d, want %d", len(body), encodedLen)
	}
	if _, err := Parse(PrefixTrade, value); err == nil {
		t.Fatal("Parse with the wrong prefix should fail")
	}
	if len(value) > 64 {
		t.Fatalf("identifier length %d exceeds the OpenAPI Identifier maximum", len(value))
	}
}

func TestNewOrdersByCreationWithinOneMillisecond(t *testing.T) {
	t.Parallel()

	frozen := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	g := NewGenerator(func() time.Time { return frozen })

	const count = 1000
	values := make([]string, count)
	for i := range values {
		values[i] = g.New(PrefixMessage)
	}

	if !sort.StringsAreSorted(values) {
		t.Fatal("identifiers minted in one millisecond are not lexically ordered")
	}
	unique := make(map[string]struct{}, count)
	for _, v := range values {
		unique[v] = struct{}{}
	}
	if len(unique) != count {
		t.Fatalf("got %d unique identifiers, want %d", len(unique), count)
	}
}

func TestNewOrdersAcrossMilliseconds(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	g := NewGenerator(func() time.Time { return now })

	first := g.New(PrefixTrade)
	now = now.Add(time.Millisecond)
	second := g.New(PrefixTrade)

	if first >= second {
		t.Fatalf("later identifier %q does not sort after %q", second, first)
	}
}

func TestNewIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	g := NewGenerator(nil)
	const workers = 16
	const perWorker = 128

	var mu sync.Mutex
	seen := make(map[string]struct{}, workers*perWorker)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				value := g.New(PrefixAccount)
				mu.Lock()
				seen[value] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != workers*perWorker {
		t.Fatalf("got %d unique identifiers, want %d", len(seen), workers*perWorker)
	}
}

func TestParseRejectsMalformedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "prefix only", value: "p_"},
		{name: "missing separator", value: "p01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{name: "too short", value: "p_01ARZ3NDEKTSV4RRFFQ69G5FA"},
		{name: "excluded letter U", value: "p_01ARZ3NDEKTSV4RRFFQ69G5FAU"},
		{name: "lowercase body", value: "p_01arz3ndektsv4rrffq69g5fav"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Parse(PrefixProduct, tt.value); err == nil {
				t.Fatalf("Parse(%q) should fail", tt.value)
			}
		})
	}
}
