package pool_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openbase/openbase/internal/pool"
)

type conn struct {
	id     int
	closed *atomic.Bool
}

func newHarness(ttl time.Duration) (*pool.Pool[string, *conn], *atomic.Int64, *atomic.Int64) {
	var dials, closes atomic.Int64
	p := pool.New(ttl,
		func(_ context.Context, key string) (*conn, error) {
			if key == "boom" {
				return nil, errors.New("dial refused")
			}
			return &conn{id: int(dials.Add(1)), closed: &atomic.Bool{}}, nil
		},
		func(_ context.Context, c *conn) error {
			closes.Add(1)
			c.closed.Store(true)
			return nil
		},
	)
	return p, &dials, &closes
}

func TestGetDialsOncePerKey(t *testing.T) {
	p, dials, _ := newHarness(time.Minute)
	defer p.Close()
	ctx := context.Background()

	first, err := p.Get(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		got, err := p.Get(ctx, "a")
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Fatalf("Get returned a different value on hit %d", i)
		}
	}
	if n := dials.Load(); n != 1 {
		t.Fatalf("dials = %d for 5 Gets of one key, want 1", n)
	}

	if _, err := p.Get(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if n := dials.Load(); n != 2 {
		t.Fatalf("dials = %d after a second key, want 2", n)
	}
	if p.Len() != 2 {
		t.Fatalf("Len = %d, want 2", p.Len())
	}

	hits, misses := p.Stats()
	if hits != 4 || misses != 2 {
		t.Fatalf("hits/misses = %d/%d, want 4/2", hits, misses)
	}
}

func TestDialErrorIsNotCached(t *testing.T) {
	p, _, _ := newHarness(time.Minute)
	defer p.Close()

	if _, err := p.Get(context.Background(), "boom"); err == nil {
		t.Fatal("expected dial error")
	}
	if p.Len() != 0 {
		t.Fatalf("Len = %d after a failed dial, want 0", p.Len())
	}
}

func TestIdleEviction(t *testing.T) {
	p, dials, closes := newHarness(20 * time.Millisecond)
	defer p.Close()
	ctx := context.Background()

	first, err := p.Get(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)

	second, err := p.Get(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("expected a re-dial after the idle window")
	}
	if n := dials.Load(); n != 2 {
		t.Fatalf("dials = %d, want 2", n)
	}
	// The evicted value must be disposed, not leaked.
	if !first.closed.Load() {
		t.Fatal("evicted value was not closed")
	}
	if n := closes.Load(); n != 1 {
		t.Fatalf("closes = %d, want 1", n)
	}
}

// A miss sweeps every other idle entry too, so one cold key does not leave
// stale live connections behind it.
func TestMissSweepsOtherIdleEntries(t *testing.T) {
	p, _, closes := newHarness(20 * time.Millisecond)
	defer p.Close()
	ctx := context.Background()

	if _, err := p.Get(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Get(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)

	if _, err := p.Get(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	if p.Len() != 1 {
		t.Fatalf("Len = %d after sweep, want 1 (only the fresh key)", p.Len())
	}
	if n := closes.Load(); n != 2 {
		t.Fatalf("closes = %d, want 2", n)
	}
}

func TestInvalidateDropsAndDisposes(t *testing.T) {
	p, dials, closes := newHarness(time.Minute)
	defer p.Close()
	ctx := context.Background()

	first, err := p.Get(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	p.Invalidate("a")
	if !first.closed.Load() {
		t.Fatal("invalidated value was not closed")
	}
	if n := closes.Load(); n != 1 {
		t.Fatalf("closes = %d, want 1", n)
	}

	second, err := p.Get(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("expected a fresh value after Invalidate")
	}
	if n := dials.Load(); n != 2 {
		t.Fatalf("dials = %d, want 2", n)
	}

	// Unknown keys are a no-op.
	p.Invalidate("nope")
}

func TestCloseDisposesAllAndRefusesFurtherGets(t *testing.T) {
	p, _, closes := newHarness(time.Minute)
	ctx := context.Background()

	for _, k := range []string{"a", "b", "c"} {
		if _, err := p.Get(ctx, k); err != nil {
			t.Fatal(err)
		}
	}
	p.Close()
	if n := closes.Load(); n != 3 {
		t.Fatalf("closes = %d, want 3", n)
	}
	if p.Len() != 0 {
		t.Fatalf("Len = %d after Close, want 0", p.Len())
	}
	if _, err := p.Get(ctx, "a"); !errors.Is(err, pool.ErrClosed) {
		t.Fatalf("Get after Close = %v, want ErrClosed", err)
	}

	// Idempotent: a second Close disposes nothing further.
	p.Close()
	if n := closes.Load(); n != 3 {
		t.Fatalf("closes = %d after second Close, want still 3", n)
	}
}

// Concurrent Gets for the same missing key must dial exactly once — the
// documented contract that lets callers treat a pooled value as shared.
func TestConcurrentGetsDialOnce(t *testing.T) {
	p, dials, _ := newHarness(time.Minute)
	defer p.Close()

	const n = 32
	var wg sync.WaitGroup
	got := make([]*conn, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			c, err := p.Get(context.Background(), "shared")
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = c
		}(i)
	}
	wg.Wait()

	if d := dials.Load(); d != 1 {
		t.Fatalf("dials = %d for %d concurrent Gets, want 1", d, n)
	}
	for i, c := range got {
		if c != got[0] {
			t.Fatalf("goroutine %d got a different value", i)
		}
	}
}
