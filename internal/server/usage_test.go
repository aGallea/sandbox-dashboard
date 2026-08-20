package server

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aGallea/sandbox-dashboard/internal/k8s"
	"github.com/aGallea/sandbox-dashboard/internal/prom"
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
)

// stubQuerier answers instant queries from canned samples, keyed by a substring
// of the query, and records every query it was asked.
type stubQuerier struct {
	stubProm
	cpu      []prom.Sample
	mem      []prom.Sample
	queryErr error
}

func (s *stubQuerier) Query(_ context.Context, q string, _ time.Time) ([]prom.Sample, error) {
	s.mu.Lock()
	s.asked = append(s.asked, q)
	s.mu.Unlock()
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	if strings.Contains(q, "container_cpu_usage_seconds_total") {
		return s.cpu, nil
	}
	return s.mem, nil
}

func (s *stubQuerier) queries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

func podSample(namespace, pod string, value float64) prom.Sample {
	return prom.Sample{Labels: map[string]string{"namespace": namespace, "pod": pod}, Value: value}
}

func usageRequest(t *testing.T, objs []client.Object, p *stubQuerier) *httptest.ResponseRecorder {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	d := Deps{Client: c, CacheSynced: func() bool { return true }}
	if p != nil {
		d.Prom = p
	}
	rec := httptest.NewRecorder()
	New(d).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil))
	return rec
}

func TestUsage_ReportsUsageForSandboxPodsOnly(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi")),
	}
	p := &stubQuerier{
		cpu: []prom.Sample{podSample("default", "pod-a", 0.052), podSample("monitoring", "loki-0", 3)},
		mem: []prom.Sample{podSample("default", "pod-a", 334_000_000), podSample("monitoring", "loki-0", 7e9)},
	}

	rec := usageRequest(t, objs, p)
	require.Equal(t, http.StatusOK, rec.Code)

	var got UsageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Pods, 1, "a pod no sandbox owns must not appear")
	require.Equal(t, 0.052, got.Pods["default/pod-a"].CPUCores)
	require.Equal(t, 334_000_000.0, got.Pods["default/pod-a"].MemBytes)

	// Both queries are scoped to the namespace the sandbox lives in.
	for _, q := range p.queries() {
		require.Contains(t, q, `namespace=~"default"`)
	}
}

func TestUsage_ReportsAPodMissingFromOneQuery(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi")),
	}
	// A pod that has just started has a memory reading but no rate() yet.
	p := &stubQuerier{mem: []prom.Sample{podSample("default", "pod-a", 1024)}}

	rec := usageRequest(t, objs, p)
	require.Equal(t, http.StatusOK, rec.Code)

	var got UsageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, 0.0, got.Pods["default/pod-a"].CPUCores)
	require.Equal(t, 1024.0, got.Pods["default/pod-a"].MemBytes)
}

// A NaN would fail JSON encoding after the status line is written, turning one
// odd sample into a broken response.
func TestUsage_SkipsNonFiniteSamples(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi")),
	}
	p := &stubQuerier{
		cpu: []prom.Sample{podSample("default", "pod-a", math.NaN())},
		mem: []prom.Sample{podSample("default", "pod-a", math.Inf(1))},
	}

	rec := usageRequest(t, objs, p)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "NaN")

	var got UsageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Empty(t, got.Pods)
}

func TestUsage_SkipsPrometheusEntirelyWhenNoSandboxHasAPod(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
	}
	p := &stubQuerier{}

	rec := usageRequest(t, objs, p)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, p.queries(), "an unscoped query would sweep the whole cluster for nothing")

	var got UsageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Empty(t, got.Pods)
}

func TestUsage_503WhenPrometheusIsUnconfigured(t *testing.T) {
	rec := usageRequest(t, nil, nil)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "prometheus-unconfigured")
}

func TestUsage_502AndHidesUpstreamDetailWhenAQueryFails(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi")),
	}
	p := &stubQuerier{queryErr: errors.New("dial tcp 10.0.0.1:9090: connection refused")}

	rec := usageRequest(t, objs, p)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "prometheus-unreachable")
	require.NotContains(t, rec.Body.String(), "10.0.0.1")
}
