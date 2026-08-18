package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aGallea/sandbox-dashboard/internal/k8s"
	"github.com/aGallea/sandbox-dashboard/internal/osb"
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

// TestSandboxes_Detail_IncludesIdentityAndOsbView pins that the detail
// handler carries the same identity and OpenSandbox fields as the list
// handler, not just id/spec/events. Before this fix, an operator opening the
// drawer for a `Pending ⚠ 9m ⏱` row saw no reason, message, or
// lastTransitionAt anywhere — only the on-demand diagnostics, which is the
// side that stayed correct during the incident.
func TestSandboxes_Detail_IncludesIdentityAndOsbView(t *testing.T) {
	created := time.Date(2026, 8, 17, 14, 8, 57, 0, time.UTC)
	observed := time.Date(2026, 8, 17, 14, 11, 40, 0, time.UTC)

	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sandbox-726f8779", Namespace: "default",
				Labels: map[string]string{
					OsbIDLabel:   "726f8779",
					"session_id": "regex-chess__33BjxVG__env",
				},
			},
			Status: v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	o := &fakeOsb{list: map[string]osb.Sandbox{
		"726f8779": {
			ID: "726f8779",
			Status: osb.Status{
				State: "Pending", Reason: "SANDBOX_PENDING",
				Message: "Sandbox is pending scheduling", LastTransitionAt: &created,
			},
		},
	}}

	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, o, observed).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/sandbox-726f8779", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got SandboxDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	require.Equal(t, CreatorOpenSandbox, got.Summary.Creator)
	require.Equal(t, "regex-chess__33BjxVG__env", got.Summary.SessionID)
	require.NotNil(t, got.Summary.Osb)
	require.Equal(t, "Pending", got.Summary.Osb.State)
	require.Equal(t, "SANDBOX_PENDING", got.Summary.Osb.Reason)
	require.Equal(t, "Sandbox is pending scheduling", got.Summary.Osb.Message)
	require.True(t, got.Summary.Osb.Diverged, "OpenSandbox Pending against a Ready pod is a disagreement")
	require.True(t, got.Summary.Osb.Stale, "the state had not moved in 2m43s")
}

// TestSandboxes_Detail_OsbFailureDoesNotFailTheResponse mirrors the list
// handler's contract: an OpenSandbox outage must never turn a working CR read
// into an error response.
func TestSandboxes_Detail_OsbFailureDoesNotFailTheResponse(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sandbox-abc", Namespace: "default",
				Labels: map[string]string{OsbIDLabel: "abc"},
			},
			Status: v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	o := &fakeOsb{err: errors.New("dial tcp: connection refused")}

	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, o, time.Now()).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/sandbox-abc", nil))

	require.Equal(t, http.StatusOK, rec.Code, "an OpenSandbox outage must not fail the detail response")
	var got SandboxDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Nil(t, got.Summary.Osb)
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
		Client:        c,
		CacheSynced:   func() bool { return true },
		Now:           func() time.Time { return now },
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

	require.NotContains(t, rec.Body.String(), "\"osb\"", "the osb key must be absent from the wire format, not present as null")

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

// TestSandboxes_List_MatchedCountIgnoresDisplayFilters pins down that the join
// (and therefore `matched`) is computed against the whole fleet, not against
// whatever survives the namespace/phase/creator filters. Before this fix,
// filtering out a joinable CR before the join lookup made `matched` undercount
// and fired a false osb_join_incomplete warning even though every CR joined.
func TestSandboxes_List_MatchedCountIgnoresDisplayFilters(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1", Labels: map[string]string{OsbIDLabel: "a"}},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns2", Labels: map[string]string{OsbIDLabel: "b"}},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	o := &fakeOsb{list: map[string]osb.Sandbox{
		"a": {ID: "a", Status: osb.Status{State: "Running"}},
		"b": {ID: "b", Status: osb.Status{State: "Running"}},
	}}

	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, o, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes?namespace=ns1", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got sandboxListBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Items, 1, "the namespace filter still narrows the displayed items")
	require.NotNil(t, got.Osb)
	require.Equal(t, 2, got.Osb.Reported)
	require.Equal(t, 2, got.Osb.Matched, "both CRs joined, even though one was filtered out of the response")
}

// TestSandboxes_List_StaleFilterYieldsEmptyNotErrorWhenOpenSandboxIsUnreachable
// pins the contract for the outage+filter combination: osbState/stale filter
// on the joined view, which is nil whenever the inventory could not be
// fetched, so they report an empty items list rather than an error. Callers
// must consult osb.status to tell "computed as empty" from "could not compute".
func TestSandboxes_List_StaleFilterYieldsEmptyNotErrorWhenOpenSandboxIsUnreachable(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default", Labels: map[string]string{OsbIDLabel: "a"}},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	o := &fakeOsb{err: errors.New("dial tcp: connection refused")}
	h := osbTestDeps(t, objs, o, time.Now())

	for _, path := range []string{"/api/v1/sandboxes?stale=true", "/api/v1/sandboxes?osbState=Running"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, rec.Code)

			var got sandboxListBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Empty(t, got.Items, "with no inventory, every view is nil so the filter matches nothing")
			require.NotNil(t, got.Osb)
			require.Equal(t, "unreachable", got.Osb.Status, "status distinguishes an empty result from an outage")
		})
	}
}

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
	require.Contains(t, rec.Body.String(), "not-an-opensandbox-sandbox",
		"must be the not-an-OpenSandbox-sandbox 404, not the sandbox-not-found 404 or chi's catch-all")
}

func TestSandboxOsb_Returns404WithDistinctSlugWhenSandboxAbsent(t *testing.T) {
	rec := httptest.NewRecorder()
	osbTestDeps(t, []client.Object{}, &fakeOsb{}, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/never-created/osb", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "sandbox-not-found")
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

func TestSandboxes_List_ExposesSessionIDForThinlyLabelledSandboxes(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sandbox-abc", Namespace: "default",
				Labels: map[string]string{
					OsbIDLabel:   "abc",
					"session_id": "regex-chess__33BjxVG__env",
				},
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
	require.Equal(t, "regex-chess__33BjxVG__env", got.Items[0].SessionID)
	require.Empty(t, got.Items[0].Owner, "this fleet stamps no owner label")
}

func TestSandboxes_List_OmitsSessionIDWhenLabelAbsent(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{
			ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "default"},
			Status:     v1alpha1.SandboxStatus{Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}},
		},
	}
	rec := httptest.NewRecorder()
	osbTestDeps(t, objs, nil, time.Now()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil))

	require.NotContains(t, rec.Body.String(), "sessionId", "the key must be omitted, not empty")
}
