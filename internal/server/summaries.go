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
