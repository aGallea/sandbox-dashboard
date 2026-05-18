//go:build integration

package server_test

import (
	"context"
	"encoding/json"
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

	"github.com/aGallea/agent-sandbox-dashboard/internal/k8s"
	"github.com/aGallea/agent-sandbox-dashboard/internal/server"
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

func crdPaths(t *testing.T) []string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	out, err := runCmd(t, root, "go", "list", "-m", "-f", "{{.Dir}}", "sigs.k8s.io/agent-sandbox")
	require.NoError(t, err)
	mod := out
	// agent-sandbox v0.4.6 ships CRDs under k8s/crds, NOT config/crd/bases.
	return []string{filepath.Join(mod, "k8s", "crds")}
}

func TestIntegration_OverviewEndToEnd(t *testing.T) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     crdPaths(t),
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err)
	t.Cleanup(func() { _ = env.Stop() })

	scheme := k8s.NewScheme()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)

	// minContainer is the minimum container spec required by CRD validation.
	minContainer := corev1.Container{Name: "agent", Image: "busybox:latest"}
	// minPodTemplate satisfies the required spec.podTemplate.spec.containers field.
	minPodTemplate := v1alpha1.PodTemplate{
		Spec: corev1.PodSpec{Containers: []corev1.Container{minContainer}},
	}

	ctx := context.Background()
	require.NoError(t, c.Create(ctx, &v1alpha1.Sandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
		Spec:       v1alpha1.SandboxSpec{PodTemplate: minPodTemplate},
	}))
	require.NoError(t, c.Create(ctx, &extv1alpha1.SandboxTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
		Spec:       extv1alpha1.SandboxTemplateSpec{PodTemplate: minPodTemplate},
	}))
	require.NoError(t, c.Create(ctx, &extv1alpha1.SandboxClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "default"},
		Spec: extv1alpha1.SandboxClaimSpec{
			TemplateRef: extv1alpha1.SandboxTemplateRef{Name: "t1"},
		},
	}))
	require.NoError(t, c.Create(ctx, &extv1alpha1.SandboxWarmPool{
		ObjectMeta: metav1.ObjectMeta{Name: "w1", Namespace: "default"},
		Spec: extv1alpha1.SandboxWarmPoolSpec{
			Replicas:    1,
			TemplateRef: extv1alpha1.SandboxTemplateRef{Name: "t1"},
		},
	}))

	time.Sleep(100 * time.Millisecond)

	r := server.New(server.Deps{Client: c, CacheSynced: func() bool { return true }})

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
