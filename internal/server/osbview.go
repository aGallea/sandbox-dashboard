package server

import (
	"time"

	"github.com/aGallea/sandbox-dashboard/internal/osb"
)

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

// osbStateRecognised reports whether state is a key of osbAgreement. Callers
// use this to log an unrecognised upstream state once per request, rather
// than reaching into the map directly.
func osbStateRecognised(state string) bool {
	_, ok := osbAgreement[state]
	return ok
}

// DefaultOsbStaleAfter is how long a non-terminal OpenSandbox state may sit
// unchanged before it is reported stale. Sixty seconds comes from measurement,
// not taste: pods in algo-studio reached Ready about two seconds after
// creation, so a minute is already far outside normal.
const DefaultOsbStaleAfter = 60 * time.Second

// DefaultOsbTimeout bounds a single OpenSandbox inventory fetch. 2s is
// generous: a live fetch of 166 records takes tens of milliseconds. The point
// is to fail fast enough that the HTTP server's WriteTimeout never races it,
// leaving the informer-cached Kubernetes data servable even when OpenSandbox
// is wedged.
const DefaultOsbTimeout = 2 * time.Second

// osbTransientStates are the states a sandbox should pass through in seconds.
// Age against these is meaningful. Running, Terminated and Failed are resting
// places, and Paused is a state a caller deliberately holds, so none of them
// can be stale no matter how old.
var osbTransientStates = map[string]bool{
	"Pending":  true,
	"Pausing":  true,
	"Resuming": true,
	"Stopping": true,
}

// isStale reports whether a transient OpenSandbox state has sat unchanged past
// the threshold. This is what distinguishes a dead watch from an ordinary
// in-flight transition: during the 2026-08-17 incident the stuck sandboxes had
// lastTransitionAt equal to createdAt, because no second event ever arrived.
func isStale(state string, lastTransitionAt *time.Time, now time.Time, threshold time.Duration) bool {
	if !osbTransientStates[state] || lastTransitionAt == nil {
		return false
	}
	return now.Sub(*lastTransitionAt) > threshold
}

// newOsbView builds the OpenSandbox column values for one sandbox, given the
// CR phase it is being compared against.
func newOsbView(s osb.Sandbox, phase string, now time.Time, staleAfter time.Duration) OsbView {
	v := OsbView{
		State:            s.Status.State,
		Reason:           s.Status.Reason,
		Message:          s.Status.Message,
		ExpiresAt:        s.ExpiresAt,
		LastTransitionAt: s.Status.LastTransitionAt,
		Diverged:         !agrees(s.Status.State, phase),
		Stale:            isStale(s.Status.State, s.Status.LastTransitionAt, now, staleAfter),
	}
	if s.Status.LastTransitionAt != nil {
		if age := now.Sub(*s.Status.LastTransitionAt); age > 0 {
			v.StateAgeSeconds = int64(age.Seconds())
		}
	}
	return v
}

// OsbStatus reports the health of the OpenSandbox join for one list response.
// It is omitted entirely when no OpenSandbox URL is configured.
type OsbStatus struct {
	Status    string     `json:"status"` // "ok" | "unreachable"
	Error     string     `json:"error,omitempty"`
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
	Reported  int        `json:"reported"` // sandboxes OpenSandbox returned
	Matched   int        `json:"matched"`  // of those, how many joined to a CR
}
