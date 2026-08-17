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
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

func TestClaims_List_BasicAndFiltered(t *testing.T) {
	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue}
	objs := []client.Object{
		&extv1alpha1.SandboxClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
			Status:     extv1alpha1.SandboxClaimStatus{Conditions: []metav1.Condition{ready}},
		},
		&extv1alpha1.SandboxClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "c2", Namespace: "ns2"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claims?namespace=ns1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Items []ResourceSummary `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	require.Equal(t, "c1", got.Items[0].Name)
	require.Equal(t, "SandboxClaim", got.Items[0].Kind)
}

func TestClaims_Detail_404OnMissing(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claims/ns1/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestClaims_Detail_IncludesTemplateAndBoundStatus(t *testing.T) {
	objs := []client.Object{
		&extv1alpha1.SandboxClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
			Spec: extv1alpha1.SandboxClaimSpec{
				TemplateRef: extv1alpha1.SandboxTemplateRef{Name: "tmpl-a"},
			},
			Status: extv1alpha1.SandboxClaimStatus{
				Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}},
				SandboxStatus: extv1alpha1.SandboxStatus{
					Name:   "bound-sandbox",
					PodIPs: []string{"10.0.0.1"},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	r := New(Deps{Client: c, CacheSynced: func() bool { return true }})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claims/ns1/c1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got ClaimDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "Ready", got.Summary.Phase)
	require.NotNil(t, got.Spec)
	require.Equal(t, "tmpl-a", got.TemplateRef)
	require.Equal(t, "bound-sandbox", got.SandboxStatus.Name)
	require.Equal(t, []string{"10.0.0.1"}, got.SandboxStatus.PodIPs)
}
