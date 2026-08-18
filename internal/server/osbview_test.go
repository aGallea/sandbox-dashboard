package server

import (
	"testing"

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
