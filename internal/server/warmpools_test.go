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

	"github.com/aGallea/agent-sandbox-dashboard/internal/k8s"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

func TestWarmPools_List_PhaseMapping(t *testing.T) {
	objs := []client.Object{
		&extv1alpha1.SandboxWarmPool{
			ObjectMeta: metav1.ObjectMeta{Name: "w-ready", Namespace: "ns1"},
			Status:     extv1alpha1.SandboxWarmPoolStatus{Replicas: 3, ReadyReplicas: 3},
		},
		&extv1alpha1.SandboxWarmPool{
			ObjectMeta: metav1.ObjectMeta{Name: "w-scaling", Namespace: "ns1"},
			Status:     extv1alpha1.SandboxWarmPoolStatus{Replicas: 3, ReadyReplicas: 1},
		},
		&extv1alpha1.SandboxWarmPool{
			ObjectMeta: metav1.ObjectMeta{Name: "w-empty", Namespace: "ns1"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/warmpools", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Items []ResourceSummary `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 3)

	phases := map[string]string{}
	for _, it := range got.Items {
		phases[it.Name] = it.Phase
	}
	require.Equal(t, "Ready", phases["w-ready"])
	require.Equal(t, "Scaling", phases["w-scaling"])
	require.Equal(t, "Unknown", phases["w-empty"])
}

func TestWarmPools_Detail_404AndOK(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(
		&extv1alpha1.SandboxWarmPool{
			ObjectMeta: metav1.ObjectMeta{Name: "w1", Namespace: "ns1"},
			Status:     extv1alpha1.SandboxWarmPoolStatus{Replicas: 2, ReadyReplicas: 2},
		},
	).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	// 404
	req := httptest.NewRequest(http.MethodGet, "/api/v1/warmpools/ns1/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// 200
	req = httptest.NewRequest(http.MethodGet, "/api/v1/warmpools/ns1/w1", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got WarmPoolDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "Ready", got.Summary.Phase)
	require.Equal(t, int32(2), got.Replicas)
	require.Equal(t, int32(2), got.ReadyReplicas)
}
