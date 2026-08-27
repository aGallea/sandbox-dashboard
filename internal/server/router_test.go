package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

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

func TestSPAFallback_RoutesNonAPIToIndex(t *testing.T) {
	r := New(Deps{
		CacheSynced: func() bool { return true },
		UIAssets:    nopFS{},
	})
	req := httptest.NewRequest(http.MethodGet, "/some/client-side/route", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	// nopFS.Open returns ErrNotExist → ServeFileFS responds 404.
	// Crucial assertion: the response is NOT JSON problem+json (i.e. we routed
	// through the SPA branch, not the /api/ 404 branch).
	require.NotEqual(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

type nopFS struct{}

func (nopFS) Open(_ string) (fs.File, error) { return nil, fs.ErrNotExist }

// The dashboard is deployed from one published image, so where it is mounted
// cannot be a build-time decision: the same binary has to serve under / on one
// cluster and under /sandbox-dashboard on another. The browser is what needs to
// know — it requests the assets and the API — so index.html carries the prefix.
func TestSPA_CarriesTheBasePathToTheBrowser(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			`<!doctype html><html><head><title>t</title></head><body></body></html>`)},
	}

	getAt := func(t *testing.T, basePath, path string) string {
		t.Helper()
		r := New(Deps{UIAssets: assets, BasePath: basePath, CacheSynced: func() bool { return true }})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		return rec.Body.String()
	}

	// The app lives under its prefix, so that is where to ask for it.
	get := func(t *testing.T, basePath string) string {
		t.Helper()
		return getAt(t, basePath, normaliseBasePath(basePath)+"/")
	}

	t.Run("mounted under a prefix, relative asset URLs resolve against it", func(t *testing.T) {
		body := get(t, "/sandbox-dashboard")
		require.Contains(t, body, `<base href="/sandbox-dashboard/">`)
		require.Contains(t, body, `window.__BASE_PATH__ = "/sandbox-dashboard"`)
	})

	// The app is served under the prefix rather than at the root with the proxy
	// rewriting the path: nothing in front has to strip anything, which is how
	// every other route on this cluster's shared tools host works.
	t.Run("serves the app under the prefix, not at the root", func(t *testing.T) {
		r := New(Deps{
			UIAssets: assets, BasePath: "/sandbox-dashboard",
			CacheSynced: func() bool { return true },
		})

		for path, want := range map[string]int{
			"/sandbox-dashboard/":                  http.StatusOK,
			"/sandbox-dashboard/sandboxes/ns/name": http.StatusOK, // deep link
			"/sandbox-dashboard":                   http.StatusMovedPermanently,
			// Probes answer at the root: the kubelet does not go through the proxy.
			"/healthz": http.StatusOK,
			"/readyz":  http.StatusOK,
			// The root is not the app any more.
			"/":            http.StatusNotFound,
			"/sandboxes":   http.StatusNotFound,
			"/api/v1/typo": http.StatusNotFound,
		} {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, want, rec.Code, path)
		}
	})

	// Without this, a deep link like /sandboxes/default/foo would resolve a
	// relative "./assets/x" against /sandboxes/default/ and 404.
	t.Run("mounted at the root, the tag is still emitted", func(t *testing.T) {
		body := getAt(t, "", "/sandboxes/default/foo")
		require.Contains(t, body, `<base href="/">`)
		require.Contains(t, body, `window.__BASE_PATH__ = ""`)
	})

	t.Run("a trailing slash or a missing leading slash is normalised", func(t *testing.T) {
		for _, given := range []string{"sandbox-dashboard", "/sandbox-dashboard/", "sandbox-dashboard/"} {
			require.Contains(t, get(t, given), `<base href="/sandbox-dashboard/">`, given)
		}
	})

	// The prefix lands inside an HTML attribute and a JS string literal, so a
	// value carrying a quote would break out of both.
	// Asserted against serveIndex directly: a prefix carrying these characters
	// cannot be expressed as a request path to route to in the first place.
	t.Run("a hostile prefix cannot break out of the attribute", func(t *testing.T) {
		rec := httptest.NewRecorder()
		serveIndex(rec, httptest.NewRequest(http.MethodGet, "/", nil), Deps{
			UIAssets: assets, BasePath: `/x"><script>alert(1)</script>`,
		})
		require.NotContains(t, rec.Body.String(), "<script>alert(1)</script>")
	})
}
