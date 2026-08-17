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
