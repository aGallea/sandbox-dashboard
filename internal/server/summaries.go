package server

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
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
	CreatorOpenSandbox  = "opensandbox"
	CreatorAgentSandbox = "agent-sandbox"
	CreatorUnknown      = "unknown"
)

// Creator is a creator this install names: a sandbox whose labels match
// Selector is reported as Name. Only Name and Icon reach the browser.
type Creator struct {
	Name     string          `json:"name"`
	Selector labels.Selector `json:"-"`
	Icon     string          `json:"icon,omitempty"`
}

// creatorFor infers which system created a sandbox, preferring the client
// that asked for it over the system it went through: the install's own
// creators first, in order, then OpenSandbox, then the agent-sandbox
// controller OpenSandbox may run on.
func creatorFor(meta *metav1.ObjectMeta, custom []Creator) string {
	for _, c := range custom {
		if c.Selector.Matches(labels.Set(meta.Labels)) {
			return c.Name
		}
	}
	if meta.Labels[OsbIDLabel] != "" {
		return CreatorOpenSandbox
	}
	// The agent-sandbox controller makes a Sandbox only for a SandboxClaim or a
	// SandboxWarmPool, and sets that object as the Sandbox's controller owner.
	if owner := metav1.GetControllerOfNoCopy(meta); owner != nil {
		if gv, err := schema.ParseGroupVersion(owner.APIVersion); err == nil && gv.Group == extv1alpha1.GroupVersion.Group {
			return CreatorAgentSandbox
		}
	}
	return CreatorUnknown
}
