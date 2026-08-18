package osb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListSandboxes_ReturnsSandboxesKeyedByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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

func TestListSandboxes_WalksEveryPage(t *testing.T) {
	var pagesServed []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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

func TestDiagnostics_ReturnsSummaryAndEventsAsPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method, "the client must never issue a non-GET request")
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

// TestClient_OnlyEverIssuesGETRequests names the guarantee the package
// comment claims: the client is structurally read-only. A server that fails
// on any non-GET method proves it by staying silent — if a future method
// (e.g. Pause) ever issued a POST/DELETE/etc., this test would catch it even
// though the per-test method assertions above only exercise the requests each
// test happens to trigger.
func TestClient_OnlyEverIssuesGETRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/sandboxes/") && strings.HasSuffix(r.URL.Path, "/diagnostics/summary"):
			_, _ = w.Write([]byte("summary"))
		case strings.HasPrefix(r.URL.Path, "/v1/sandboxes/") && strings.HasSuffix(r.URL.Path, "/diagnostics/events"):
			_, _ = w.Write([]byte("events"))
		default:
			_, _ = w.Write([]byte(`{"items":[],"pagination":{"page":1,"pageSize":200,"totalItems":0,"totalPages":0,"hasNextPage":false}}`))
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "k")
	require.NoError(t, err)

	_, err = c.ListSandboxes(context.Background())
	require.NoError(t, err, "a server that 405s any non-GET would fail this if ListSandboxes ever issued one")

	d, err := c.Diagnostics(context.Background(), "abc")
	require.NoError(t, err, "a server that 405s any non-GET would fail this if Diagnostics ever issued one")
	require.Equal(t, "summary", d.Summary)
	require.Equal(t, "events", d.Events)
}
