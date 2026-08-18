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

// Cache is a TTL cache in front of a Lister. A failed fetch is cached for the
// TTL the same as a success: the mutex below is held across the upstream
// call, so if failures were retried on every request, a wedged OpenSandbox
// would turn every concurrent caller into its own multi-second stall instead
// of sharing one failed attempt. The caller still sees the error every time —
// only the retry is throttled, not the failure itself.
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
	cachedErr error
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
// otherwise it fetches from upstream. A failed fetch is cached for the TTL
// just like a success, and returned to every caller until it expires — see
// the Cache doc comment for why. The returned map is the caller's own copy:
// mutating it never affects the cache or any other caller.
func (c *Cache) ListSandboxes(ctx context.Context) (map[string]Sandbox, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.fetchedAt.IsZero() && c.ttl > 0 && c.now().Sub(c.fetchedAt) < c.ttl {
		if c.cachedErr != nil {
			return nil, c.cachedErr
		}
		return maps.Clone(c.cached), nil
	}
	fresh, err := c.inner.ListSandboxes(ctx)
	c.fetchedAt = c.now()
	c.cachedErr = err
	if err != nil {
		c.cached = nil
		return nil, err
	}
	c.cached = fresh
	return maps.Clone(fresh), nil
}
