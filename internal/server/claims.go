package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

// ClaimDetail surfaces the spec, conditions, the referenced template name,
// and the embedded bound-Sandbox status from SandboxClaimStatus.
type ClaimDetail struct {
	Summary       ResourceSummary               `json:"summary"`
	Spec          *extv1alpha1.SandboxClaimSpec `json:"spec"`
	Conditions    []metav1.Condition            `json:"conditions"`
	TemplateRef   string                        `json:"templateRef"`
	SandboxStatus extv1alpha1.SandboxStatus     `json:"sandboxStatus"`
	Events        []EventEntry                  `json:"events"`
}

func handleClaimList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nsFilter := r.URL.Query().Get("namespace")
		phaseFilter := r.URL.Query().Get("phase")

		var list extv1alpha1.SandboxClaimList
		if err := d.Client.List(r.Context(), &list); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-claims",
				Detail:    "could not list claims",
				LogReason: err.Error(),
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
			phase := readyPhase(it.Status.Conditions)
			if phaseFilter != "" && phase != phaseFilter {
				continue
			}
			out = append(out, ResourceSummary{
				Name:       it.Name,
				Namespace:  it.Namespace,
				Kind:       "SandboxClaim",
				Phase:      phase,
				AgeSeconds: ageSeconds(it.ObjectMeta, now),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

func handleClaimDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		var c extv1alpha1.SandboxClaim
		if err := d.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &c); err != nil {
			if apierrors.IsNotFound(err) {
				writeProblem(w, d.Logger, problemArgs{
					Status: http.StatusNotFound,
					Type:   "claim-not-found",
					Detail: "claim not found",
				})
				return
			}
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "get-claim",
				Detail:    "could not load claim",
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

		writeJSON(w, http.StatusOK, ClaimDetail{
			Summary: ResourceSummary{
				Name:       c.Name,
				Namespace:  c.Namespace,
				Kind:       "SandboxClaim",
				Phase:      readyPhase(c.Status.Conditions),
				AgeSeconds: ageSeconds(c.ObjectMeta, time.Now()),
			},
			Spec:          &c.Spec,
			Conditions:    c.Status.Conditions,
			TemplateRef:   c.Spec.TemplateRef.Name,
			SandboxStatus: c.Status.SandboxStatus,
			Events:        events,
		})
	}
}
