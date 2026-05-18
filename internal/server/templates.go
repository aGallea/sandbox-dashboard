package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

// TemplateDetail is the JSON returned by GET /api/v1/templates/{namespace}/{name}.
type TemplateDetail struct {
	Summary ResourceSummary                  `json:"summary"`
	Spec    *extv1alpha1.SandboxTemplateSpec `json:"spec"`
}

func handleTemplateList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nsFilter := r.URL.Query().Get("namespace")

		var list extv1alpha1.SandboxTemplateList
		if err := d.Client.List(r.Context(), &list); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusInternalServerError, Type: "list-templates",
				Detail: "could not list templates", LogReason: err.Error(),
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
			out = append(out, ResourceSummary{
				Name: it.Name, Namespace: it.Namespace, Kind: "SandboxTemplate",
				Phase: "", AgeSeconds: ageSeconds(it.ObjectMeta, now),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

func handleTemplateDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		var t extv1alpha1.SandboxTemplate
		if err := d.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &t); err != nil {
			if apierrors.IsNotFound(err) {
				writeProblem(w, d.Logger, problemArgs{
					Status: http.StatusNotFound, Type: "template-not-found", Detail: "template not found",
				})
				return
			}
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusInternalServerError, Type: "get-template",
				Detail: "could not load template", LogReason: err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, TemplateDetail{
			Summary: ResourceSummary{
				Name: t.Name, Namespace: t.Namespace, Kind: "SandboxTemplate",
				Phase: "", AgeSeconds: ageSeconds(t.ObjectMeta, time.Now()),
			},
			Spec: &t.Spec,
		})
	}
}
