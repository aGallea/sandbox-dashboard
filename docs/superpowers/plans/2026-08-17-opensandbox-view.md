# OpenSandbox View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show OpenSandbox's own lifecycle state alongside the Kubernetes Ready condition for every sandbox, so a sandbox that is `Ready` to Kubernetes and `Pending` to OpenSandbox is visible as such.

**Architecture:** A new `internal/osb` package calls the OpenSandbox REST API and is joined onto the existing `Sandbox` CR list server-side, keyed on the `opensandbox.io/id` label. OSB is an optional dependency modelled exactly like the existing Prometheus integration: unset means the columns are absent, unreachable means the list still serves CR data with a status marker. Two independent signals are computed server-side — `diverged` (the two control planes disagree) and `stale` (OSB has not moved off a non-terminal state).

**Tech Stack:** Go 1.26.3, chi v5, controller-runtime v0.23.3, testify/require, React 19 + Vite + Tailwind, @tanstack/react-query.

**Spec:** `docs/superpowers/specs/2026-08-17-opensandbox-view-design.md`

## Global Constraints

- **Read-only.** The dashboard performs no writes. `internal/osb` must expose no method capable of issuing a non-GET request, even though the OSB API offers `POST /v1/sandboxes/{id}/pause`, `DELETE /v1/sandboxes/{id}`, and others.
- **The API key never leaves the process.** It travels only in the `OPEN-SANDBOX-API-KEY` request header — never in a URL, a log line, an error string, or a problem+json body.
- **The sandbox list must never fail because OSB is down.** An OSB error yields HTTP 200 with CR data plus `osb.status: "unreachable"`.
- **Join key is the `opensandbox.io/id` label**, never the CR name. Measured: the label matches 102/102; the name matches 29/92.
- **OSB states** are exactly `Pending, Running, Pausing, Paused, Resuming, Stopping, Terminated, Failed` (from `/openapi.json`).
- **Go style:** `make lint` runs `go vet ./cmd/... ./internal/...` and fails on any `gofmt -l` output. Run `gofmt -w` on every file you touch.
- **Tests:** `make test` runs `go test -race -count=1 ./cmd/... ./internal/...`. Test names describe behaviour, not method names.
- **Commits:** conventional commits (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`, `chore:`).
- **Env var prefix** stays `AGENT_SANDBOX_DASHBOARD_` to match the existing `AGENT_SANDBOX_DASHBOARD_METRICS_TIMEOUT`. Do not "fix" the stale prefix here.

## File Structure

| File | Responsibility |
|---|---|
| `internal/osb/client.go` (new) | HTTP client: types, `ListSandboxes`, `Diagnostics`, pagination |
| `internal/osb/client_test.go` (new) | Client behaviour against `httptest` |
| `internal/osb/cache.go` (new) | TTL + mutex wrapper around a lister |
| `internal/osb/cache_test.go` (new) | TTL and single-fetch-under-concurrency behaviour |
| `internal/server/osbview.go` (new) | `OsbView`, `OsbStatus`, agreement table, staleness — pure functions |
| `internal/server/osbview_test.go` (new) | Divergence and staleness table tests |
| `internal/server/sandboxes.go` | Join OSB into the list; new `/osb` drawer route handler |
| `internal/server/sandboxes_test.go` | Join, filters, degradation |
| `internal/server/summaries.go` | `creator`/`owner`/`team`/`experiment` from labels |
| `internal/server/router.go` | `Deps.Osb`, `Deps.Now`, `Deps.OsbStaleAfter`, route registration |
| `cmd/dashboard/main.go` | Flag/env wiring, typed-nil guard |
| `deploy/kustomize/deployment.yaml` | Commented-out OSB env block |
| `README.md` | OSB configuration section |
| `ui/src/api/client.ts` | `OsbView`, `OsbStatus`, `fetchSandboxOsb` |
| `ui/src/resources/config.ts` | `showOsb` flag |
| `ui/src/pages/ResourceList.tsx` | OSB State column, `⚠`/`⏱`, creator/owner columns, filters, banner |
| `ui/src/components/DetailDrawer.tsx` | OpenSandbox section |

**A note on PR 3 verification.** This repo has no JavaScript test runner — `ui/package.json` has no `vitest`, `jest`, or `test` script. Adding one is a dependency decision outside this plan's scope. PR 3 tasks are therefore verified by `npm run build` (which runs `tsc -b`, so type errors fail the build), `npm run lint`, and a manual check against the live cluster. Every behavioural rule that *can* live in Go is computed server-side in PR 2 precisely so it is covered by real tests; the UI only renders what the server decided.

---

# PR 1 — `internal/osb` client

No wiring, nothing user-visible. Reviewable and mergeable on its own.

### Task 1: OSB types and single-page `ListSandboxes`

**Files:**
- Create: `internal/osb/client.go`
- Test: `internal/osb/client_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `osb.Sandbox`, `osb.Status`, `osb.Client`, `osb.NewClient(baseURL, apiKey string, opts ...Option) (*Client, error)`, `osb.WithLogger(*slog.Logger) Option`, `osb.WithHTTPClient(*http.Client) Option`, `(*Client).ListSandboxes(ctx) (map[string]Sandbox, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/osb/client_test.go`:

```go
package osb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListSandboxes_ReturnsSandboxesKeyedByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items": [
				{"id":"aaa","status":{"state":"Running","reason":"DependenciesReady","message":"Pod is Ready","lastTransitionAt":"2026-08-17T14:00:05Z"},"createdAt":"2026-08-17T14:00:00Z","expiresAt":"2026-08-18T14:00:00Z"},
				{"id":"bbb","status":{"state":"Pending","reason":"SANDBOX_PENDING","message":"Sandbox is pending scheduling","lastTransitionAt":"2026-08-17T14:09:04Z"},"createdAt":"2026-08-17T14:09:04Z"}
			],
			"pagination": {"page":1,"pageSize":200,"totalItems":2,"totalPages":1,"hasNextPage":false}
		}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "secret-key")
	require.NoError(t, err)

	got, err := c.ListSandboxes(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)

	require.Equal(t, "Running", got["aaa"].Status.State)
	require.Equal(t, "DependenciesReady", got["aaa"].Status.Reason)
	require.Equal(t, "Pod is Ready", got["aaa"].Status.Message)
	require.Equal(t, time.Date(2026, 8, 17, 14, 0, 5, 0, time.UTC), got["aaa"].Status.LastTransitionAt.UTC())
	require.Equal(t, time.Date(2026, 8, 18, 14, 0, 0, 0, time.UTC), got["aaa"].ExpiresAt.UTC())

	require.Equal(t, "Pending", got["bbb"].Status.State)
	require.Nil(t, got["bbb"].ExpiresAt, "expiresAt is optional and must stay nil when absent")
}

func TestListSandboxes_SendsAPIKeyHeaderAndNeverPutsKeyInURL(t *testing.T) {
	var gotHeader, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("OPEN-SANDBOX-API-KEY")
		gotURL = r.URL.String()
		_, _ = w.Write([]byte(`{"items":[],"pagination":{"page":1,"pageSize":200,"totalItems":0,"totalPages":0,"hasNextPage":false}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "secret-key")
	require.NoError(t, err)
	_, err = c.ListSandboxes(context.Background())
	require.NoError(t, err)

	require.Equal(t, "secret-key", gotHeader)
	require.NotContains(t, gotURL, "secret-key")
}

func TestListSandboxes_ErrorsOnNon200AndKeepsKeyOutOfMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"AUTH::INVALID_KEY","message":"bad key"}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "secret-key")
	require.NoError(t, err)
	_, err = c.ListSandboxes(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
	require.NotContains(t, err.Error(), "secret-key")
}

func TestListSandboxes_ErrorsOnMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)
	_, err = c.ListSandboxes(context.Background())
	require.Error(t, err)
}

func TestListSandboxes_HonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = c.ListSandboxes(ctx)
	require.Error(t, err)
}

func TestNewClient_RejectsEmptyBaseURL(t *testing.T) {
	_, err := NewClient("", "k")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/osb/ -run TestListSandboxes -v`
Expected: FAIL — the `osb` package does not exist yet (`no Go files in .../internal/osb`).

- [ ] **Step 3: Write the minimal implementation**

Create `internal/osb/client.go`:

```go
// Package osb is a read-only client for the OpenSandbox Lifecycle API.
//
// The dashboard is read-only, so this package deliberately exposes no method
// capable of issuing a non-GET request, even though the upstream API offers
// pause, resume, delete and snapshot routes.
package osb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiKeyHeader is the header the OpenSandbox server authenticates on.
const apiKeyHeader = "OPEN-SANDBOX-API-KEY"

// Status is OpenSandbox's own view of a sandbox's lifecycle state. State is one
// of Pending, Running, Pausing, Paused, Resuming, Stopping, Terminated, Failed.
type Status struct {
	State            string     `json:"state"`
	Reason           string     `json:"reason"`
	Message          string     `json:"message"`
	LastTransitionAt *time.Time `json:"lastTransitionAt"`
}

// Sandbox is the subset of the OpenSandbox sandbox record the dashboard uses.
// Fields the CR already carries (image, entrypoint, metadata) are read from the
// CR instead, so that they survive an OpenSandbox outage.
type Sandbox struct {
	ID        string     `json:"id"`
	Status    Status     `json:"status"`
	ExpiresAt *time.Time `json:"expiresAt"`
	CreatedAt *time.Time `json:"createdAt"`
}

type pagination struct {
	Page        int  `json:"page"`
	PageSize    int  `json:"pageSize"`
	TotalItems  int  `json:"totalItems"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
}

type listResponse struct {
	Items      []Sandbox  `json:"items"`
	Pagination pagination `json:"pagination"`
}

// Client is a read-only OpenSandbox API client. It is safe for concurrent use.
type Client struct {
	base   string
	key    string
	hc     *http.Client
	logger *slog.Logger // nil means warnings are dropped silently
}

// Option configures a Client.
type Option func(*Client)

// WithLogger sets a structured logger for non-fatal warnings.
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.logger = l } }

// WithHTTPClient overrides the default HTTP client. Used by tests.
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.hc = hc } }

// NewClient builds a Client against the given OpenSandbox base URL.
func NewClient(baseURL, apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("opensandbox: base URL is empty")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("opensandbox: parse base URL: %w", err)
	}
	c := &Client{
		base: strings.TrimRight(baseURL, "/"),
		key:  apiKey,
		hc:   &http.Client{Timeout: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// get issues a GET against path and returns the response body. This is the only
// request-issuing method in the package; keeping it unexported and GET-only is
// what makes the read-only guarantee structural rather than conventional.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("opensandbox: build request: %w", err)
	}
	req.Header.Set(apiKeyHeader, c.key)

	resp, err := c.hc.Do(req)
	if err != nil {
		// err may embed the request URL but never the header, so the key cannot leak.
		return nil, fmt.Errorf("opensandbox: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("opensandbox: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opensandbox: unexpected status %d for %s", resp.StatusCode, path)
	}
	return body, nil
}

// ListSandboxes returns every sandbox OpenSandbox knows about, keyed by id.
func (c *Client) ListSandboxes(ctx context.Context) (map[string]Sandbox, error) {
	body, err := c.get(ctx, "/v1/sandboxes?pageSize=200&page=1")
	if err != nil {
		return nil, err
	}
	var lr listResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, fmt.Errorf("opensandbox: decode sandbox list: %w", err)
	}
	out := make(map[string]Sandbox, len(lr.Items))
	for _, s := range lr.Items {
		out[s.ID] = s
	}
	return out, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/osb/ && go test ./internal/osb/ -v`
Expected: PASS — all six tests.

- [ ] **Step 5: Commit**

```bash
git add internal/osb/client.go internal/osb/client_test.go
git commit -m "feat: read-only OpenSandbox API client"
```

---

### Task 2: Pagination across pages, with a page cap

The live server holds 102 sandboxes at `pageSize=200`, so one request suffices today — but `totalItems` grows and `pageSize` is capped at 200 by the API, so the loop is required.

**Files:**
- Modify: `internal/osb/client.go` (`ListSandboxes`)
- Test: `internal/osb/client_test.go`

**Interfaces:**
- Consumes: `Client.get`, `listResponse` from Task 1.
- Produces: unchanged `ListSandboxes` signature; adds package constants `maxPageSize = 200` and `maxPages = 50`.

- [ ] **Step 1: Write the failing test**

Append to `internal/osb/client_test.go`:

```go
func TestListSandboxes_WalksEveryPage(t *testing.T) {
	var pagesServed []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pagesServed = append(pagesServed, page)
		switch page {
		case "1":
			_, _ = w.Write([]byte(`{"items":[{"id":"a","status":{"state":"Running"}}],
				"pagination":{"page":1,"pageSize":200,"totalItems":3,"totalPages":3,"hasNextPage":true}}`))
		case "2":
			_, _ = w.Write([]byte(`{"items":[{"id":"b","status":{"state":"Running"}}],
				"pagination":{"page":2,"pageSize":200,"totalItems":3,"totalPages":3,"hasNextPage":true}}`))
		default:
			_, _ = w.Write([]byte(`{"items":[{"id":"c","status":{"state":"Pending"}}],
				"pagination":{"page":3,"pageSize":200,"totalItems":3,"totalPages":3,"hasNextPage":false}}`))
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)
	got, err := c.ListSandboxes(context.Background())
	require.NoError(t, err)

	require.Len(t, got, 3)
	require.Contains(t, got, "a")
	require.Contains(t, got, "b")
	require.Contains(t, got, "c")
	require.Equal(t, []string{"1", "2", "3"}, pagesServed)
}

func TestListSandboxes_StopsAtPageCapWhenServerAlwaysClaimsNextPage(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		// A broken server that never stops claiming there is more.
		_, _ = w.Write([]byte(`{"items":[{"id":"dup","status":{"state":"Running"}}],
			"pagination":{"page":1,"pageSize":200,"totalItems":99999,"totalPages":99999,"hasNextPage":true}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)
	_, err = c.ListSandboxes(context.Background())
	require.NoError(t, err, "hitting the cap is not an error; it is a bounded read")
	require.Equal(t, maxPages, requests, "must stop at the page cap rather than loop forever")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/osb/ -run 'TestListSandboxes_WalksEveryPage|TestListSandboxes_StopsAtPageCap' -v`
Expected: FAIL — `undefined: maxPages`, and `WalksEveryPage` returns only 1 item.

- [ ] **Step 3: Write the minimal implementation**

In `internal/osb/client.go`, add the constants below `apiKeyHeader`:

```go
// maxPageSize is the largest pageSize the OpenSandbox API accepts.
const maxPageSize = 200

// maxPages bounds the pagination loop.
// ponytail: fixed cap, not a resumable cursor — 50 pages is 10,000 sandboxes,
// far past any real fleet. If a cluster ever exceeds it, switch to streaming
// pages into the caller instead of raising this number.
const maxPages = 50
```

Replace `ListSandboxes` entirely with:

```go
// ListSandboxes returns every sandbox OpenSandbox knows about, keyed by id.
// It walks pages until the server reports no next page, or until maxPages is
// reached. Reaching the cap is not an error: the caller gets a bounded,
// possibly partial view, and a warning is logged.
func (c *Client) ListSandboxes(ctx context.Context) (map[string]Sandbox, error) {
	out := make(map[string]Sandbox)
	for page := 1; page <= maxPages; page++ {
		body, err := c.get(ctx, fmt.Sprintf("/v1/sandboxes?pageSize=%d&page=%d", maxPageSize, page))
		if err != nil {
			return nil, err
		}
		var lr listResponse
		if err := json.Unmarshal(body, &lr); err != nil {
			return nil, fmt.Errorf("opensandbox: decode sandbox list page %d: %w", page, err)
		}
		for _, s := range lr.Items {
			out[s.ID] = s
		}
		if !lr.Pagination.HasNextPage {
			return out, nil
		}
		if page == maxPages && c.logger != nil {
			c.logger.Warn("opensandbox_page_cap_reached",
				"pages", maxPages, "collected", len(out), "server_total", lr.Pagination.TotalItems)
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/osb/ && go test -race ./internal/osb/ -v`
Expected: PASS — all eight tests.

- [ ] **Step 5: Commit**

```bash
git add internal/osb/client.go internal/osb/client_test.go
git commit -m "feat: walk OpenSandbox list pagination with a bounded page cap"
```

---

### Task 3: TTL cache

The UI refetches every list at 5s. Without a cache, list latency is coupled to OSB on every poll.

**Files:**
- Create: `internal/osb/cache.go`
- Test: `internal/osb/cache_test.go`

**Interfaces:**
- Consumes: `osb.Sandbox` from Task 1.
- Produces: `osb.Lister` interface, `osb.NewCache(inner Lister, ttl time.Duration, now func() time.Time) *Cache`, `(*Cache).ListSandboxes(ctx) (map[string]Sandbox, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/osb/cache_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/osb/ -run TestCache -v`
Expected: FAIL — `undefined: NewCache`.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/osb/cache.go`:

```go
package osb

import (
	"context"
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
// swap in golang.org/x/sync/singleflight — it is already an indirect dep.
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
// otherwise it fetches from upstream. Errors are never cached.
func (c *Cache) ListSandboxes(ctx context.Context) (map[string]Sandbox, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && c.ttl > 0 && c.now().Sub(c.fetchedAt) < c.ttl {
		return c.cached, nil
	}
	fresh, err := c.inner.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	c.cached = fresh
	c.fetchedAt = c.now()
	return fresh, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/osb/ && go test -race ./internal/osb/ -v`
Expected: PASS — all twelve tests. `-race` matters here; it exercises the concurrency test.

- [ ] **Step 5: Commit**

```bash
git add internal/osb/cache.go internal/osb/cache_test.go
git commit -m "feat: TTL cache for the OpenSandbox sandbox inventory"
```

---

### Task 4: Diagnostics fetch

`/diagnostics/summary` and `/diagnostics/events` return **`text/plain`**, not JSON — verified against the live server. Do not try to unmarshal them.

**Files:**
- Modify: `internal/osb/client.go`
- Test: `internal/osb/client_test.go`

**Interfaces:**
- Consumes: `Client.get` from Task 1.
- Produces: `osb.Diagnostics` struct with fields `Summary string` / `Events string`; `(*Client).Diagnostics(ctx, id string) (Diagnostics, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/osb/client_test.go`:

```go
func TestDiagnostics_ReturnsSummaryAndEventsAsPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sandboxes/abc/diagnostics/summary":
			_, _ = w.Write([]byte("SANDBOX DIAGNOSTICS SUMMARY\nPhase: Running\n"))
		case "/v1/sandboxes/abc/diagnostics/events":
			_, _ = w.Write([]byte("[14:08:57] Normal Scheduled Successfully assigned\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)

	d, err := c.Diagnostics(context.Background(), "abc")
	require.NoError(t, err)
	require.Contains(t, d.Summary, "Phase: Running")
	require.Contains(t, d.Events, "Successfully assigned")
}

func TestDiagnostics_EscapesSandboxIDInPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)
	_, err = c.Diagnostics(context.Background(), "a/../b")
	require.NoError(t, err)

	// What matters is that the separators are percent-encoded, so the id stays a
	// single path segment. The literal ".." survives escaping and is harmless
	// once it cannot be preceded by an unescaped slash.
	require.Contains(t, gotPath, "a%2F..%2Fb", "the id's slashes must be escaped")
	require.NotContains(t, gotPath, "/../", "no traversable segment may reach the server")
}

func TestDiagnostics_ErrorsWhenSandboxUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"KUBERNETES::SANDBOX_NOT_FOUND","message":"Sandbox 'zzz' not found"}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)
	_, err = c.Diagnostics(context.Background(), "zzz")
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/osb/ -run TestDiagnostics -v`
Expected: FAIL — `c.Diagnostics undefined`.

- [ ] **Step 3: Write the minimal implementation**

Add to `internal/osb/client.go` (and add `"net/url"` to the imports if the linter reports it unused after edits — it is already imported by Task 1):

```go
// Diagnostics is OpenSandbox's own diagnostic output for one sandbox. Both
// fields are plain text as returned by the API, not JSON.
type Diagnostics struct {
	Summary string `json:"summary"`
	Events  string `json:"events"`
}

// Diagnostics fetches the summary and event text for one sandbox id. These
// routes are served by on-demand Kubernetes reads rather than OpenSandbox's
// watch-fed state, so they stay accurate even when the watch has stalled.
func (c *Client) Diagnostics(ctx context.Context, id string) (Diagnostics, error) {
	esc := url.PathEscape(id)
	summary, err := c.get(ctx, "/v1/sandboxes/"+esc+"/diagnostics/summary")
	if err != nil {
		return Diagnostics{}, err
	}
	events, err := c.get(ctx, "/v1/sandboxes/"+esc+"/diagnostics/events")
	if err != nil {
		return Diagnostics{}, err
	}
	return Diagnostics{Summary: string(summary), Events: string(events)}, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/osb/ && go test -race ./internal/osb/ -v && go vet ./internal/osb/`
Expected: PASS — all fifteen tests, no vet output.

- [ ] **Step 5: Commit and open PR 1**

```bash
git add internal/osb/client.go internal/osb/client_test.go
git commit -m "feat: fetch OpenSandbox per-sandbox diagnostics text"
git push -u origin feat/osb-client
gh pr create --title "feat: read-only OpenSandbox API client" \
  --body "First of three PRs for the OpenSandbox view (spec: docs/superpowers/specs/2026-08-17-opensandbox-view-design.md).

Adds internal/osb: a read-only client for the OpenSandbox Lifecycle API, with pagination, a TTL cache, and per-sandbox diagnostics. Nothing is wired up yet — no behaviour change to the dashboard.

The package exposes no method capable of a non-GET request, which is deliberate: the upstream API offers pause/resume/delete and this dashboard is read-only.

🤖 Generated with [Claude Code](https://claude.com/claude-code)"
```

---

# PR 2 — Server-side join, divergence, and staleness

Branch from `main` after PR 1 merges: `git checkout main && git pull && git checkout -b feat/osb-join`.

### Task 5: Creator and identity from CR labels

**Files:**
- Modify: `internal/server/summaries.go`
- Test: `internal/server/summaries_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.CreatorOpenSandbox`, `server.CreatorUnknown` constants; `server.OsbIDLabel` constant; `func creatorFor(labels map[string]string) string`; `func identityFor(labels map[string]string) (owner, team, experiment string)`; four new `ResourceSummary` fields.

- [ ] **Step 1: Write the failing test**

Append to `internal/server/summaries_test.go`:

```go
func TestCreatorFor_IdentifiesOpenSandboxByLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"opensandbox id label present", map[string]string{"opensandbox.io/id": "abc"}, CreatorOpenSandbox},
		{"label present but empty is not a creator", map[string]string{"opensandbox.io/id": ""}, CreatorUnknown},
		{"a different creator's labels", map[string]string{"app": "x", "swe-instance-id": "y"}, CreatorUnknown},
		{"no labels at all", nil, CreatorUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, creatorFor(tc.labels))
		})
	}
}

func TestIdentityFor_ReadsOwnerTeamExperimentFromLabels(t *testing.T) {
	owner, team, experiment := identityFor(map[string]string{
		"owner": "odeda", "team": "intelligent-gateway", "experiment": "tbv-v2",
	})
	require.Equal(t, "odeda", owner)
	require.Equal(t, "intelligent-gateway", team)
	require.Equal(t, "tbv-v2", experiment)
}

func TestIdentityFor_ToleratesPartialMetadata(t *testing.T) {
	// 30 of 92 sandboxes measured in algo-studio carried only session_id.
	owner, team, experiment := identityFor(map[string]string{"session_id": "abc__env"})
	require.Empty(t, owner)
	require.Empty(t, team)
	require.Empty(t, experiment)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run 'TestCreatorFor|TestIdentityFor' -v`
Expected: FAIL — `undefined: CreatorOpenSandbox`, `undefined: creatorFor`, `undefined: identityFor`.

- [ ] **Step 3: Write the minimal implementation**

In `internal/server/summaries.go`, add to the imports nothing new, and append:

```go
// OsbIDLabel is the label OpenSandbox stamps on every Sandbox CR it creates.
// It is the join key between the CR and the OpenSandbox API: measured against
// algo-studio, the label matched 102/102 records while the CR name matched
// only 29/92, because OpenSandbox writes both "<uuid>" and "sandbox-<uuid>".
const OsbIDLabel = "opensandbox.io/id"

// Creator values reported in ResourceSummary.Creator.
const (
	CreatorOpenSandbox = "opensandbox"
	CreatorUnknown     = "unknown"
)

// creatorFor infers which system created a sandbox. No Sandbox CR in the wild
// carries ownerReferences, so labels are the only available signal.
func creatorFor(labels map[string]string) string {
	if labels[OsbIDLabel] != "" {
		return CreatorOpenSandbox
	}
	return CreatorUnknown
}

// identityFor pulls the human-meaningful labels. These are read from the CR
// rather than the OpenSandbox API, which carries identical values, so that an
// OpenSandbox outage costs one column instead of the whole table.
func identityFor(labels map[string]string) (owner, team, experiment string) {
	return labels["owner"], labels["team"], labels["experiment"]
}
```

Then extend `ResourceSummary` in the same file:

```go
// ResourceSummary is the per-row DTO returned by every list endpoint.
type ResourceSummary struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Phase      string `json:"phase"`      // Ready | NotReady | Unknown | "" for kinds with no Ready cond
	AgeSeconds int64  `json:"ageSeconds"` // seconds since creation

	// The fields below are only populated for sandboxes; other kinds omit them.
	Creator    string   `json:"creator,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Team       string   `json:"team,omitempty"`
	Experiment string   `json:"experiment,omitempty"`
	Osb        *OsbView `json:"osb,omitempty"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/server/ && go test ./internal/server/ -run 'TestCreatorFor|TestIdentityFor' -v`
Expected: FAIL still — `undefined: OsbView`, which Task 6 defines. This is expected; the compile error names exactly the next task. If you want a green checkpoint now, temporarily comment out the `Osb *OsbView` line, confirm the tests pass, then restore it before committing.

- [ ] **Step 5: Commit**

```bash
git add internal/server/summaries.go internal/server/summaries_test.go
git commit -m "feat: derive sandbox creator and identity from CR labels"
```

---

### Task 6: Divergence agreement table

**Files:**
- Create: `internal/server/osbview.go`
- Test: `internal/server/osbview_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.OsbView` struct (fields `State`, `Reason`, `Message`, `ExpiresAt`, `LastTransitionAt`, `StateAgeSeconds`, `Diverged`, `Stale`); `func agrees(osbState, phase string) bool`.

- [ ] **Step 1: Write the failing test**

Create `internal/server/osbview_test.go`:

```go
package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgrees_RunningMatchesReadyOnly(t *testing.T) {
	require.True(t, agrees("Running", "Ready"))
	require.False(t, agrees("Running", "NotReady"))
	require.False(t, agrees("Running", "Unknown"))
}

func TestAgrees_NonRunningStatesMatchNotReadyAndUnknown(t *testing.T) {
	for _, state := range []string{"Pending", "Pausing", "Paused", "Resuming", "Stopping", "Terminated", "Failed"} {
		t.Run(state, func(t *testing.T) {
			require.True(t, agrees(state, "NotReady"))
			require.True(t, agrees(state, "Unknown"))
			require.False(t, agrees(state, "Ready"),
				"this is the incident signature: OpenSandbox not-Running while the pod is Ready")
		})
	}
}

func TestAgrees_UnrecognisedStateNeverReportsDisagreement(t *testing.T) {
	// A future OpenSandbox state must ship as a blank cell, not a fleet-wide alarm.
	for _, phase := range []string{"Ready", "NotReady", "Unknown", ""} {
		require.True(t, agrees("SomeFutureState", phase))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run TestAgrees -v`
Expected: FAIL — `undefined: agrees`.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/server/osbview.go`:

```go
package server

import "time"

// OsbView is OpenSandbox's own view of a sandbox, plus the two signals the
// dashboard derives from it. It is only ever populated for sandboxes that
// carry the OsbIDLabel and were matched against the OpenSandbox inventory.
type OsbView struct {
	State            string     `json:"state"`
	Reason           string     `json:"reason,omitempty"`
	Message          string     `json:"message,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	LastTransitionAt *time.Time `json:"lastTransitionAt,omitempty"`
	StateAgeSeconds  int64      `json:"stateAgeSeconds"`
	Diverged         bool       `json:"diverged"`
	Stale            bool       `json:"stale"`
}

// osbAgreement maps each OpenSandbox state to the CR phases it is consistent
// with. OpenSandbox's eight-state lifecycle collapses into the Ready
// condition's three values, so this table is where that judgment lives.
var osbAgreement = map[string][]string{
	"Running":    {"Ready"},
	"Pending":    {"NotReady", "Unknown"},
	"Pausing":    {"NotReady", "Unknown"},
	"Paused":     {"NotReady", "Unknown"},
	"Resuming":   {"NotReady", "Unknown"},
	"Stopping":   {"NotReady", "Unknown"},
	"Terminated": {"NotReady", "Unknown"},
	"Failed":     {"NotReady", "Unknown"},
}

// agrees reports whether an OpenSandbox state is consistent with a CR phase.
// An unrecognised state agrees with everything: a state this build has never
// heard of is "no opinion", not a disagreement, so adding a state upstream
// cannot light up the whole fleet.
func agrees(osbState, phase string) bool {
	allowed, known := osbAgreement[osbState]
	if !known {
		return true
	}
	for _, p := range allowed {
		if p == phase {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/server/ && go test ./internal/server/ -run TestAgrees -v`
Expected: PASS — three tests, ten subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/server/osbview.go internal/server/osbview_test.go
git commit -m "feat: agreement table for OpenSandbox state versus CR phase"
```

---

### Task 7: Staleness

The sharper of the two signals. During the incident every stuck sandbox had `lastTransitionAt == createdAt`, because OSB never received a second event.

**Files:**
- Modify: `internal/server/osbview.go`
- Test: `internal/server/osbview_test.go`

**Interfaces:**
- Consumes: `OsbView`, `agrees` from Task 6; `osb.Sandbox` from Task 1.
- Produces: `func isStale(state string, lastTransitionAt *time.Time, now time.Time, threshold time.Duration) bool`; `func newOsbView(s osb.Sandbox, phase string, now time.Time, staleAfter time.Duration) OsbView`; `const DefaultOsbStaleAfter = 60 * time.Second`.

- [ ] **Step 1: Write the failing test**

Append to `internal/server/osbview_test.go` (add `"time"`, `"github.com/aGallea/sandbox-dashboard/internal/osb"` to the imports):

```go
func TestIsStale_YoungNonTerminalStateIsNotStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	twentySecondsAgo := now.Add(-20 * time.Second)
	require.False(t, isStale("Pending", &twentySecondsAgo, now, time.Minute))
}

func TestIsStale_NonTerminalStatePastThresholdIsStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	nineMinutesAgo := now.Add(-9 * time.Minute)
	for _, state := range []string{"Pending", "Pausing", "Resuming", "Stopping"} {
		t.Run(state, func(t *testing.T) {
			require.True(t, isStale(state, &nineMinutesAgo, now, time.Minute))
		})
	}
}

func TestIsStale_RestingStatesAreNeverStaleRegardlessOfAge(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	longAgo := now.Add(-30 * time.Hour)
	// Running is where a healthy sandbox lives for hours; Terminated and Failed
	// are final; Paused is a state a caller deliberately holds.
	for _, state := range []string{"Running", "Terminated", "Failed", "Paused"} {
		t.Run(state, func(t *testing.T) {
			require.False(t, isStale(state, &longAgo, now, time.Minute))
		})
	}
}

func TestIsStale_MissingTimestampIsNotStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	require.False(t, isStale("Pending", nil, now, time.Minute))
}

func TestIsStale_UnrecognisedStateIsNotStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	longAgo := now.Add(-30 * time.Hour)
	require.False(t, isStale("SomeFutureState", &longAgo, now, time.Minute))
}

func TestNewOsbView_FlagsTheRecordedIncidentAsDivergedAndStale(t *testing.T) {
	// Recorded from algo-studio, sandbox 726f8779-…: created 14:08:57, pod Ready
	// at 14:08:59, OpenSandbox still Pending with lastTransitionAt == createdAt
	// when observed at 14:11:40.
	created := time.Date(2026, 8, 17, 14, 8, 57, 0, time.UTC)
	observed := time.Date(2026, 8, 17, 14, 11, 40, 0, time.UTC)
	s := osb.Sandbox{
		ID:        "726f8779-a7df-4c9c-a5ba-561c5f4a3acf",
		Status:    osb.Status{State: "Pending", Reason: "SANDBOX_PENDING", Message: "Sandbox is pending scheduling", LastTransitionAt: &created},
		CreatedAt: &created,
	}

	v := newOsbView(s, "Ready", observed, DefaultOsbStaleAfter)

	require.Equal(t, "Pending", v.State)
	require.Equal(t, "SANDBOX_PENDING", v.Reason)
	require.True(t, v.Diverged, "OpenSandbox Pending against a Ready pod is a disagreement")
	require.True(t, v.Stale, "the state had not moved in 2m43s")
	require.Equal(t, int64(163), v.StateAgeSeconds)
}

func TestNewOsbView_HealthyRunningSandboxFlagsNothing(t *testing.T) {
	transitioned := time.Date(2026, 8, 16, 19, 37, 17, 0, time.UTC)
	observed := time.Date(2026, 8, 17, 14, 11, 40, 0, time.UTC)
	s := osb.Sandbox{
		ID:     "fb52dbeb",
		Status: osb.Status{State: "Running", Reason: "DependenciesReady", LastTransitionAt: &transitioned},
	}

	v := newOsbView(s, "Ready", observed, DefaultOsbStaleAfter)

	require.False(t, v.Diverged)
	require.False(t, v.Stale, "a sandbox running for 18 hours is healthy, not stale")
}

// This is the case that justifies keeping the two flags separate: an ordinary
// in-flight creation disagrees for a moment, but nothing is wrong.
func TestNewOsbView_YoungPendingSandboxIsNotStale(t *testing.T) {
	created := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	observed := created.Add(3 * time.Second)
	s := osb.Sandbox{
		ID:     "young",
		Status: osb.Status{State: "Pending", LastTransitionAt: &created},
	}

	v := newOsbView(s, "Ready", observed, DefaultOsbStaleAfter)

	require.True(t, v.Diverged)
	require.False(t, v.Stale, "3s of disagreement must not raise the staleness alarm")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run 'TestIsStale|TestNewOsbView' -v`
Expected: FAIL — `undefined: isStale`, `undefined: newOsbView`, `undefined: DefaultOsbStaleAfter`.

- [ ] **Step 3: Write the minimal implementation**

Append to `internal/server/osbview.go`, and add `"github.com/aGallea/sandbox-dashboard/internal/osb"` to its imports:

```go
// DefaultOsbStaleAfter is how long a non-terminal OpenSandbox state may sit
// unchanged before it is reported stale. Sixty seconds comes from measurement,
// not taste: pods in algo-studio reached Ready about two seconds after
// creation, so a minute is already far outside normal.
const DefaultOsbStaleAfter = 60 * time.Second

// osbTransientStates are the states a sandbox should pass through in seconds.
// Age against these is meaningful. Running, Terminated and Failed are resting
// places, and Paused is a state a caller deliberately holds, so none of them
// can be stale no matter how old.
var osbTransientStates = map[string]bool{
	"Pending":  true,
	"Pausing":  true,
	"Resuming": true,
	"Stopping": true,
}

// isStale reports whether a transient OpenSandbox state has sat unchanged past
// the threshold. This is what distinguishes a dead watch from an ordinary
// in-flight transition: during the 2026-08-17 incident the stuck sandboxes had
// lastTransitionAt equal to createdAt, because no second event ever arrived.
func isStale(state string, lastTransitionAt *time.Time, now time.Time, threshold time.Duration) bool {
	if !osbTransientStates[state] || lastTransitionAt == nil {
		return false
	}
	return now.Sub(*lastTransitionAt) > threshold
}

// newOsbView builds the OpenSandbox column values for one sandbox, given the
// CR phase it is being compared against.
func newOsbView(s osb.Sandbox, phase string, now time.Time, staleAfter time.Duration) OsbView {
	v := OsbView{
		State:            s.Status.State,
		Reason:           s.Status.Reason,
		Message:          s.Status.Message,
		ExpiresAt:        s.ExpiresAt,
		LastTransitionAt: s.Status.LastTransitionAt,
		Diverged:         !agrees(s.Status.State, phase),
		Stale:            isStale(s.Status.State, s.Status.LastTransitionAt, now, staleAfter),
	}
	if s.Status.LastTransitionAt != nil {
		if age := now.Sub(*s.Status.LastTransitionAt); age > 0 {
			v.StateAgeSeconds = int64(age.Seconds())
		}
	}
	return v
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/server/ && go test ./internal/server/ -run 'TestIsStale|TestNewOsbView|TestAgrees' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/osbview.go internal/server/osbview_test.go
git commit -m "feat: flag OpenSandbox states that stop advancing"
```

---

### Task 8: Join OSB into the sandbox list

**Files:**
- Modify: `internal/server/router.go` (`Deps`)
- Modify: `internal/server/sandboxes.go` (`handleSandboxList`)
- Test: `internal/server/sandboxes_test.go`

**Interfaces:**
- Consumes: `newOsbView`, `creatorFor`, `identityFor`, `osb.Sandbox`.
- Produces: `server.OsbClient` interface; `Deps.Osb OsbClient`, `Deps.Now func() time.Time`, `Deps.OsbStaleAfter time.Duration`; `server.OsbStatus` struct; sandbox list response `{items, osb?}`.

- [ ] **Step 1: Write the failing test**

Append to `internal/server/sandboxes_test.go` (add `"context"`, `"errors"`, `"time"`, and `"github.com/aGallea/sandbox-dashboard/internal/osb"` to the imports):

```go
// fakeOsb is a stand-in for *osb.Client in handler tests.
type fakeOsb struct {
	list map[string]osb.Sandbox
	err  error
	diag osb.Diagnostics
}

func (f *fakeOsb) ListSandboxes(context.Context) (map[string]osb.Sandbox, error) {
	return f.list, f.err
}

func (f *fakeOsb) Diagnostics(context.Context, string) (osb.Diagnostics, error) {
	return f.diag, f.err
}

// sandboxListBody is the shape of GET /api/v1/sandboxes.
type sandboxListBody struct {
	Items []ResourceSummary `json:"items"`
	Osb   *OsbStatus        `json:"osb"`
}

func osbTestDeps(t *testing.T, objs []client.Object, o OsbClient, now time.Time) http.Handler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	d := Deps{
		Client:       c,
		CacheSynced:  func() bool { return true },
		Now:          func() time.Time { return now },
		OsbStaleAfter: DefaultOsbStaleAfter,
	}
	if o != nil {
		d.Osb = o
	}
	return New(d)
}

func TestSandboxes_List_JoinsOpenSandboxStateOnTheIDLabel(t *testing.T) {
	created := time.Date(2026, 8, 17, 14, 8, 57, 0, time.UTC)
	observed := time.Date(2026, 8, 17, 14, 11, 40, 0, time.UTC)

	// The CR name is deliberately unequal to the id: only the label may join.
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sandbox-726f8779", Namespace: "default",
				Labels: map[string]string{OsbIDLabel: "726f8779", "owner": "odeda", "team": "ig"},
			},
			Status: v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	o := &fakeOsb{list: map[string]osb.Sandbox{
		"726f8779": {
			ID:     "726f8779",
			Status: osb.Status{State: "Pending", Reason: "SANDBOX_PENDING", LastTransitionAt: &created},
		},
	}}

	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, o, observed).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got sandboxListBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)

	it := got.Items[0]
	require.Equal(t, CreatorOpenSandbox, it.Creator)
	require.Equal(t, "odeda", it.Owner)
	require.Equal(t, "ig", it.Team)
	require.Equal(t, "Ready", it.Phase)
	require.NotNil(t, it.Osb)
	require.Equal(t, "Pending", it.Osb.State)
	require.True(t, it.Osb.Diverged)
	require.True(t, it.Osb.Stale)

	require.NotNil(t, got.Osb)
	require.Equal(t, "ok", got.Osb.Status)
	require.Equal(t, 1, got.Osb.Reported)
	require.Equal(t, 1, got.Osb.Matched)
}

func TestSandboxes_List_NonOpenSandboxCRGetsNoOsbBlock(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "instance-element-web", Namespace: "default",
				Labels: map[string]string{"app": "element", "swe-instance-id": "x"},
			},
			Status: v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, &fakeOsb{list: map[string]osb.Sandbox{}}, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil))

	var got sandboxListBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	require.Equal(t, CreatorUnknown, got.Items[0].Creator)
	require.Nil(t, got.Items[0].Osb)
}

func TestSandboxes_List_LabelledCRWithNoMatchingOsbRecordGetsNoOsbBlock(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "orphan", Namespace: "default",
				Labels: map[string]string{OsbIDLabel: "not-in-inventory"},
			},
			Status: v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, &fakeOsb{list: map[string]osb.Sandbox{"other": {ID: "other"}}}, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil))

	var got sandboxListBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, CreatorOpenSandbox, got.Items[0].Creator, "the label still identifies the creator")
	require.Nil(t, got.Items[0].Osb, "but there is no state to show")
	require.Equal(t, 1, got.Osb.Reported)
	require.Equal(t, 0, got.Osb.Matched)
}

func TestSandboxes_List_StillServesCRDataWhenOpenSandboxIsUnreachable(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "a", Namespace: "default",
				Labels: map[string]string{OsbIDLabel: "a"},
			},
			Status: v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	o := &fakeOsb{err: errors.New("dial tcp: connection refused to http://osb:80?key=secret-key")}

	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, o, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil))

	require.Equal(t, http.StatusOK, rec.Code, "an OpenSandbox outage must not fail the list")
	require.NotContains(t, rec.Body.String(), "secret-key", "upstream error text must never reach the client")

	var got sandboxListBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	require.Equal(t, "Ready", got.Items[0].Phase)
	require.Nil(t, got.Items[0].Osb)
	require.NotNil(t, got.Osb)
	require.Equal(t, "unreachable", got.Osb.Status)
}

func TestSandboxes_List_OmitsOsbBlockEntirelyWhenUnconfigured(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, nil, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil))

	var got sandboxListBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	require.Nil(t, got.Osb, "no OpenSandbox configured means no osb block at all")
}

func TestSandboxes_List_FiltersByCreatorStateAndStaleness(t *testing.T) {
	created := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	observed := created.Add(10 * time.Minute)
	recent := observed.Add(-2 * time.Second)

	objs := []client.Object{
		&v1alpha1.Sandbox{ // stale + diverged
			ObjectMeta: metav1.ObjectMeta{Name: "stuck", Namespace: "default", Labels: map[string]string{OsbIDLabel: "stuck"}},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
		&v1alpha1.Sandbox{ // healthy
			ObjectMeta: metav1.ObjectMeta{Name: "fine", Namespace: "default", Labels: map[string]string{OsbIDLabel: "fine"}},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
		&v1alpha1.Sandbox{ // another creator
			ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default", Labels: map[string]string{"app": "x"}},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	o := &fakeOsb{list: map[string]osb.Sandbox{
		"stuck": {ID: "stuck", Status: osb.Status{State: "Pending", LastTransitionAt: &created}},
		"fine":  {ID: "fine", Status: osb.Status{State: "Running", LastTransitionAt: &recent}},
	}}
	h := osbTestDeps(t, objs, o, observed)

	tests := []struct {
		path string
		want []string
	}{
		{"/api/v1/sandboxes", []string{"stuck", "fine", "other"}},
		{"/api/v1/sandboxes?creator=opensandbox", []string{"stuck", "fine"}},
		{"/api/v1/sandboxes?creator=unknown", []string{"other"}},
		{"/api/v1/sandboxes?osbState=Pending", []string{"stuck"}},
		{"/api/v1/sandboxes?osbState=Running", []string{"fine"}},
		{"/api/v1/sandboxes?stale=true", []string{"stuck"}},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			require.Equal(t, http.StatusOK, rec.Code)
			var got sandboxListBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			names := make([]string, len(got.Items))
			for i := range got.Items {
				names[i] = got.Items[i].Name
			}
			require.ElementsMatch(t, tc.want, names)
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run TestSandboxes_List -v`
Expected: FAIL — `undefined: OsbClient`, `undefined: OsbStatus`, `Deps.Now undefined`.

- [ ] **Step 3: Write the minimal implementation**

First, in `internal/server/router.go`, add to the `Deps` struct (keeping the existing doc-comment style):

```go
	// Osb is the optional OpenSandbox client. If nil, sandbox rows carry no
	// OpenSandbox state and the list response omits its osb block.
	Osb OsbClient
	// Now supplies the current time; tests substitute a fixed clock.
	// If nil, time.Now is used.
	Now func() time.Time
	// OsbStaleAfter is how long a transient OpenSandbox state may sit before it
	// is reported stale. If zero, DefaultOsbStaleAfter is used.
	OsbStaleAfter time.Duration
```

Add `"time"` and the `osb` import to `router.go`'s import block, then add above `New`:

```go
// OsbClient is the subset of *osb.Client the sandbox handlers depend on.
type OsbClient interface {
	ListSandboxes(ctx context.Context) (map[string]osb.Sandbox, error)
	Diagnostics(ctx context.Context, id string) (osb.Diagnostics, error)
}

// now returns the Deps clock, defaulting to time.Now.
func (d Deps) now() time.Time {
	if d.Now == nil {
		return time.Now()
	}
	return d.Now()
}

// staleAfter returns the configured staleness threshold, defaulting to DefaultOsbStaleAfter.
func (d Deps) staleAfter() time.Duration {
	if d.OsbStaleAfter <= 0 {
		return DefaultOsbStaleAfter
	}
	return d.OsbStaleAfter
}
```

Add `"context"` to `router.go` imports as well.

Next, in `internal/server/osbview.go`, append the response-level status type:

```go
// OsbStatus reports the health of the OpenSandbox join for one list response.
// It is omitted entirely when no OpenSandbox URL is configured.
type OsbStatus struct {
	Status    string     `json:"status"` // "ok" | "unreachable"
	Error     string     `json:"error,omitempty"`
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
	Reported  int        `json:"reported"` // sandboxes OpenSandbox returned
	Matched   int        `json:"matched"`  // of those, how many joined to a CR
}
```

Finally, in `internal/server/sandboxes.go`, replace `handleSandboxList` entirely:

```go
// sandboxListResponse is the JSON returned by GET /api/v1/sandboxes.
type sandboxListResponse struct {
	Items []ResourceSummary `json:"items"`
	Osb   *OsbStatus        `json:"osb,omitempty"`
}

func handleSandboxList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nsFilter := r.URL.Query().Get("namespace")
		phaseFilter := r.URL.Query().Get("phase")
		creatorFilter := r.URL.Query().Get("creator")
		osbStateFilter := r.URL.Query().Get("osbState")
		staleOnly := r.URL.Query().Get("stale") == "true"

		var list v1alpha1.SandboxList
		if err := d.Client.List(r.Context(), &list); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-sandboxes",
				Detail:    "could not list sandboxes",
				LogReason: err.Error(),
			})
			return
		}

		now := d.now()
		staleAfter := d.staleAfter()

		// Fetch the OpenSandbox inventory once for the whole page. A failure
		// here is never fatal: the CR data is still worth serving.
		var (
			inventory map[string]osb.Sandbox
			osbStatus *OsbStatus
		)
		if d.Osb != nil {
			fetched, err := d.Osb.ListSandboxes(r.Context())
			if err != nil {
				if d.Logger != nil {
					d.Logger.Error("osb_list_failed", "err", err.Error())
				}
				osbStatus = &OsbStatus{Status: "unreachable", Error: "OpenSandbox is unreachable"}
			} else {
				inventory = fetched
				at := now
				osbStatus = &OsbStatus{Status: "ok", FetchedAt: &at, Reported: len(fetched)}
			}
		}

		summaries := make([]ResourceSummary, 0, len(list.Items))
		matched := 0
		for i := range list.Items {
			item := &list.Items[i]
			phase := readyPhase(item.Status.Conditions)
			creator := creatorFor(item.Labels)
			owner, team, experiment := identityFor(item.Labels)

			// Join before any display filter: `matched` must count join success
			// across the whole fleet, not "joined and survived the filters".
			// Counting it after the filters made ?namespace= report fewer matches
			// than reported and fire a false osb_join_incomplete warning.
			var view *OsbView
			if id := item.Labels[OsbIDLabel]; id != "" {
				if s, ok := inventory[id]; ok {
					matched++
					v := newOsbView(s, phase, now, staleAfter)
					view = &v
				}
			}

			if nsFilter != "" && item.Namespace != nsFilter {
				continue
			}
			if phaseFilter != "" && phase != phaseFilter {
				continue
			}
			if creatorFilter != "" && creator != creatorFilter {
				continue
			}
			// These two yield an empty list when the inventory is unavailable,
			// because every view is nil. A client MUST check osb.status == "ok"
			// before presenting an empty result as "nothing is stale".
			if osbStateFilter != "" && (view == nil || view.State != osbStateFilter) {
				continue
			}
			if staleOnly && (view == nil || !view.Stale) {
				continue
			}

			summaries = append(summaries, ResourceSummary{
				Name:       item.Name,
				Namespace:  item.Namespace,
				Kind:       "Sandbox",
				Phase:      phase,
				AgeSeconds: ageSeconds(item.ObjectMeta, now),
				Creator:    creator,
				Owner:      owner,
				Team:       team,
				Experiment: experiment,
				Osb:        view,
			})
		}

		// Reported-versus-matched makes a broken join key visible instead of
		// silently degrading every row to creator "unknown".
		if osbStatus != nil && osbStatus.Status == "ok" {
			osbStatus.Matched = matched
			if d.Logger != nil && osbStatus.Reported != matched {
				d.Logger.Warn("osb_join_incomplete", "reported", osbStatus.Reported, "matched", matched)
			}
		}

		writeJSON(w, http.StatusOK, sandboxListResponse{Items: summaries, Osb: osbStatus})
	}
}
```

Add `"github.com/aGallea/sandbox-dashboard/internal/osb"` to `sandboxes.go`'s imports.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -w internal/server/ && go test -race ./internal/server/ -v`
Expected: PASS — including the pre-existing `TestSandboxes_List_FiltersByNamespaceAndPhase`, which must still pass unchanged because `items` kept its shape.

- [ ] **Step 5: Commit**

```bash
git add internal/server/router.go internal/server/sandboxes.go internal/server/osbview.go internal/server/sandboxes_test.go
git commit -m "feat: join OpenSandbox state into the sandbox list"
```

---

### Task 9: Per-sandbox diagnostics route

**Files:**
- Modify: `internal/server/sandboxes.go`
- Modify: `internal/server/router.go` (route registration)
- Test: `internal/server/sandboxes_test.go`

**Interfaces:**
- Consumes: `Deps.Osb`, `OsbIDLabel`.
- Produces: `GET /api/v1/sandboxes/{namespace}/{name}/osb` returning `{"id","summary","events"}`.

- [ ] **Step 1: Write the failing test**

Append to `internal/server/sandboxes_test.go`:

```go
func TestSandboxOsb_ReturnsDiagnosticsForAnOpenSandboxSandbox(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sandbox-abc", Namespace: "default",
				Labels: map[string]string{OsbIDLabel: "abc"},
			},
		},
	}
	o := &fakeOsb{diag: osb.Diagnostics{Summary: "Phase: Running", Events: "Normal Scheduled"}}

	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, o, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/sandbox-abc/osb", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
		Events  string `json:"events"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "abc", got.ID)
	require.Equal(t, "Phase: Running", got.Summary)
	require.Equal(t, "Normal Scheduled", got.Events)
}

func TestSandboxOsb_Returns404WhenSandboxHasNoOpenSandboxLabel(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "default"}},
	}
	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, &fakeOsb{}, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/plain/osb", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSandboxOsb_Returns503WhenOpenSandboxUnconfigured(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sandbox-abc", Namespace: "default",
				Labels: map[string]string{OsbIDLabel: "abc"},
			},
		},
	}
	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, nil, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/sandbox-abc/osb", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestSandboxOsb_Returns502AndHidesUpstreamDetailWhenFetchFails(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sandbox-abc", Namespace: "default",
				Labels: map[string]string{OsbIDLabel: "abc"},
			},
		},
	}
	o := &fakeOsb{err: errors.New("boom at http://osb?key=secret-key")}

	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, o, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/sandbox-abc/osb", nil))

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), "secret-key")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run TestSandboxOsb -v`
Expected: FAIL — the route 404s for every case because it is not registered (the `Returns404` case passes by accident; the others fail).

- [ ] **Step 3: Write the minimal implementation**

Append to `internal/server/sandboxes.go`:

```go
// SandboxOsbDetail is the JSON returned by GET /api/v1/sandboxes/{ns}/{name}/osb.
type SandboxOsbDetail struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Events  string `json:"events"`
}

func handleSandboxOsb(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Osb == nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusServiceUnavailable,
				Type:   "opensandbox-unconfigured",
				Detail: "OpenSandbox URL not configured on this dashboard install",
			})
			return
		}
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		var sb v1alpha1.Sandbox
		if err := d.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &sb); err != nil {
			if apierrors.IsNotFound(err) {
				writeProblem(w, d.Logger, problemArgs{
					Status: http.StatusNotFound, Type: "sandbox-not-found",
					Detail: "sandbox not found",
				})
				return
			}
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "get-sandbox",
				Detail:    "could not load sandbox",
				LogReason: err.Error(),
			})
			return
		}

		id := sb.Labels[OsbIDLabel]
		if id == "" {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusNotFound,
				Type:   "not-an-opensandbox-sandbox",
				Detail: "this sandbox was not created by OpenSandbox",
			})
			return
		}

		diag, err := d.Osb.Diagnostics(r.Context(), id)
		if err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusBadGateway,
				Type:      "opensandbox-unreachable",
				Detail:    "could not load OpenSandbox diagnostics",
				LogReason: err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, SandboxOsbDetail{ID: id, Summary: diag.Summary, Events: diag.Events})
	}
}
```

In `internal/server/router.go`, register it directly after the existing sandbox detail route:

```go
			r.Get("/sandboxes/{namespace}/{name}", handleSandboxDetail(d))
			r.Get("/sandboxes/{namespace}/{name}/osb", handleSandboxOsb(d))
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -w internal/server/ && go test -race ./internal/server/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/sandboxes.go internal/server/router.go internal/server/sandboxes_test.go
git commit -m "feat: serve OpenSandbox diagnostics per sandbox"
```

---

### Task 10: Wire OSB into `main.go`, the Deployment, and the README

**Files:**
- Modify: `cmd/dashboard/main.go`
- Modify: `deploy/kustomize/deployment.yaml`
- Modify: `README.md`

**Interfaces:**
- Consumes: `osb.NewClient`, `osb.NewCache`, `server.Deps.Osb`.
- Produces: `--opensandbox-url` flag, `OPENSANDBOX_URL`, `OPENSANDBOX_API_KEY`, `AGENT_SANDBOX_DASHBOARD_OSB_TTL`, `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER`.

- [ ] **Step 1: Add the flag, env fallbacks, and duration helpers**

In `cmd/dashboard/main.go`, add after the `prometheus-url` flag registration:

```go
	var osbURL string
	flag.StringVar(&osbURL, "opensandbox-url", "", "Optional OpenSandbox base URL (e.g. http://opensandbox-server.default.svc). If empty, sandbox rows carry no OpenSandbox state.")
```

`osb.Cache` satisfies only the list half of `server.OsbClient` — `Diagnostics` lives on the raw client — so the two are combined by a small adapter. Add it at the bottom of `main.go`, together with the env-duration helper:

```go
// osbAdapter combines a cached lister with the uncached client's diagnostics,
// satisfying server.OsbClient. Diagnostics are per-sandbox and only fetched
// when a drawer is opened, so they are deliberately not cached.
type osbAdapter struct {
	*osb.Cache
	client *osb.Client
}

func (a osbAdapter) Diagnostics(ctx context.Context, id string) (osb.Diagnostics, error) {
	return a.client.Diagnostics(ctx, id)
}

// durationFromEnv reads a Go duration string (e.g. "10s") from the environment,
// returning def when unset or invalid.
func durationFromEnv(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
```

Then add the wiring itself, after the existing Prometheus client block:

```go
	if osbURL == "" {
		osbURL = os.Getenv("OPENSANDBOX_URL")
	}
	var osbClient server.OsbClient
	if osbURL != "" {
		apiKey := os.Getenv("OPENSANDBOX_API_KEY")
		if apiKey == "" {
			logger.Warn("OPENSANDBOX_API_KEY is empty — OpenSandbox requests will likely be rejected")
		}
		raw, err := osb.NewClient(osbURL, apiKey, osb.WithLogger(logger))
		if err != nil {
			logger.Error("create opensandbox client", "err", err)
			os.Exit(1)
		}
		cache := osb.NewCache(raw, durationFromEnv("AGENT_SANDBOX_DASHBOARD_OSB_TTL", 5*time.Second), time.Now)
		osbClient = osbAdapter{Cache: cache, client: raw}
		logger.Info("opensandbox client configured", "url", osbURL)
	} else {
		logger.Info("opensandbox URL not set — sandbox rows will carry no OpenSandbox state")
	}
```

Then extend the `deps` assignment, mirroring the existing typed-nil comment:

```go
	deps := server.Deps{
		Client:        mgr.GetClient(),
		CacheSynced:   cacheSynced.Load,
		UIAssets:      assets,
		Logger:        logger,
		OsbStaleAfter: durationFromEnv("AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER", server.DefaultOsbStaleAfter),
	}
	if promClient != nil {
		deps.Prom = promClient
	}
	// Same typed-nil hazard as Prom above: only assign when non-nil.
	if osbClient != nil {
		deps.Osb = osbClient
	}
```

Add `"context"` and `"github.com/aGallea/sandbox-dashboard/internal/osb"` to `main.go`'s imports.

- [ ] **Step 2: Verify it builds and every test still passes**

Run: `gofmt -w cmd internal && go build ./... && go vet ./cmd/... ./internal/... && go test -race -count=1 ./cmd/... ./internal/...`
Expected: build succeeds, no vet output, all tests PASS.

- [ ] **Step 3: Add the Deployment env block**

In `deploy/kustomize/deployment.yaml`, inside the dashboard container spec, add a commented block (OSB is optional, so it ships disabled — matching how `imagePullSecrets` is handled in this repo):

```yaml
          # Optional OpenSandbox integration. Uncomment to show OpenSandbox's own
          # lifecycle state next to the Kubernetes Ready condition. The API key is
          # read from a Secret you create; the dashboard never reads Secrets via the
          # API server, so no extra RBAC is needed.
          # env:
          #   - name: OPENSANDBOX_URL
          #     value: "http://opensandbox-server.default.svc"
          #   - name: OPENSANDBOX_API_KEY
          #     valueFrom:
          #       secretKeyRef:
          #         name: opensandbox-server-api-key
          #         key: api-key
```

- [ ] **Step 4: Document it in the README**

Add a subsection after "Configuring Prometheus (optional)". The outer fence below is four
backticks because the content itself contains fenced blocks — copy the inside only:

````markdown
### Configuring OpenSandbox (optional)

When sandboxes are created by [OpenSandbox](https://github.com/open-sandbox), the dashboard
can show OpenSandbox's own lifecycle state next to the Kubernetes Ready condition. The two
can disagree — a sandbox whose pod is `Ready` may still be `Pending` to OpenSandbox — and
that disagreement is worth seeing.

```bash
OPENSANDBOX_URL=http://opensandbox-server.default.svc \
OPENSANDBOX_API_KEY=$(kubectl -n default get secret opensandbox-server-api-key \
  -o jsonpath='{.data.api-key}' | base64 -d) \
./dashboard --kubeconfig=$HOME/.kube/config
```

Sandboxes are matched to OpenSandbox records on the `opensandbox.io/id` label, not the
resource name. The list gains three things:

- a **Creator** column, so sandboxes from other creators stay legible
- an **OSB State** column, marked `⚠` when it disagrees with the Ready condition
- a **stale** marker (`⏱`) when OpenSandbox has not advanced a transient state within
  `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER` (default `60s`). Filter with `?stale=true`.

Both are optional and degrade quietly: with `OPENSANDBOX_URL` unset the columns disappear,
and if OpenSandbox is unreachable the list still serves Kubernetes data with a warning.

| Env | Default | Purpose |
|---|---|---|
| `OPENSANDBOX_URL` | unset | Base URL; unset disables the integration |
| `OPENSANDBOX_API_KEY` | unset | Sent as the `OPEN-SANDBOX-API-KEY` header |
| `AGENT_SANDBOX_DASHBOARD_OSB_TTL` | `5s` | How long the inventory is cached |
| `AGENT_SANDBOX_DASHBOARD_OSB_STALE_AFTER` | `60s` | Staleness threshold |
````

- [ ] **Step 5: Commit and open PR 2**

```bash
gofmt -w cmd internal
make lint && make test
git add cmd/dashboard/main.go deploy/kustomize/deployment.yaml README.md
git commit -m "feat: wire the optional OpenSandbox client into the dashboard"
git push -u origin feat/osb-join
gh pr create --title "feat: join OpenSandbox state into the sandbox API" \
  --body "Second of three PRs for the OpenSandbox view (spec: docs/superpowers/specs/2026-08-17-opensandbox-view-design.md).

Joins the OpenSandbox inventory onto the Sandbox CR list on the \`opensandbox.io/id\` label, and derives two independent signals server-side:

- \`diverged\` — OpenSandbox's state disagrees with the Ready condition
- \`stale\` — a transient OpenSandbox state has not advanced within the threshold

Staleness exists because a bare state diff cannot tell a dead watch from an ordinary in-flight creation. The test fixtures are the real values recorded during the 2026-08-17 incident, when a silently dead watch had OpenSandbox reporting \`Pending\` for sandboxes whose pods had been \`Ready\` for minutes.

OpenSandbox is optional throughout: unset means the fields are absent, and unreachable still returns 200 with Kubernetes data. No UI changes yet.

🤖 Generated with [Claude Code](https://claude.com/claude-code)"
```

---

# PR 3 — UI

Branch from `main` after PR 2 merges: `git checkout main && git pull && git checkout -b feat/osb-ui`.

There is no JS test runner in this repo. Each task below is verified by `npm run build` (which runs `tsc -b`, so a type error fails it) and `npm run lint`, plus the manual check in Task 13.

### Task 11: API types and resource config

**Files:**
- Modify: `ui/src/api/client.ts`
- Modify: `ui/src/resources/config.ts`

**Interfaces:**
- Consumes: the JSON from PR 2.
- Produces: `OsbView`, `OsbStatus`, `SandboxOsbDetail` types; `ResourceSummary` extra fields; `ListResponse.osb`; `fetchSandboxOsb`; `ResourceConfig.showOsb`; `fetchList` params `creator`/`osbState`/`stale`.

- [ ] **Step 1: Add the types**

In `ui/src/api/client.ts`, add above `ResourceSummary`:

```ts
export type OsbState =
  | 'Pending'
  | 'Running'
  | 'Pausing'
  | 'Paused'
  | 'Resuming'
  | 'Stopping'
  | 'Terminated'
  | 'Failed';

export interface OsbView {
  state: OsbState | string;
  reason?: string;
  message?: string;
  expiresAt?: string;
  lastTransitionAt?: string;
  stateAgeSeconds: number;
  /** OpenSandbox's state disagrees with the Kubernetes Ready condition. */
  diverged: boolean;
  /** A transient OpenSandbox state has not advanced within the threshold. */
  stale: boolean;
}

export interface OsbStatus {
  status: 'ok' | 'unreachable';
  error?: string;
  fetchedAt?: string;
  reported: number;
  matched: number;
}

export interface SandboxOsbDetail {
  id: string;
  summary: string;
  events: string;
}
```

Extend `ResourceSummary` and `ListResponse`:

```ts
export interface ResourceSummary {
  name: string;
  namespace: string;
  kind: 'Sandbox' | 'SandboxClaim' | 'SandboxTemplate' | 'SandboxWarmPool';
  phase: '' | 'Ready' | 'NotReady' | 'Unknown' | 'Scaling';
  ageSeconds: number;
  /** Sandboxes only. */
  creator?: string;
  owner?: string;
  team?: string;
  experiment?: string;
  osb?: OsbView;
}

export interface ListResponse {
  items: ResourceSummary[];
  /** Absent when no OpenSandbox URL is configured. */
  osb?: OsbStatus;
}
```

Replace `fetchList` with the filter-aware version:

```ts
export async function fetchList(
  kind: ResourceKind,
  params: {
    namespace?: string;
    phase?: string;
    creator?: string;
    osbState?: string;
    stale?: boolean;
  } = {},
): Promise<ListResponse> {
  const q = new URLSearchParams();
  if (params.namespace) q.set('namespace', params.namespace);
  if (params.phase) q.set('phase', params.phase);
  if (params.creator) q.set('creator', params.creator);
  if (params.osbState) q.set('osbState', params.osbState);
  if (params.stale) q.set('stale', 'true');
  const qs = q.toString();
  const url = `/api/v1/${kind}${qs ? `?${qs}` : ''}`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${kind} list failed: ${res.status}`);
  return res.json();
}
```

Append the diagnostics fetcher:

```ts
export async function fetchSandboxOsb(
  namespace: string,
  name: string,
): Promise<SandboxOsbDetail> {
  const res = await fetch(`/api/v1/sandboxes/${namespace}/${name}/osb`);
  if (res.status === 503) throw new Error('opensandbox-unconfigured');
  if (res.status === 404) throw new Error('not-an-opensandbox-sandbox');
  if (!res.ok) throw new Error(`opensandbox detail failed: ${res.status}`);
  return res.json();
}
```

- [ ] **Step 2: Add `showOsb` to the resource config**

In `ui/src/resources/config.ts`, extend the interface and the `sandboxes` entry only:

```ts
export interface ResourceConfig {
  kind: ResourceKind;
  label: string;          // human label in nav
  singular: string;       // human label in drawer / detail
  showPhase: boolean;     // false for templates (no Ready cond)
  phases: string[];       // filter values for the phase dropdown; [] for templates
  showOsb: boolean;       // true only for sandboxes: creator + OpenSandbox columns
  osbStates: string[];    // filter values for the OSB state dropdown
}
```

Set `showOsb: true` plus the state list on `sandboxes`, and `showOsb: false, osbStates: []` on `claims`, `templates` and `warmpools`:

```ts
  sandboxes: {
    kind: 'sandboxes',
    label: 'Sandboxes',
    singular: 'Sandbox',
    showPhase: true,
    phases: ['Ready', 'NotReady', 'Unknown'],
    showOsb: true,
    osbStates: [
      'Pending', 'Running', 'Pausing', 'Paused',
      'Resuming', 'Stopping', 'Terminated', 'Failed',
    ],
  },
```

- [ ] **Step 3: Verify types and lint**

Run: `cd ui && npx tsc -b && npm run lint`
Expected: no type errors, no lint errors. `ResourceList.tsx` still compiles because it does not yet read the new fields.

- [ ] **Step 4: Commit**

```bash
git add ui/src/api/client.ts ui/src/resources/config.ts
git commit -m "feat: OpenSandbox types and filters in the UI API client"
```

---

### Task 12: Sandbox list columns, markers, filters, and banner

**Files:**
- Modify: `ui/src/pages/ResourceList.tsx`

**Interfaces:**
- Consumes: `OsbView`, `OsbStatus`, `showOsb`, `osbStates`, `fetchList` from Task 11.
- Produces: `OsbStatePill` component.

- [ ] **Step 1: Read the new filters from the URL and pass them to the query**

In `ResourceListPage`, after the existing `phaseFilter` line:

```tsx
  const creatorFilter = searchParams.get('creator') ?? '';
  const osbStateFilter = searchParams.get('osbState') ?? '';
  const staleOnly = searchParams.get('stale') === 'true';
```

Update the query to include them in both the key and the call:

```tsx
  const { data, isLoading, error } = useQuery({
    queryKey: ['list', kind, nsFilter, phaseFilter, creatorFilter, osbStateFilter, staleOnly],
    queryFn: () =>
      fetchList(kind, {
        namespace: nsFilter || undefined,
        phase: phaseFilter || undefined,
        creator: creatorFilter || undefined,
        osbState: osbStateFilter || undefined,
        stale: staleOnly || undefined,
      }),
    refetchInterval: 5_000,
  });
```

Widen `updateFilter`'s key type:

```tsx
  const updateFilter = (key: 'namespace' | 'phase' | 'creator' | 'osbState' | 'stale', value: string) => {
```

- [ ] **Step 2: Add the filter controls**

After the existing phase `<select>` block, inside the sticky header div:

```tsx
          {cfg.showOsb && (
            <>
              <select
                className="border border-slate-300 rounded px-2 py-1 text-sm"
                value={creatorFilter}
                onChange={(e) => updateFilter('creator', e.target.value)}
              >
                <option value="">any creator</option>
                <option value="opensandbox">opensandbox</option>
                <option value="unknown">unknown</option>
              </select>
              <select
                className="border border-slate-300 rounded px-2 py-1 text-sm"
                value={osbStateFilter}
                onChange={(e) => updateFilter('osbState', e.target.value)}
              >
                <option value="">any OSB state</option>
                {cfg.osbStates.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
              <label className="flex items-center gap-1 text-sm text-slate-600">
                <input
                  type="checkbox"
                  checked={staleOnly}
                  onChange={(e) => updateFilter('stale', e.target.checked ? 'true' : '')}
                />
                stale only
              </label>
            </>
          )}
```

- [ ] **Step 3: Add the unreachable banner**

Directly above the `{isLoading && …}` line:

```tsx
        {data?.osb?.status === 'unreachable' && (
          <div className="mx-6 mt-3 rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
            OpenSandbox is unreachable — showing Kubernetes state only.
          </div>
        )}
        {data?.osb?.status === 'ok' && data.osb.reported !== data.osb.matched && (
          <div className="mx-6 mt-3 rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
            OpenSandbox reported {data.osb.reported} sandboxes; {data.osb.matched} matched a
            Kubernetes resource.
          </div>
        )}
```

- [ ] **Step 4: Add the columns**

In `<thead>`, after the Phase header:

```tsx
                {cfg.showOsb && <th className="px-3 py-2">OSB State</th>}
                {cfg.showOsb && <th className="px-3 py-2">Creator</th>}
                {cfg.showOsb && <th className="px-3 py-2">Owner</th>}
```

In `<tbody>`, after the Phase cell:

```tsx
                    {cfg.showOsb && (
                      <td className="px-3 py-2">
                        <OsbStatePill osb={it.osb} />
                      </td>
                    )}
                    {cfg.showOsb && (
                      <td className="px-3 py-2 text-slate-600">{it.creator ?? '—'}</td>
                    )}
                    {cfg.showOsb && (
                      <td className="px-3 py-2 text-slate-600">{it.owner || '—'}</td>
                    )}
```

- [ ] **Step 5: Add the `OsbStatePill` component**

Next to `PhasePill` at the bottom of the file, and add `OsbView` to the type import from `../api/client`:

```tsx
function OsbStatePill({ osb }: { osb?: OsbView }) {
  if (!osb) return <span className="text-slate-400">—</span>;

  const cls =
    osb.state === 'Running'
      ? 'bg-emerald-100 text-emerald-800'
      : osb.state === 'Failed' || osb.state === 'Terminated'
      ? 'bg-red-100 text-red-800'
      : 'bg-amber-100 text-amber-800';

  return (
    <span className="inline-flex items-center gap-1">
      <span className={`px-2 py-0.5 rounded text-xs ${cls}`}>{osb.state}</span>
      {osb.diverged && (
        <span title="OpenSandbox disagrees with the Kubernetes Ready condition">⚠</span>
      )}
      {osb.stale && (
        <span
          className="text-xs text-red-700 tabular-nums"
          title={`OpenSandbox has not advanced this state in ${formatAge(osb.stateAgeSeconds)}`}
        >
          ⏱ {formatAge(osb.stateAgeSeconds)}
        </span>
      )}
    </span>
  );
}
```

- [ ] **Step 6: Verify types, lint, and build**

Run: `cd ui && npx tsc -b && npm run lint && npm run build`
Expected: no type errors, no lint errors, build succeeds.

- [ ] **Step 7: Commit**

```bash
git add ui/src/pages/ResourceList.tsx
git commit -m "feat: OSB state, creator and owner columns in the sandbox list"
```

---

### Task 13: OpenSandbox section in the detail drawer, and end-to-end verification

**Files:**
- Modify: `ui/src/components/DetailDrawer.tsx`

**Interfaces:**
- Consumes: `fetchSandboxOsb`, `SandboxOsbDetail` from Task 11.
- Produces: `OsbSection` component.

- [ ] **Step 1: Add the section component**

In `ui/src/components/DetailDrawer.tsx`, add `fetchSandboxOsb` and `type SandboxOsbDetail` to the imports from `../api/client`, then add at the bottom of the file:

```tsx
function OsbSection({ namespace, name }: { namespace: string; name: string }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['osb-detail', namespace, name],
    queryFn: () => fetchSandboxOsb(namespace, name),
    refetchInterval: 10_000,
    retry: false,
  });

  // Not an OpenSandbox sandbox, or OpenSandbox is not configured: show nothing
  // rather than an error the operator can do nothing about.
  const msg = (error as Error | null)?.message;
  if (msg === 'not-an-opensandbox-sandbox' || msg === 'opensandbox-unconfigured') return null;

  return (
    <section>
      <h3 className="text-sm font-semibold mb-2">OpenSandbox</h3>
      {isLoading && <div className="text-sm text-slate-500">Loading…</div>}
      {error && <div className="text-sm text-red-700">{msg}</div>}
      {data && (
        <>
          <div className="text-xs text-slate-500 mb-1">id: {data.id}</div>
          <pre className="text-xs bg-slate-50 border border-slate-200 rounded p-2 overflow-x-auto whitespace-pre">
            {data.summary}
          </pre>
          <h4 className="text-xs font-semibold mt-3 mb-1">OpenSandbox events</h4>
          <pre className="text-xs bg-slate-50 border border-slate-200 rounded p-2 overflow-x-auto whitespace-pre">
            {data.events}
          </pre>
        </>
      )}
    </section>
  );
}
```

- [ ] **Step 2: Render it for sandboxes only**

`SandboxBody` needs the namespace and name, which it does not currently receive. Change its signature and call site.

In the drawer body, replace the sandboxes line:

```tsx
          {kind === 'sandboxes' && (
            <SandboxBody d={data as SandboxDetail} namespace={namespace} name={name} />
          )}
```

And change `SandboxBody` to accept them, adding `OsbSection` after the existing Events section:

```tsx
function SandboxBody({
  d,
  namespace,
  name,
}: {
  d: SandboxDetail;
  namespace: string;
  name: string;
}) {
  return (
    <>
      <section>
        <h3 className="text-sm font-semibold mb-2">Status</h3>
        <ConditionsTable conditions={d.conditions} />
        <dl className="mt-3 grid grid-cols-2 gap-x-3 text-sm">
          <dt className="text-slate-500">Replicas</dt>
          <dd className="tabular-nums">{d.replicas}</dd>
          <dt className="text-slate-500">Service</dt>
          <dd className="break-all">{d.serviceFqdn || '—'}</dd>
          <dt className="text-slate-500">Pod IPs</dt>
          <dd>{(d.podIPs ?? []).join(', ') || '—'}</dd>
        </dl>
      </section>
      <YamlBlock value={d.spec} />
      <section>
        <h3 className="text-sm font-semibold mb-2">Events</h3>
        <EventsList events={d.events} />
      </section>
      <OsbSection namespace={namespace} name={name} />
    </>
  );
}
```

- [ ] **Step 3: Verify types, lint, and build**

Run: `cd ui && npx tsc -b && npm run lint && npm run build`
Expected: no type errors, no lint errors, build succeeds.

- [ ] **Step 4: Full build and Go test suite**

Run from the repo root: `make lint && make test && make build`
Expected: all PASS, `./dashboard` produced with the SPA embedded.

- [ ] **Step 5: Manual end-to-end check against algo-studio**

Local rancher-desktop has no OpenSandbox server, so verify read-only against algo-studio.

```bash
# terminal 1 — reach the OpenSandbox server
kubectl --context gke_algo-studio-main_us-central1_main -n default \
  port-forward svc/opensandbox-server 18080:80

# terminal 2 — run the dashboard against that cluster
OPENSANDBOX_URL=http://127.0.0.1:18080 \
OPENSANDBOX_API_KEY=$(kubectl --context gke_algo-studio-main_us-central1_main -n default \
  get secret opensandbox-server-api-key -o jsonpath='{.data.api-key}' | base64 -d) \
./dashboard --kubeconfig=$HOME/.kube/config
```

Open http://localhost:8080/sandboxes and confirm:

1. Every OpenSandbox row shows `Creator: opensandbox`, an owner where the label exists, and an **OSB State** of `Running`.
2. Rows from other creators show `unknown` and `—` in the OSB State column.
3. No `reported ≠ matched` banner appears — the join is complete.
4. Opening a sandbox shows the **OpenSandbox** section with summary and event text; opening a non-OpenSandbox sandbox shows no such section.
5. Stop the port-forward and reload: rows keep their Kubernetes phase, creator and owner, the OSB State column falls back to `—`, and the amber "OpenSandbox is unreachable" banner appears. Restart the port-forward and confirm it recovers.

**Expect the `⚠` and `⏱` markers to be absent.** A healthy fleet produces no divergence and no staleness, and their absence here is not evidence they are broken — those paths are covered by the Go tests seeded from the recorded incident. To see them live you would need OpenSandbox's watch to break again, which is not something to induce.

- [ ] **Step 6: Commit and open PR 3**

```bash
git add ui/src/components/DetailDrawer.tsx
git commit -m "feat: OpenSandbox diagnostics section in the sandbox drawer"
git push -u origin feat/osb-ui
gh pr create --title "feat: show OpenSandbox state in the dashboard UI" \
  --body "Third of three PRs for the OpenSandbox view (spec: docs/superpowers/specs/2026-08-17-opensandbox-view-design.md).

Adds the OSB State, Creator and Owner columns to the sandbox list, with \`⚠\` for divergence and \`⏱\` for staleness, filters for creator/state/stale-only, an unreachable banner, and an OpenSandbox diagnostics section in the detail drawer.

All rules are computed server-side in PR 2; this only renders them. Verified with \`tsc -b\`, \`npm run lint\`, and a manual read-only run against algo-studio including an OpenSandbox-unreachable check. This repo has no JS test runner, so there are no unit tests here — adding one is a separate decision.

🤖 Generated with [Claude Code](https://claude.com/claude-code)"
```

---

## Self-review notes

**Spec coverage.** Every spec section maps to a task: data split → 5; `internal/osb` incl. pagination cap, read-only, cache → 1–4; divergence → 6; staleness → 7; API changes, `osb.status`, filters, drawer route → 8–9; UI → 11–13; configuration → 10; testing → the test steps throughout; verification → 13 step 5; PR boundaries → the three PR groupings.

**Deliberately not built**, per the spec's Out of scope: `diagnostics/logs`, per-creator Overview counts, `/v1/pools`, `/v1/snapshots`, and any write action.

**Two known rough edges**, called out rather than hidden:

- Task 5 leaves the tree briefly uncompilable because `ResourceSummary.Osb` references `OsbView`, defined in Task 6. The step says so and gives the workaround. Splitting differently would mean either a one-line task or a much larger one.
- Task 10's `osbAdapter` exists because `osb.Cache` covers only `ListSandboxes` while `server.OsbClient` also needs `Diagnostics`. Caching per-sandbox diagnostics was not worth it — they are fetched only when a drawer opens.
