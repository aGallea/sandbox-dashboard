package osb

import (
	"context"
	"maps"
	"sync"
	"time"
)

// Lister is the read side of the OpenSandbox sandbox inventory.
type Lister interface {
	ListSandboxes(ctx context.Context) (map[string]Sandbox, error)
}

// Cache is a TTL cache in front of a Lister.
//
// ponytail: one mutex held across the upstream fetch, not singleflight.
// Concurrent callers block on the same fetch, which is the same outcome for
// less machinery. If a slow OpenSandbox ever makes that blocking visible,
// swap in golang.org/x/sync/singleflight — it is already a direct dep.
type Cache struct {
	inner Lister
	ttl   time.Duration
	now   func() time.Time

	mu        sync.Mutex
	cached    map[string]Sandbox
	fetchedAt time.Time
}

// NewCache wraps inner with a TTL cache. A non-positive ttl disables caching.
// now is injected so tests need no sleeps; pass time.Now in production.
func NewCache(inner Lister, ttl time.Duration, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{inner: inner, ttl: ttl, now: now}
}

// ListSandboxes returns the cached inventory if it is younger than the TTL,
// otherwise it fetches from upstream. Errors are never cached. The returned
// map is the caller's own copy: mutating it never affects the cache or any
// other caller.
func (c *Cache) ListSandboxes(ctx context.Context) (map[string]Sandbox, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && c.ttl > 0 && c.now().Sub(c.fetchedAt) < c.ttl {
		return maps.Clone(c.cached), nil
	}
	fresh, err := c.inner.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	c.cached = fresh
	c.fetchedAt = c.now()
	return maps.Clone(fresh), nil
}
