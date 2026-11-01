package server

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceSummary is the per-row DTO returned by every list endpoint.
type ResourceSummary struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Phase      string `json:"phase"`      // Ready | NotReady | Unknown | "" for kinds with no Ready cond
	AgeSeconds int64  `json:"ageSeconds"` // seconds since creation

	// The fields below are only populated for sandboxes; other kinds omit them.
	Creator   string   `json:"creator,omitempty"`
	SessionID string   `json:"sessionId,omitempty"`
	Osb       *OsbView `json:"osb,omitempty"`
	Pod       *PodView `json:"pod,omitempty"`

	// Labels is the sandbox's labels verbatim. The overview page derives its
	// grouping dimensions from whatever keys the fleet actually carries, so no
	// curated subset would do: the useful key on one cluster is `team`, on
	// another `policy.ai21.com/preemptible`, and picking for them is guesswork.
	Labels map[string]string `json:"labels,omitempty"`
}

// readyPhase maps the Ready condition's Status to the dashboard's phase string.
func readyPhase(conds []metav1.Condition) string {
	for i := range conds {
		if conds[i].Type == "Ready" {
			switch conds[i].Status {
			case metav1.ConditionTrue:
				return "Ready"
			case metav1.ConditionFalse:
				return "NotReady"
			default:
				return "Unknown"
			}
		}
	}
	return "Unknown"
}

// ageSeconds returns the number of seconds between meta.CreationTimestamp and now.
// Returns 0 if creation timestamp is zero or negative.
func ageSeconds(meta metav1.ObjectMeta, now time.Time) int64 {
	if meta.CreationTimestamp.IsZero() {
		return 0
	}
	d := now.Sub(meta.CreationTimestamp.Time)
	if d < 0 {
		return 0
	}
	return int64(d.Seconds())
}

// OsbIDLabel is the label OpenSandbox stamps on every Sandbox CR it creates.
// It is the join key between the CR and the OpenSandbox API: measured against
// algo-studio, the label matched 102/102 records while the CR name matched
// only 29/92, because OpenSandbox writes both "<uuid>" and "sandbox-<uuid>".
const OsbIDLabel = "opensandbox.io/id"

// SessionIDLabel names the task a sandbox runs, on fleets whose client stamps
// one. The list labels its rows by it.
const SessionIDLabel = "session_id"

// Creator values reported in ResourceSummary.Creator.
const (
	CreatorOpenSandbox = "opensandbox"
	CreatorUnknown     = "unknown"
)

// creatorFor infers which system created a sandbox. No Sandbox CR in the wild
// carries ownerReferences, so labels are the only available signal.
func creatorFor(labels map[string]string) string {
	if labels[OsbIDLabel] != "" {
		return CreatorOpenSandbox
	}
	return CreatorUnknown
}
