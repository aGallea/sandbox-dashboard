package server

import "time"

// OsbView is OpenSandbox's own view of a sandbox, plus the two signals the
// dashboard derives from it. It is only ever populated for sandboxes that
// carry the OsbIDLabel and were matched against the OpenSandbox inventory.
type OsbView struct {
	State            string     `json:"state"`
	Reason           string     `json:"reason,omitempty"`
	Message          string     `json:"message,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	LastTransitionAt *time.Time `json:"lastTransitionAt,omitempty"`
	StateAgeSeconds  int64      `json:"stateAgeSeconds"`
	Diverged         bool       `json:"diverged"`
	Stale            bool       `json:"stale"`
}

// osbAgreement maps each OpenSandbox state to the CR phases it is consistent
// with. OpenSandbox's eight-state lifecycle collapses into the Ready
// condition's three values, so this table is where that judgment lives.
var osbAgreement = map[string][]string{
	"Running":    {"Ready"},
	"Pending":    {"NotReady", "Unknown"},
	"Pausing":    {"NotReady", "Unknown"},
	"Paused":     {"NotReady", "Unknown"},
	"Resuming":   {"NotReady", "Unknown"},
	"Stopping":   {"NotReady", "Unknown"},
	"Terminated": {"NotReady", "Unknown"},
	"Failed":     {"NotReady", "Unknown"},
}

// agrees reports whether an OpenSandbox state is consistent with a CR phase.
// An unrecognised state agrees with everything: a state this build has never
// heard of is "no opinion", not a disagreement, so adding a state upstream
// cannot light up the whole fleet.
func agrees(osbState, phase string) bool {
	allowed, known := osbAgreement[osbState]
	if !known {
		return true
	}
	for _, p := range allowed {
		if p == phase {
			return true
		}
	}
	return false
}
