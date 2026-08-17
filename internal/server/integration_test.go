//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/aGallea/sandbox-dashboard/internal/k8s"
	"github.com/aGallea/sandbox-dashboard/internal/server"
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

func crdPaths(t *testing.T) []string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	out, err := runCmd(t, root, "go", "list", "-m", "-f", "{{.Dir}}", "sigs.k8s.io/agent-sandbox")
	require.NoError(t, err)
	// agent-sandbox v0.4.6 ships CRDs under k8s/crds, NOT config/crd/bases.
	return []string{filepath.Join(out, "k8s", "crds")}
}

func TestIntegration_OverviewAndDetailViaCachedClient(t *testing.T) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     crdPaths(t),
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err)
	t.Cleanup(func() { _ = env.Stop() })

	// Build a real manager (the production code path).
	mgr, err := k8s.NewManager(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mgrErr := make(chan error, 1)
	go func() { mgrErr <- mgr.Start(ctx) }()

	require.True(t, mgr.GetCache().WaitForCacheSync(ctx), "manager cache must sync")

	// Create fixtures with an uncached direct client — synchronous writes.
	directClient, err := client.New(cfg, client.Options{Scheme: k8s.NewScheme()})
	require.NoError(t, err)

	require.NoError(t, directClient.Create(ctx, mkSandbox("s1", "default")))
	require.NoError(t, directClient.Create(ctx, mkTemplate("t1", "default")))
	require.NoError(t, directClient.Create(ctx, mkClaim("c1", "default", "t1")))
	require.NoError(t, directClient.Create(ctx, mkWarmPool("w1", "default", "t1")))

	// Wait for all four informers to observe the writes — each kind has an
	// independent watch and there is no global "cache caught up" signal.
	cached := mgr.GetClient()
	require.Eventually(t, func() bool {
		var sbs v1alpha1.SandboxList
		var cs extv1alpha1.SandboxClaimList
		var ts extv1alpha1.SandboxTemplateList
		var ws extv1alpha1.SandboxWarmPoolList
		return cached.List(ctx, &sbs) == nil && len(sbs.Items) == 1 &&
			cached.List(ctx, &cs) == nil && len(cs.Items) == 1 &&
			cached.List(ctx, &ts) == nil && len(ts.Items) == 1 &&
			cached.List(ctx, &ws) == nil && len(ws.Items) == 1
	}, 5*time.Second, 50*time.Millisecond, "all four informers must observe their fixtures")

	r := server.New(server.Deps{
		Client:      mgr.GetClient(),
		CacheSynced: func() bool { return true },
		Logger:      slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	})

	// Overview
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		t.Logf("overview: %+v", got)
		require.EqualValues(t, 1, got["sandboxes"].(map[string]any)["total"])
		require.EqualValues(t, 1, got["claims"].(map[string]any)["total"])
		require.EqualValues(t, 1, got["templates"].(map[string]any)["total"])
		require.EqualValues(t, 1, got["warmPools"].(map[string]any)["total"])
	}

	// Sandbox detail
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/s1", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		summary := got["summary"].(map[string]any)
		require.Equal(t, "s1", summary["name"])
		require.Equal(t, "Sandbox", summary["kind"])
	}

	// 404 path
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/missing", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}

	cancel()
	<-mgrErr
}

// minContainer is the minimum container spec required by CRD validation.
var minContainer = corev1.Container{Name: "agent", Image: "busybox:latest"}

// minPodTemplate satisfies the required spec.podTemplate.spec.containers field.
var minPodTemplate = v1alpha1.PodTemplate{
	Spec: corev1.PodSpec{Containers: []corev1.Container{minContainer}},
}

func mkSandbox(name, ns string) *v1alpha1.Sandbox {
	return &v1alpha1.Sandbox{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       v1alpha1.SandboxSpec{PodTemplate: minPodTemplate},
	}
}

func mkTemplate(name, ns string) *extv1alpha1.SandboxTemplate {
	return &extv1alpha1.SandboxTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       extv1alpha1.SandboxTemplateSpec{PodTemplate: minPodTemplate},
	}
}

func mkClaim(name, ns, templateRef string) *extv1alpha1.SandboxClaim {
	return &extv1alpha1.SandboxClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: extv1alpha1.SandboxClaimSpec{
			TemplateRef: extv1alpha1.SandboxTemplateRef{Name: templateRef},
		},
	}
}

func mkWarmPool(name, ns, templateRef string) *extv1alpha1.SandboxWarmPool {
	return &extv1alpha1.SandboxWarmPool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: extv1alpha1.SandboxWarmPoolSpec{
			Replicas:    1,
			TemplateRef: extv1alpha1.SandboxTemplateRef{Name: templateRef},
		},
	}
}

func runCmd(t *testing.T, dir, name string, args ...string) (string, error) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	b, err := c.Output()
	return string(stripTrailingNL(b)), err
}

func stripTrailingNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
