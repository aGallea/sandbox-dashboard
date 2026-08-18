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
	owner, team, experiment := identityFor(map[string]string{
		"owner": "odeda", "team": "intelligent-gateway", "experiment": "tbv-v2",
	})
	require.Equal(t, "odeda", owner)
	require.Equal(t, "intelligent-gateway", team)
	require.Equal(t, "tbv-v2", experiment)
}

func TestIdentityFor_ToleratesPartialMetadata(t *testing.T) {
	// 30 of 92 sandboxes measured in algo-studio carried only session_id.
	owner, team, experiment := identityFor(map[string]string{"session_id": "abc__env"})
	require.Empty(t, owner)
	require.Empty(t, team)
	require.Empty(t, experiment)
}
