package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aGallea/sandbox-dashboard/internal/k8s"
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
)

// fakeLogs serves one canned stream and remembers what it was asked for.
type fakeLogs struct {
	content string
	err     error
	gotPod  string
	gotOpts *corev1.PodLogOptions
}

func (f *fakeLogs) PodLogs(_ context.Context, _ string, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	f.gotPod = pod
	f.gotOpts = opts
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

func logsRouter(t *testing.T, logs PodLogStreamer, objs ...client.Object) http.Handler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	return New(Deps{Client: c, CacheSynced: func() bool { return true }, Logs: logs})
}

func sandboxWithPod() []client.Object {
	pod := sandboxPod("sb-a", "default", "uid-a", nil)
	pod.Spec.Containers = []corev1.Container{{Name: "main"}, {Name: "sidecar"}}
	return []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		pod,
	}
}

func getLogs(r http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestSandboxLogs_TailsTheFirstContainerByDefault(t *testing.T) {
	logs := &fakeLogs{content: "one\ntwo\n"}
	rec := getLogs(logsRouter(t, logs, sandboxWithPod()...), "/api/v1/sandboxes/default/sb-a/logs")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, "one\ntwo\n", rec.Body.String())
	require.Equal(t, "sb-a", logs.gotPod)
	require.Equal(t, "main", logs.gotOpts.Container)
	require.NotNil(t, logs.gotOpts.TailLines)
	require.Equal(t, DefaultLogLines, *logs.gotOpts.TailLines)
}

func TestSandboxLogs_TailAndContainerAreForwarded_Capped(t *testing.T) {
	logs := &fakeLogs{}
	rec := getLogs(logsRouter(t, logs, sandboxWithPod()...),
		"/api/v1/sandboxes/default/sb-a/logs?tail=5000&container=sidecar")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "sidecar", logs.gotOpts.Container)
	require.Equal(t, MaxLogLines, *logs.gotOpts.TailLines)
}

func TestSandboxLogs_HeadReadsTheFirstLinesOfTheWholeStream(t *testing.T) {
	logs := &fakeLogs{content: "one\ntwo\nthree\nfour"}
	rec := getLogs(logsRouter(t, logs, sandboxWithPod()...), "/api/v1/sandboxes/default/sb-a/logs?head=2")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, logs.gotOpts.TailLines, "head must not also tail, or the first lines would be the wrong ones")
	require.Equal(t, "one\ntwo\n", rec.Body.String())
}

func TestSandboxLogs_HeadPastTheEndReturnsWhatThereIs(t *testing.T) {
	logs := &fakeLogs{content: "only\nno newline at end"}
	rec := getLogs(logsRouter(t, logs, sandboxWithPod()...), "/api/v1/sandboxes/default/sb-a/logs?head=10")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "only\nno newline at end", rec.Body.String())
}

func TestSandboxLogs_ProblemStatuses(t *testing.T) {
	noPod := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
	}
	waiting := apierrors.NewBadRequest(`container "main" in pod "sb-a" is waiting to start: ContainerCreating`)
	gone := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "sb-a")

	tests := []struct {
		name string
		logs PodLogStreamer
		objs []client.Object
		path string
		want int
	}{
		{"nil streamer", nil, sandboxWithPod(), "/api/v1/sandboxes/default/sb-a/logs", http.StatusServiceUnavailable},
		{"unknown sandbox", &fakeLogs{}, sandboxWithPod(), "/api/v1/sandboxes/default/nope/logs", http.StatusNotFound},
		{"sandbox without a pod", &fakeLogs{}, noPod, "/api/v1/sandboxes/default/sb-a/logs", http.StatusNotFound},
		{"container not started", &fakeLogs{err: waiting}, sandboxWithPod(), "/api/v1/sandboxes/default/sb-a/logs", http.StatusConflict},
		{"kubelet refused", &fakeLogs{err: gone}, sandboxWithPod(), "/api/v1/sandboxes/default/sb-a/logs", http.StatusBadGateway},
		{"other failure", &fakeLogs{err: errors.New("boom")}, sandboxWithPod(), "/api/v1/sandboxes/default/sb-a/logs", http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := getLogs(logsRouter(t, tc.logs, tc.objs...), tc.path)
			require.Equal(t, tc.want, rec.Code)
			require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
		})
	}
}

func TestClampLines(t *testing.T) {
	tests := map[string]int64{"": DefaultLogLines, "abc": DefaultLogLines, "0": DefaultLogLines, "-3": DefaultLogLines, "200": 200, "99999": MaxLogLines}
	for raw, want := range tests {
		require.Equal(t, want, clampLines(raw), "clampLines(%q)", raw)
	}
}
