package osb

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeLister struct {
	calls  atomic.Int32
	result map[string]Sandbox
	err    error
	block  chan struct{} // when non-nil, ListSandboxes waits on it
}

func (f *fakeLister) ListSandboxes(context.Context) (map[string]Sandbox, error) {
	f.calls.Add(1)
	if f.block != nil {
		<-f.block
	}
	return f.result, f.err
}

func TestCache_ServesFromCacheInsideTTL(t *testing.T) {
	inner := &fakeLister{result: map[string]Sandbox{"a": {ID: "a"}}}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	c := NewCache(inner, 5*time.Second, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		got, err := c.ListSandboxes(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)
	}
	require.Equal(t, int32(1), inner.calls.Load(), "three calls inside the TTL must hit upstream once")
}

func TestCache_RefetchesAfterTTLExpires(t *testing.T) {
	inner := &fakeLister{result: map[string]Sandbox{"a": {ID: "a"}}}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	c := NewCache(inner, 5*time.Second, func() time.Time { return now })

	_, err := c.ListSandboxes(context.Background())
	require.NoError(t, err)

	now = now.Add(6 * time.Second)
	_, err = c.ListSandboxes(context.Background())
	require.NoError(t, err)

	require.Equal(t, int32(2), inner.calls.Load())
}

func TestCache_ConcurrentCallersTriggerOneUpstreamFetch(t *testing.T) {
	inner := &fakeLister{result: map[string]Sandbox{"a": {ID: "a"}}, block: make(chan struct{})}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	c := NewCache(inner, 5*time.Second, func() time.Time { return now })

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.ListSandboxes(context.Background())
		}()
	}
	close(inner.block)
	wg.Wait()

	require.Equal(t, int32(1), inner.calls.Load(), "ten concurrent callers must share one upstream fetch")
}

func TestCache_DoesNotCacheErrors(t *testing.T) {
	inner := &fakeLister{err: errors.New("upstream down")}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	c := NewCache(inner, 5*time.Second, func() time.Time { return now })

	_, err := c.ListSandboxes(context.Background())
	require.Error(t, err)
	_, err = c.ListSandboxes(context.Background())
	require.Error(t, err)

	require.Equal(t, int32(2), inner.calls.Load(), "a failed fetch must be retried, not cached")
}

func TestCache_MutatingReturnedMapDoesNotAffectCache(t *testing.T) {
	inner := &fakeLister{result: map[string]Sandbox{"a": {ID: "a"}}}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	c := NewCache(inner, 5*time.Second, func() time.Time { return now })

	got, err := c.ListSandboxes(context.Background())
	require.NoError(t, err)
	got["b"] = Sandbox{ID: "b"}

	got2, err := c.ListSandboxes(context.Background())
	require.NoError(t, err)
	require.NotContains(t, got2, "b", "mutating a returned map must not leak into the cache or later callers")
}

func TestCache_NonPositiveTTLDisablesCaching(t *testing.T) {
	inner := &fakeLister{result: map[string]Sandbox{"a": {ID: "a"}}}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	c := NewCache(inner, 0, func() time.Time { return now })

	_, err := c.ListSandboxes(context.Background())
	require.NoError(t, err)
	_, err = c.ListSandboxes(context.Background())
	require.NoError(t, err)

	require.Equal(t, int32(2), inner.calls.Load(), "a non-positive TTL must disable caching")
}

func TestCache_CachesSuccessfulEmptyResult(t *testing.T) {
	inner := &fakeLister{result: map[string]Sandbox{}}
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	c := NewCache(inner, 5*time.Second, func() time.Time { return now })

	got, err := c.ListSandboxes(context.Background())
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = c.ListSandboxes(context.Background())
	require.NoError(t, err)

	require.Equal(t, int32(1), inner.calls.Load(), "an empty but successful fetch must still be cached")
}
