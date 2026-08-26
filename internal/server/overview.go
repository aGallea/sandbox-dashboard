package server

import (
	"encoding/json"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

// OverviewResponse is the JSON returned by GET /api/v1/overview.
type OverviewResponse struct {
	Sandboxes ResourceCounts `json:"sandboxes"`
	Claims    ResourceCounts `json:"claims"`
	Templates TemplateCounts `json:"templates"`
	WarmPools WarmPoolCounts `json:"warmPools"`
	// Scope is absent when the install watches every namespace, which is what
	// lets the UI tell "the whole cluster" from "these namespaces".
	Scope *Scope `json:"scope,omitempty"`
}

// Scope is the part of the cluster this install can see. A narrowed install
// counts a partial fleet, and without this the count reads as the whole cluster.
type Scope struct {
	Namespaces []string `json:"namespaces"`
}

// ResourceCounts is the count rollup for Sandbox and SandboxClaim — both expose Ready conditions.
type ResourceCounts struct {
	Total    int `json:"total"`
	Ready    int `json:"ready"`
	NotReady int `json:"notReady"`
	Unknown  int `json:"unknown"`
}

// TemplateCounts is the rollup for SandboxTemplate (no status subresource).
type TemplateCounts struct {
	Total int `json:"total"`
}

// WarmPoolCounts is the rollup for SandboxWarmPool — replica-shaped.
type WarmPoolCounts struct {
	Total         int   `json:"total"`
	Replicas      int32 `json:"replicas"`
	ReadyReplicas int32 `json:"readyReplicas"`
}

func handleOverview(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		resp := OverviewResponse{}
		if len(d.WatchNamespaces) > 0 {
			resp.Scope = &Scope{Namespaces: d.WatchNamespaces}
		}

		var sbs v1alpha1.SandboxList
		if err := d.Client.List(ctx, &sbs); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-sandboxes",
				Detail:    "could not list sandboxes",
				LogReason: err.Error(),
			})
			return
		}
		resp.Sandboxes = countByReady(sbs.Items, func(i int) []metav1.Condition {
			return sbs.Items[i].Status.Conditions
		})

		var claims extv1alpha1.SandboxClaimList
		if err := d.Client.List(ctx, &claims); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-claims",
				Detail:    "could not list claims",
				LogReason: err.Error(),
			})
			return
		}
		resp.Claims = countByReady(claims.Items, func(i int) []metav1.Condition {
			return claims.Items[i].Status.Conditions
		})

		var tmpls extv1alpha1.SandboxTemplateList
		if err := d.Client.List(ctx, &tmpls); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-templates",
				Detail:    "could not list templates",
				LogReason: err.Error(),
			})
			return
		}
		resp.Templates = TemplateCounts{Total: len(tmpls.Items)}

		var pools extv1alpha1.SandboxWarmPoolList
		if err := d.Client.List(ctx, &pools); err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusInternalServerError,
				Type:      "list-warmpools",
				Detail:    "could not list warm pools",
				LogReason: err.Error(),
			})
			return
		}
		resp.WarmPools.Total = len(pools.Items)
		for i := range pools.Items {
			resp.WarmPools.Replicas += pools.Items[i].Status.Replicas
			resp.WarmPools.ReadyReplicas += pools.Items[i].Status.ReadyReplicas
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func countByReady[T any](items []T, conds func(i int) []metav1.Condition) ResourceCounts {
	out := ResourceCounts{Total: len(items)}
	for i := range items {
		switch readyPhase(conds(i)) {
		case "Ready":
			out.Ready++
		case "NotReady":
			out.NotReady++
		default:
			out.Unknown++
		}
	}
	return out
}
