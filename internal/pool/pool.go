// Package pool is a tiny keyed cache of live, shareable values with idle
// eviction. The API server uses it to hold one connected adapter per project
// instead of dialing (TCP + TLS + auth + Ping) and tearing down a fresh pool
// on every HTTP request. It is generic and dependency-free so both the
// adapter pool and future caches (e.g. policy sets) can reuse it.
package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned by Get once Close has been called.
var ErrClosed = errors.New("pool: closed")

type entry[V any] struct {
	value    V
	lastUsed time.Time
}

// Pool holds at most one live value per key. Values idle longer than ttl are
// evicted lazily on the next Get; Invalidate drops a key immediately (e.g.
// when the underlying credentials change). All methods are safe for
// concurrent use. Dial runs while holding the pool lock, so concurrent Gets
// for the same missing key dial exactly once — dial must therefore never call
// back into the pool.
type Pool[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]*entry[V]
	ttl     time.Duration
	dial    func(ctx context.Context, key K) (V, error)
	close   func(ctx context.Context, v V) error
	closed  bool

	hits   atomic.Int64
	misses atomic.Int64
}

// New returns a Pool that dials missing keys with dial, disposes evicted or
// invalidated values with close (which may be nil), and evicts values idle
// longer than ttl.
func New[K comparable, V any](
	ttl time.Duration,
	dial func(ctx context.Context, key K) (V, error),
	close func(ctx context.Context, v V) error,
) *Pool[K, V] {
	return &Pool[K, V]{entries: make(map[K]*entry[V]), ttl: ttl, dial: dial, close: close}
}

// Get returns the live value for key, dialing it on a miss. A dial error is
// returned unwrapped and nothing is cached.
func (p *Pool[K, V]) Get(ctx context.Context, key K) (V, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		var zero V
		return zero, ErrClosed
	}
	now := time.Now()
	if e, ok := p.entries[key]; ok && now.Sub(e.lastUsed) <= p.ttl {
		e.lastUsed = now
		p.hits.Add(1)
		return e.value, nil
	}
	// Lazy eviction: drop everything else that has gone idle while we hold
	// the lock. Entry counts are bounded by key cardinality (projects), so a
	// full sweep per miss is cheap.
	for k, e := range p.entries {
		if now.Sub(e.lastUsed) > p.ttl {
			delete(p.entries, k)
			if p.close != nil {
				_ = p.close(context.Background(), e.value)
			}
		}
	}
	p.misses.Add(1)
	v, err := p.dial(ctx, key)
	var zero V
	if err != nil {
		return zero, err
	}
	p.entries[key] = &entry[V]{value: v, lastUsed: now}
	return v, nil
}

// Invalidate drops key immediately, disposing its value. It is a no-op for
// unknown keys.
func (p *Pool[K, V]) Invalidate(key K) {
	p.mu.Lock()
	e, ok := p.entries[key]
	if ok {
		delete(p.entries, key)
	}
	p.mu.Unlock()
	if ok && p.close != nil {
		_ = p.close(context.Background(), e.value)
	}
}

// InvalidateWhere drops every entry whose key satisfies match, disposing their
// values. Callers that cannot reconstruct an exact key use this: the adapter
// pool keys on a credential generation, so a save/rotate does not know the
// outgoing key and invalidates by project instead. Returns the number dropped.
func (p *Pool[K, V]) InvalidateWhere(match func(K) bool) int {
	p.mu.Lock()
	var dropped []V
	for k, e := range p.entries {
		if match(k) {
			delete(p.entries, k)
			dropped = append(dropped, e.value)
		}
	}
	p.mu.Unlock()
	if p.close != nil {
		for _, v := range dropped {
			_ = p.close(context.Background(), v)
		}
	}
	return len(dropped)
}

// Len returns the number of live entries. For tests and /metrics.
func (p *Pool[K, V]) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

// Stats returns lifetime hit/miss counters. For tests and /metrics.
func (p *Pool[K, V]) Stats() (hits, misses int64) {
	return p.hits.Load(), p.misses.Load()
}

// Close shuts the pool down: no further dials, all live values disposed.
// Idempotent.
func (p *Pool[K, V]) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	entries := p.entries
	p.entries = make(map[K]*entry[V])
	p.mu.Unlock()
	if p.close != nil {
		for _, e := range entries {
			_ = p.close(context.Background(), e.value)
		}
	}
}
