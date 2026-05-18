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
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
)

func TestSandboxes_List_FiltersByNamespaceAndPhase(t *testing.T) {
	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue}
	notReady := metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse}
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1"},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{ready}},
		},
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns1"},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{notReady}},
		},
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns2"},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{ready}},
		},
	}
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	tests := []struct {
		path string
		want []string
	}{
		{"/api/v1/sandboxes", []string{"a", "b", "c"}},
		{"/api/v1/sandboxes?namespace=ns1", []string{"a", "b"}},
		{"/api/v1/sandboxes?namespace=ns1&phase=Ready", []string{"a"}},
		{"/api/v1/sandboxes?phase=NotReady", []string{"b"}},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			var got struct {
				Items []ResourceSummary `json:"items"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			names := make([]string, len(got.Items))
			for i := range got.Items {
				names[i] = got.Items[i].Name
			}
			require.ElementsMatch(t, tc.want, names)
		})
	}
}

func TestSandboxes_Detail_IncludesSpecStatusEvents(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "my-sb", Namespace: "ns1"},
			Status: v1alpha1.SandboxStatus{
				Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllUp"}},
				Replicas:   1,
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/ns1/my-sb", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got SandboxDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "my-sb", got.Summary.Name)
	require.Equal(t, "Ready", got.Summary.Phase)
	require.NotNil(t, got.Spec)
	require.Len(t, got.Conditions, 1)
	require.Equal(t, "AllUp", got.Conditions[0].Reason)
	require.Equal(t, int32(1), got.Replicas)
	require.NotNil(t, got.Events)
}

func TestSandboxes_Detail_404OnMissing(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/ns1/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}
