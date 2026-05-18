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

func TestTemplates_List_ByNamespace(t *testing.T) {
	objs := []client.Object{
		&extv1alpha1.SandboxTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "ns1"}},
		&extv1alpha1.SandboxTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t2", Namespace: "ns2"}},
	}
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Items []ResourceSummary `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 2)
	for _, it := range got.Items {
		require.Equal(t, "SandboxTemplate", it.Kind)
		require.Equal(t, "", it.Phase, "templates have no Ready phase")
	}
}

func TestTemplates_Detail_404AndOK(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(
		&extv1alpha1.SandboxTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "ns1"}},
	).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	// 404
	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates/ns1/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// 200
	req = httptest.NewRequest(http.MethodGet, "/api/v1/templates/ns1/t1", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got TemplateDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "t1", got.Summary.Name)
	require.NotNil(t, got.Spec)
}
