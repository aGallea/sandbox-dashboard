package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
)

// Log line limits. The tail default matches what `kubectl logs --tail` users
// reach for first; the cap keeps one drawer from pulling a whole day of output
// through the dashboard.
const (
	DefaultLogLines int64 = 50
	MaxLogLines     int64 = 1000
)

// PodLogStreamer opens a pod's log stream. The controller-runtime cache cannot
// serve logs — they are a subresource, not an object — so this is the one place
// the dashboard talks to the API server directly. Nil disables the endpoint.
type PodLogStreamer interface {
	PodLogs(ctx context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error)
}

// ClientsetLogs adapts a client-go clientset to PodLogStreamer.
type ClientsetLogs struct {
	kubernetes.Interface
}

func (c ClientsetLogs) PodLogs(ctx context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	return c.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
}

// handleSandboxLogs serves GET /api/v1/sandboxes/{namespace}/{name}/logs as
// plain text: the raw lines the container wrote, for the browser to parse and
// colour. Parsing here would commit the server to one log format, and a fleet
// runs many.
//
//	?tail=N       last N lines (default 50, max 1000)
//	?head=N       first N lines instead
//	?container=x  which container; defaults to the pod's first
func handleSandboxLogs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Logs == nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusServiceUnavailable,
				Type:   "logs-unconfigured",
				Detail: "this dashboard install cannot read pod logs",
			})
			return
		}
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		sb, ok := getSandboxOrProblem(w, r, d, ns, name)
		if !ok {
			return
		}
		pods, err := podsBySandboxUID(r.Context(), d.Client, ns)
		if err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-pods",
				Detail:    "could not find the sandbox's pod",
				LogReason: err.Error(),
			})
			return
		}
		pod, ok := pods[sb.UID]
		if !ok {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusNotFound,
				Type:   "sandbox-has-no-pod",
				Detail: "the sandbox has no pod yet, or it is already gone",
			})
			return
		}

		q := r.URL.Query()
		opts := &corev1.PodLogOptions{Container: q.Get("container")}
		if opts.Container == "" && len(pod.Spec.Containers) > 0 {
			opts.Container = pod.Spec.Containers[0].Name
		}
		// The API only tails. "First N lines" is read here by taking N lines off
		// the full stream and closing it, which stops the kubelet's transfer.
		head := q.Has("head")
		lines := clampLines(q.Get("head"))
		if !head {
			lines = clampLines(q.Get("tail"))
			opts.TailLines = &lines
		}

		stream, err := d.Logs.PodLogs(r.Context(), ns, pod.Name, opts)
		if err != nil {
			// The kubelet answers 400 for a container that has not started —
			// nothing to read yet, which is a state of the pod, not a fault.
			if apierrors.IsBadRequest(err) {
				writeProblem(w, d.Logger, problemArgs{
					Status: http.StatusConflict,
					Type:   "container-not-started",
					Detail: "the container has not started yet, so it has no logs",
				})
				return
			}
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusBadGateway,
				Type:      "pod-logs-failed",
				Detail:    "could not read the pod's logs",
				LogReason: err.Error(),
			})
			return
		}
		defer func() { _ = stream.Close() }()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if head {
			_ = copyLines(w, stream, lines)
			return
		}
		_, _ = io.Copy(w, stream)
	}
}

// clampLines parses a line count, falling back to the default when absent or
// unparsable and capping what it accepts.
func clampLines(raw string) int64 {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return DefaultLogLines
	}
	return min(n, MaxLogLines)
}

// copyLines writes at most n newline-terminated lines from r to w.
func copyLines(w io.Writer, r io.Reader, n int64) error {
	br := bufio.NewReader(r)
	for ; n > 0; n-- {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return werr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	return nil
}
