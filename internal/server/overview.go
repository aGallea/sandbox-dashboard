package server

import (
	"encoding/json"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

// OverviewResponse is the JSON returned by GET /api/v1/overview.
type OverviewResponse struct {
	Sandboxes ResourceCounts `json:"sandboxes"`
	Claims    ResourceCounts `json:"claims"`
	Templates TemplateCounts `json:"templates"`
	WarmPools WarmPoolCounts `json:"warmPools"`
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

func handleOverview(reader client.Reader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		resp := OverviewResponse{}

		var sbs v1alpha1.SandboxList
		if err := reader.List(ctx, &sbs); err != nil {
			writeProblem(w, http.StatusInternalServerError, "list-sandboxes", err.Error())
			return
		}
		resp.Sandboxes = countByReady(sbs.Items, func(i int) []metav1.Condition {
			return sbs.Items[i].Status.Conditions
		})

		var claims extv1alpha1.SandboxClaimList
		if err := reader.List(ctx, &claims); err != nil {
			writeProblem(w, http.StatusInternalServerError, "list-claims", err.Error())
			return
		}
		resp.Claims = countByReady(claims.Items, func(i int) []metav1.Condition {
			return claims.Items[i].Status.Conditions
		})

		var tmpls extv1alpha1.SandboxTemplateList
		if err := reader.List(ctx, &tmpls); err != nil {
			writeProblem(w, http.StatusInternalServerError, "list-templates", err.Error())
			return
		}
		resp.Templates = TemplateCounts{Total: len(tmpls.Items)}

		var pools extv1alpha1.SandboxWarmPoolList
		if err := reader.List(ctx, &pools); err != nil {
			writeProblem(w, http.StatusInternalServerError, "list-warmpools", err.Error())
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
		switch readyState(conds(i)) {
		case metav1.ConditionTrue:
			out.Ready++
		case metav1.ConditionFalse:
			out.NotReady++
		default:
			out.Unknown++
		}
	}
	return out
}

func readyState(conds []metav1.Condition) metav1.ConditionStatus {
	for i := range conds {
		if conds[i].Type == "Ready" {
			return conds[i].Status
		}
	}
	return metav1.ConditionUnknown
}

// writeProblem emits an RFC 7807 problem+json response.
func writeProblem(w http.ResponseWriter, status int, typ, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "/errors/" + typ,
		"status": status,
		"detail": detail,
	})
}
