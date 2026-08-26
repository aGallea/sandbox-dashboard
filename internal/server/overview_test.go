package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aGallea/sandbox-dashboard/internal/k8s"
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

func TestOverview_AggregatesCountsByPhase(t *testing.T) {
	readyCond := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue}
	notReadyCond := metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse}

	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "ns1"},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{readyCond}},
		},
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "s2", Namespace: "ns1"},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{notReadyCond}},
		},
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "s3", Namespace: "ns2"},
			// no conditions → unknown
		},
		&extv1alpha1.SandboxClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
			Status:     extv1alpha1.SandboxClaimStatus{Conditions: []metav1.Condition{readyCond}},
		},
		&extv1alpha1.SandboxTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "ns1"},
		},
		&extv1alpha1.SandboxWarmPool{
			ObjectMeta: metav1.ObjectMeta{Name: "w1", Namespace: "ns1"},
			Status:     extv1alpha1.SandboxWarmPoolStatus{Replicas: 3, ReadyReplicas: 2},
		},
	}

	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got OverviewResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	require.Equal(t, 3, got.Sandboxes.Total)
	require.Equal(t, 1, got.Sandboxes.Ready)
	require.Equal(t, 1, got.Sandboxes.NotReady)
	require.Equal(t, 1, got.Sandboxes.Unknown)

	require.Equal(t, 1, got.Claims.Total)
	require.Equal(t, 1, got.Claims.Ready)

	require.Equal(t, 1, got.Templates.Total)

	require.Equal(t, 1, got.WarmPools.Total)
	require.Equal(t, int32(3), got.WarmPools.Replicas)
	require.Equal(t, int32(2), got.WarmPools.ReadyReplicas)
}

func TestOverview_503WhenCacheNotSynced(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return false }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

// A narrowed install shows a partial fleet. The scope is in the startup log and
// in NOTES.txt, but nothing reaches the page, so the count reads as the whole
// cluster. The response has to carry the scope for the UI to be able to say so.
func TestOverview_ReportsTheWatchedNamespaceScope(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).Build()

	t.Run("names the namespaces a narrowed install watches", func(t *testing.T) {
		r := New(Deps{
			Client:          c,
			CacheSynced:     func() bool { return true },
			WatchNamespaces: []string{"default", "team-a"},
		})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
		require.Equal(t, http.StatusOK, rec.Code)

		var got OverviewResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Equal(t, []string{"default", "team-a"}, got.Scope.Namespaces)
	})

	// Absent, not an empty list: "watching everything" and "watching nothing"
	// must not serialise the same way.
	t.Run("omits the scope when the install watches every namespace", func(t *testing.T) {
		r := New(Deps{Client: c, CacheSynced: func() bool { return true }})
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.NotContains(t, rec.Body.String(), "namespaces")
	})
}
