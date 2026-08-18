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
	Creator    string   `json:"creator,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	Team       string   `json:"team,omitempty"`
	Experiment string   `json:"experiment,omitempty"`
	SessionID  string   `json:"sessionId,omitempty"`
	Osb        *OsbView `json:"osb,omitempty"`
	Pod        *PodView `json:"pod,omitempty"`

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

// identityFor pulls the human-meaningful labels. These are read from the CR
// rather than the OpenSandbox API, which carries identical values, so that an
// OpenSandbox outage costs one column instead of the whole table.
//
// sessionID is returned separately because it is the most reliably present of
// the four: measured on the live cluster, 166 of 166 sandboxes carried
// session_id while none carried owner/team/experiment, which the eval harness
// stamps only for some workloads.
//
// Owner is accepted under two keys because the fleet has no single convention:
// the domain-qualified ai21.com/owner and the bare owner. Neither appeared on
// the 252 sandboxes measured in algo-studio, so this only pays off on fleets
// whose harness stamps one of them.
func identityFor(labels map[string]string) (owner, team, experiment, sessionID string) {
	return firstLabel(labels, "ai21.com/owner", "owner"),
		labels["team"], labels["experiment"], labels["session_id"]
}

// firstLabel returns the first key that is present and non-empty.
func firstLabel(labels map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := labels[k]; v != "" {
			return v
		}
	}
	return ""
}
