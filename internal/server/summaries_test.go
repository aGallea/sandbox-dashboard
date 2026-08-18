package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadyPhase_Mapping(t *testing.T) {
	require.Equal(t, "Ready", readyPhase([]metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}))
	require.Equal(t, "NotReady", readyPhase([]metav1.Condition{{Type: "Ready", Status: metav1.ConditionFalse}}))
	require.Equal(t, "Unknown", readyPhase([]metav1.Condition{{Type: "Ready", Status: metav1.ConditionUnknown}}))
	require.Equal(t, "Unknown", readyPhase(nil))
	require.Equal(t, "Unknown", readyPhase([]metav1.Condition{{Type: "Other", Status: metav1.ConditionTrue}}))
}

func TestAge_FromCreationTimestamp(t *testing.T) {
	now := time.Now()
	meta := metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute))}
	age := ageSeconds(meta, now)
	require.InDelta(t, 300, age, 1)
}

func TestCreatorFor_IdentifiesOpenSandboxByLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"opensandbox id label present", map[string]string{"opensandbox.io/id": "abc"}, CreatorOpenSandbox},
		{"label present but empty is not a creator", map[string]string{"opensandbox.io/id": ""}, CreatorUnknown},
		{"a different creator's labels", map[string]string{"app": "x", "swe-instance-id": "y"}, CreatorUnknown},
		{"no labels at all", nil, CreatorUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, creatorFor(tc.labels))
		})
	}
}

func TestIdentityFor_ReadsOwnerTeamExperimentFromLabels(t *testing.T) {
	owner, team, experiment, _ := identityFor(map[string]string{
		"owner": "odeda", "team": "intelligent-gateway", "experiment": "tbv-v2",
	})
	require.Equal(t, "odeda", owner)
	require.Equal(t, "intelligent-gateway", team)
	require.Equal(t, "tbv-v2", experiment)
}

func TestIdentityFor_ReadsOwnerFromTheDomainQualifiedLabel(t *testing.T) {
	owner, _, _, _ := identityFor(map[string]string{"ai21.com/owner": "odeda"})
	require.Equal(t, "odeda", owner)
}

func TestIdentityFor_PrefersTheDomainQualifiedOwnerLabel(t *testing.T) {
	owner, _, _, _ := identityFor(map[string]string{
		"ai21.com/owner": "odeda", "owner": "stale-value",
	})
	require.Equal(t, "odeda", owner)
}

func TestIdentityFor_IgnoresAnEmptyOwnerLabel(t *testing.T) {
	owner, _, _, _ := identityFor(map[string]string{"ai21.com/owner": "", "owner": "odeda"})
	require.Equal(t, "odeda", owner)
}

func TestIdentityFor_ToleratesPartialMetadata(t *testing.T) {
	// 30 of 92 sandboxes measured in algo-studio carried only session_id.
	owner, team, experiment, sessionID := identityFor(map[string]string{"session_id": "abc__env"})
	require.Empty(t, owner)
	require.Empty(t, team)
	require.Empty(t, experiment)
	require.Equal(t, "abc__env", sessionID)
}

func TestIdentityFor_ReadsSessionIDFromLabels(t *testing.T) {
	_, _, _, sessionID := identityFor(map[string]string{"session_id": "regex-chess__33BjxVG__env"})
	require.Equal(t, "regex-chess__33BjxVG__env", sessionID)
}

func TestIdentityFor_SessionIDIsTheOnlyIdentityOnAThinlyLabelledSandbox(t *testing.T) {
	// Measured on the live cluster: 166 of 166 sandboxes carried only these three
	// labels, so session_id is the sole identity signal available for that fleet.
	labels := map[string]string{
		"opensandbox.io/id":           "abc",
		"policy.ai21.com/preemptible": "false",
		"session_id":                  "pytorch-model-recovery__ocs9ngs__env",
	}
	owner, team, experiment, sessionID := identityFor(labels)
	require.Empty(t, owner)
	require.Empty(t, team)
	require.Empty(t, experiment)
	require.Equal(t, "pytorch-model-recovery__ocs9ngs__env", sessionID)
}
