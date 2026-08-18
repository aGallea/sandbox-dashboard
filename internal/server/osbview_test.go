package server

import (
	"testing"
	"time"

	"github.com/aGallea/sandbox-dashboard/internal/osb"
	"github.com/stretchr/testify/require"
)

func TestAgrees_RunningMatchesReadyOnly(t *testing.T) {
	require.True(t, agrees("Running", "Ready"))
	require.False(t, agrees("Running", "NotReady"))
	require.False(t, agrees("Running", "Unknown"))
}

func TestAgrees_NonRunningStatesMatchNotReadyAndUnknown(t *testing.T) {
	for _, state := range []string{"Pending", "Pausing", "Paused", "Resuming", "Stopping", "Terminated", "Failed"} {
		t.Run(state, func(t *testing.T) {
			require.True(t, agrees(state, "NotReady"))
			require.True(t, agrees(state, "Unknown"))
			require.False(t, agrees(state, "Ready"),
				"this is the incident signature: OpenSandbox not-Running while the pod is Ready")
		})
	}
}

func TestAgrees_UnrecognisedStateNeverReportsDisagreement(t *testing.T) {
	// A future OpenSandbox state must ship as a blank cell, not a fleet-wide alarm.
	for _, phase := range []string{"Ready", "NotReady", "Unknown", ""} {
		require.True(t, agrees("SomeFutureState", phase))
	}
}

func TestIsStale_YoungNonTerminalStateIsNotStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	twentySecondsAgo := now.Add(-20 * time.Second)
	require.False(t, isStale("Pending", &twentySecondsAgo, now, time.Minute))
}

func TestIsStale_NonTerminalStatePastThresholdIsStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	nineMinutesAgo := now.Add(-9 * time.Minute)
	for _, state := range []string{"Pending", "Pausing", "Resuming", "Stopping"} {
		t.Run(state, func(t *testing.T) {
			require.True(t, isStale(state, &nineMinutesAgo, now, time.Minute))
		})
	}
}

func TestIsStale_RestingStatesAreNeverStaleRegardlessOfAge(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	longAgo := now.Add(-30 * time.Hour)
	// Running is where a healthy sandbox lives for hours; Terminated and Failed
	// are final; Paused is a state a caller deliberately holds.
	for _, state := range []string{"Running", "Terminated", "Failed", "Paused"} {
		t.Run(state, func(t *testing.T) {
			require.False(t, isStale(state, &longAgo, now, time.Minute))
		})
	}
}

func TestIsStale_MissingTimestampIsNotStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	require.False(t, isStale("Pending", nil, now, time.Minute))
}

func TestIsStale_UnrecognisedStateIsNotStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	longAgo := now.Add(-30 * time.Hour)
	require.False(t, isStale("SomeFutureState", &longAgo, now, time.Minute))
}

func TestNewOsbView_FlagsTheRecordedIncidentAsDivergedAndStale(t *testing.T) {
	// Recorded from algo-studio, sandbox 726f8779-…: created 14:08:57, pod Ready
	// at 14:08:59, OpenSandbox still Pending with lastTransitionAt == createdAt
	// when observed at 14:11:40.
	created := time.Date(2026, 8, 17, 14, 8, 57, 0, time.UTC)
	observed := time.Date(2026, 8, 17, 14, 11, 40, 0, time.UTC)
	s := osb.Sandbox{
		ID:        "726f8779-a7df-4c9c-a5ba-561c5f4a3acf",
		Status:    osb.Status{State: "Pending", Reason: "SANDBOX_PENDING", Message: "Sandbox is pending scheduling", LastTransitionAt: &created},
		CreatedAt: &created,
	}

	v := newOsbView(s, "Ready", observed, DefaultOsbStaleAfter)

	require.Equal(t, "Pending", v.State)
	require.Equal(t, "SANDBOX_PENDING", v.Reason)
	require.True(t, v.Diverged, "OpenSandbox Pending against a Ready pod is a disagreement")
	require.True(t, v.Stale, "the state had not moved in 2m43s")
	require.Equal(t, int64(163), v.StateAgeSeconds)
}

func TestNewOsbView_HealthyRunningSandboxFlagsNothing(t *testing.T) {
	transitioned := time.Date(2026, 8, 16, 19, 37, 17, 0, time.UTC)
	observed := time.Date(2026, 8, 17, 14, 11, 40, 0, time.UTC)
	s := osb.Sandbox{
		ID:     "fb52dbeb",
		Status: osb.Status{State: "Running", Reason: "DependenciesReady", LastTransitionAt: &transitioned},
	}

	v := newOsbView(s, "Ready", observed, DefaultOsbStaleAfter)

	require.False(t, v.Diverged)
	require.False(t, v.Stale, "a sandbox running for 18 hours is healthy, not stale")
}

// This is the case that justifies keeping the two flags separate: an ordinary
// in-flight creation disagrees for a moment, but nothing is wrong.
func TestNewOsbView_YoungPendingSandboxIsNotStale(t *testing.T) {
	created := time.Date(2026, 8, 17, 14, 10, 0, 0, time.UTC)
	observed := created.Add(3 * time.Second)
	s := osb.Sandbox{
		ID:     "young",
		Status: osb.Status{State: "Pending", LastTransitionAt: &created},
	}

	v := newOsbView(s, "Ready", observed, DefaultOsbStaleAfter)

	require.True(t, v.Diverged)
	require.False(t, v.Stale, "3s of disagreement must not raise the staleness alarm")
}
