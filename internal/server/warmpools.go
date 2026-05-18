package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

// WarmPoolDetail is the JSON returned by GET /api/v1/warmpools/{namespace}/{name}.
type WarmPoolDetail struct {
	Summary       ResourceSummary                  `json:"summary"`
	Spec          *extv1alpha1.SandboxWarmPoolSpec `json:"spec"`
	Replicas      int32                            `json:"replicas"`
	ReadyReplicas int32                            `json:"readyReplicas"`
	Selector      string                           `json:"selector"`
}

// warmPoolPhase maps replica counts to a phase string the SPA can render.
func warmPoolPhase(replicas, readyReplicas int32) string {
	switch {
	case replicas == 0 && readyReplicas == 0:
		return "Unknown"
	case readyReplicas == replicas:
		return "Ready"
	default:
		return "Scaling"
	}
}

func handleWarmPoolList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nsFilter := r.URL.Query().Get("namespace")
		phaseFilter := r.URL.Query().Get("phase")

		var list extv1alpha1.SandboxWarmPoolList
		if err := d.Client.List(r.Context(), &list); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusInternalServerError, Type: "list-warmpools",
				Detail: "could not list warmpools", LogReason: err.Error(),
			})
			return
		}
		now := time.Now()
		out := make([]ResourceSummary, 0, len(list.Items))
		for i := range list.Items {
			it := &list.Items[i]
			if nsFilter != "" && it.Namespace != nsFilter {
				continue
			}
			phase := warmPoolPhase(it.Status.Replicas, it.Status.ReadyReplicas)
			if phaseFilter != "" && phase != phaseFilter {
				continue
			}
			out = append(out, ResourceSummary{
				Name: it.Name, Namespace: it.Namespace, Kind: "SandboxWarmPool",
				Phase: phase, AgeSeconds: ageSeconds(it.ObjectMeta, now),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

func handleWarmPoolDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		var p extv1alpha1.SandboxWarmPool
		if err := d.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &p); err != nil {
			if apierrors.IsNotFound(err) {
				writeProblem(w, d.Logger, problemArgs{
					Status: http.StatusNotFound, Type: "warmpool-not-found", Detail: "warmpool not found",
				})
				return
			}
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusInternalServerError, Type: "get-warmpool",
				Detail: "could not load warmpool", LogReason: err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, WarmPoolDetail{
			Summary: ResourceSummary{
				Name: p.Name, Namespace: p.Namespace, Kind: "SandboxWarmPool",
				Phase:      warmPoolPhase(p.Status.Replicas, p.Status.ReadyReplicas),
				AgeSeconds: ageSeconds(p.ObjectMeta, time.Now()),
			},
			Spec:          &p.Spec,
			Replicas:      p.Status.Replicas,
			ReadyReplicas: p.Status.ReadyReplicas,
			Selector:      p.Status.Selector,
		})
	}
}
