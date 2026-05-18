package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
)

// SandboxDetail is the JSON returned by GET /api/v1/sandboxes/{namespace}/{name}.
type SandboxDetail struct {
	Summary     ResourceSummary       `json:"summary"`
	Spec        *v1alpha1.SandboxSpec `json:"spec"`
	Conditions  []metav1.Condition    `json:"conditions"`
	Replicas    int32                 `json:"replicas"`
	PodIPs      []string              `json:"podIPs"`
	ServiceFQDN string                `json:"serviceFqdn"`
	Events      []EventEntry          `json:"events"`
}

func handleSandboxList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nsFilter := r.URL.Query().Get("namespace")
		phaseFilter := r.URL.Query().Get("phase")

		var list v1alpha1.SandboxList
		if err := d.Client.List(r.Context(), &list); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-sandboxes",
				Detail:    "could not list sandboxes",
				LogReason: err.Error(),
			})
			return
		}
		now := time.Now()
		summaries := make([]ResourceSummary, 0, len(list.Items))
		for i := range list.Items {
			item := &list.Items[i]
			if nsFilter != "" && item.Namespace != nsFilter {
				continue
			}
			phase := readyPhase(item.Status.Conditions)
			if phaseFilter != "" && phase != phaseFilter {
				continue
			}
			summaries = append(summaries, ResourceSummary{
				Name:       item.Name,
				Namespace:  item.Namespace,
				Kind:       "Sandbox",
				Phase:      phase,
				AgeSeconds: ageSeconds(item.ObjectMeta, now),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": summaries})
	}
}

func handleSandboxDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		var sb v1alpha1.Sandbox
		if err := d.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &sb); err != nil {
			if apierrors.IsNotFound(err) {
				writeProblem(w, d.Logger, problemArgs{
					Status: http.StatusNotFound,
					Type:   "sandbox-not-found",
					Detail: "sandbox not found",
				})
				return
			}
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "get-sandbox",
				Detail:    "could not load sandbox",
				LogReason: err.Error(),
			})
			return
		}

		events, err := eventsFor(r.Context(), d.Client, ns, name, 50)
		if err != nil {
			if d.Logger != nil {
				d.Logger.Error("events_fetch_failed", "ns", ns, "name", name, "err", err.Error())
			}
			events = []EventEntry{}
		}
		if events == nil {
			events = []EventEntry{}
		}

		writeJSON(w, http.StatusOK, SandboxDetail{
			Summary: ResourceSummary{
				Name:       sb.Name,
				Namespace:  sb.Namespace,
				Kind:       "Sandbox",
				Phase:      readyPhase(sb.Status.Conditions),
				AgeSeconds: ageSeconds(sb.ObjectMeta, time.Now()),
			},
			Spec:        &sb.Spec,
			Conditions:  sb.Status.Conditions,
			Replicas:    sb.Status.Replicas,
			PodIPs:      sb.Status.PodIPs,
			ServiceFQDN: sb.Status.ServiceFQDN,
			Events:      events,
		})
	}
}

// writeJSON writes v as JSON with Content-Type set.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
