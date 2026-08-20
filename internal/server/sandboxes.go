package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aGallea/sandbox-dashboard/internal/osb"
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

// sandboxListResponse is the JSON returned by GET /api/v1/sandboxes.
type sandboxListResponse struct {
	Items []ResourceSummary `json:"items"`
	Osb   *OsbStatus        `json:"osb,omitempty"`
}

func handleSandboxList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nsFilter := r.URL.Query().Get("namespace")
		phaseFilter := r.URL.Query().Get("phase")
		creatorFilter := r.URL.Query().Get("creator")
		osbStateFilter := r.URL.Query().Get("osbState")
		staleOnly := r.URL.Query().Get("stale") == "true"

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

		now := d.now()
		staleAfter := d.staleAfter()

		// Fetch the OpenSandbox inventory once for the whole page. A failure
		// here is never fatal: the CR data is still worth serving.
		var (
			inventory map[string]osb.Sandbox
			osbStatus *OsbStatus
		)
		if d.Osb != nil {
			ctx, cancel := context.WithTimeout(r.Context(), d.osbTimeout())
			fetched, err := d.Osb.ListSandboxes(ctx)
			cancel()
			if err != nil {
				if d.Logger != nil {
					d.Logger.Error("osb_list_failed", "err", err.Error())
				}
				osbStatus = &OsbStatus{Status: "unreachable", Error: "OpenSandbox is unreachable"}
			} else {
				inventory = fetched
				at := now
				osbStatus = &OsbStatus{Status: "ok", FetchedAt: &at, Reported: len(fetched)}
			}
		}

		// Pods carry what the CR cannot report: scheduling phase, restarts, node
		// and the reservation actually held. A failure here is never fatal —
		// same rule as the OpenSandbox join above: degrade the row, serve the
		// rest. Scoped to nsFilter when set, since rows outside it are dropped.
		pods, err := podsBySandboxUID(r.Context(), d.Client, nsFilter)
		if err != nil && d.Logger != nil {
			d.Logger.Error("pod_list_failed", "err", err.Error())
		}

		summaries := make([]ResourceSummary, 0, len(list.Items))
		matched := 0
		unrecognisedStates := map[string]int{}
		for i := range list.Items {
			item := &list.Items[i]
			phase := readyPhase(item.Status.Conditions)
			creator := creatorFor(item.Labels)
			owner, team, experiment, sessionID := identityFor(item.Labels)

			// Join before any display filter: `matched` must count join success
			// across the whole fleet, not "joined and survived the filters".
			// Counting it after the filters made ?namespace= fire a false
			// osb_join_incomplete warning.
			var view *OsbView
			if id := item.Labels[OsbIDLabel]; id != "" {
				if s, ok := inventory[id]; ok {
					matched++
					v := newOsbView(s, phase, now, staleAfter)
					view = &v
					if !osbStateRecognised(s.Status.State) {
						unrecognisedStates[s.Status.State]++
					}
				}
			}

			if nsFilter != "" && item.Namespace != nsFilter {
				continue
			}
			if phaseFilter != "" && phase != phaseFilter {
				continue
			}
			if creatorFilter != "" && creator != creatorFilter {
				continue
			}
			// osbState and stale filter on `view`, which is nil whenever the
			// OpenSandbox inventory is unavailable (unconfigured or unreachable).
			// In that case both filters yield an EMPTY items list — not "nothing
			// matches", but "the join could not be computed". A client must check
			// osb.status == "ok" before presenting an empty result under these
			// filters as "nothing is stale" / "nothing in that state".
			if osbStateFilter != "" && (view == nil || view.State != osbStateFilter) {
				continue
			}
			if staleOnly && (view == nil || !view.Stale) {
				continue
			}

			summaries = append(summaries, ResourceSummary{
				Name:       item.Name,
				Namespace:  item.Namespace,
				Kind:       "Sandbox",
				Phase:      phase,
				AgeSeconds: ageSeconds(item.ObjectMeta, now),
				Creator:    creator,
				Owner:      owner,
				Team:       team,
				Experiment: experiment,
				SessionID:  sessionID,
				Osb:        view,
				Pod:        podViewFor(pods, item.UID),
				Labels:     item.Labels,
			})
		}

		// Reported-versus-matched makes a broken join key visible instead of
		// silently degrading every row to creator "unknown".
		if osbStatus != nil && osbStatus.Status == "ok" {
			osbStatus.Matched = matched
			if d.Logger != nil && osbStatus.Reported != matched {
				d.Logger.Warn("osb_join_incomplete", "reported", osbStatus.Reported, "matched", matched)
			}
		}

		// One log line per distinct unrecognised state per request, not once per
		// row: an upstream rollout that ships a new state to a whole fleet must
		// not spam the log per sandbox.
		if d.Logger != nil {
			for state, count := range unrecognisedStates {
				d.Logger.Warn("osb_unrecognised_state", "state", state, "count", count)
			}
		}

		writeJSON(w, http.StatusOK, sandboxListResponse{Items: summaries, Osb: osbStatus})
	}
}

func handleSandboxDetail(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		sb, ok := getSandboxOrProblem(w, r, d, ns, name)
		if !ok {
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

		now := d.now()
		phase := readyPhase(sb.Status.Conditions)
		creator := creatorFor(sb.Labels)
		owner, team, experiment, sessionID := identityFor(sb.Labels)

		pods, err := podsBySandboxUID(r.Context(), d.Client, ns)
		if err != nil && d.Logger != nil {
			d.Logger.Error("pod_list_failed", "ns", ns, "name", name, "err", err.Error())
		}

		// An OpenSandbox failure here must not fail the detail response: the CR
		// data is still worth serving. Mirrors handleSandboxList's fallback.
		var view *OsbView
		if d.Osb != nil {
			if id := sb.Labels[OsbIDLabel]; id != "" {
				ctx, cancel := context.WithTimeout(r.Context(), d.osbTimeout())
				inventory, err := d.Osb.ListSandboxes(ctx)
				cancel()
				if err != nil {
					if d.Logger != nil {
						d.Logger.Error("osb_list_failed", "err", err.Error())
					}
				} else if s, found := inventory[id]; found {
					v := newOsbView(s, phase, now, d.staleAfter())
					view = &v
					if !osbStateRecognised(s.Status.State) && d.Logger != nil {
						d.Logger.Warn("osb_unrecognised_state", "state", s.Status.State, "count", 1)
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, SandboxDetail{
			Summary: ResourceSummary{
				Name:       sb.Name,
				Namespace:  sb.Namespace,
				Kind:       "Sandbox",
				Phase:      phase,
				AgeSeconds: ageSeconds(sb.ObjectMeta, now),
				Creator:    creator,
				Owner:      owner,
				Team:       team,
				Experiment: experiment,
				SessionID:  sessionID,
				Osb:        view,
				Pod:        podViewFor(pods, sb.UID),
				Labels:     sb.Labels,
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

// getSandboxOrProblem fetches the named Sandbox CR, writing the appropriate
// problem+json response (and returning ok=false) when it cannot be found or
// read. Shared by handleSandboxDetail and handleSandboxOsb, which otherwise
// duplicated this lookup verbatim.
func getSandboxOrProblem(w http.ResponseWriter, r *http.Request, d Deps, ns, name string) (v1alpha1.Sandbox, bool) {
	var sb v1alpha1.Sandbox
	if err := d.Client.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &sb); err != nil {
		if apierrors.IsNotFound(err) {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusNotFound,
				Type:   "sandbox-not-found",
				Detail: "sandbox not found",
			})
			return v1alpha1.Sandbox{}, false
		}
		writeProblem(w, d.Logger, problemArgs{
			Status:    http.StatusInternalServerError,
			Type:      "get-sandbox",
			Detail:    "could not load sandbox",
			LogReason: err.Error(),
		})
		return v1alpha1.Sandbox{}, false
	}
	return sb, true
}

// SandboxOsbDetail is the JSON returned by GET /api/v1/sandboxes/{ns}/{name}/osb.
type SandboxOsbDetail struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Events  string `json:"events"`
}

func handleSandboxOsb(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Osb == nil {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusServiceUnavailable,
				Type:   "opensandbox-unconfigured",
				Detail: "OpenSandbox URL not configured on this dashboard install",
			})
			return
		}
		ns := chi.URLParam(r, "namespace")
		name := chi.URLParam(r, "name")

		sb, ok := getSandboxOrProblem(w, r, d, ns, name)
		if !ok {
			return
		}

		id := sb.Labels[OsbIDLabel]
		if id == "" {
			writeProblem(w, d.Logger, problemArgs{
				Status: http.StatusNotFound,
				Type:   "not-an-opensandbox-sandbox",
				Detail: "this sandbox was not created by OpenSandbox",
			})
			return
		}

		diag, err := d.Osb.Diagnostics(r.Context(), id)
		if err != nil {
			writeProblem(w, d.Logger, problemArgs{
				Status:    http.StatusBadGateway,
				Type:      "opensandbox-unreachable",
				Detail:    "could not load OpenSandbox diagnostics",
				LogReason: err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, SandboxOsbDetail{ID: id, Summary: diag.Summary, Events: diag.Events})
	}
}

// writeJSON writes v as JSON with Content-Type set.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
