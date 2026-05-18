package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthz_AlwaysOK(t *testing.T) {
	r := New(Deps{CacheSynced: func() bool { return false }})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ok", rec.Body.String())
}

func TestReadyz_503UntilCacheSynced(t *testing.T) {
	synced := false
	r := New(Deps{CacheSynced: func() bool { return synced }})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	synced = true
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAPI_404IsJSON(t *testing.T) {
	r := New(Deps{
		CacheSynced: func() bool { return true },
		UIAssets:    nopFS{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/typo", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

type nopFS struct{}

func (nopFS) Open(_ string) (fs.File, error) { return nil, fs.ErrNotExist }
