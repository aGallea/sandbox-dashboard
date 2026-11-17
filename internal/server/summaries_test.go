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

func TestCreatorFor_NamesTheSystemThatAskedForTheSandbox(t *testing.T) {
	isController := true
	controlledBy := func(apiVersion, kind string) []metav1.OwnerReference {
		return []metav1.OwnerReference{{APIVersion: apiVersion, Kind: kind, Name: "x", Controller: &isController}}
	}
	osbLabels := map[string]string{"opensandbox.io/id": "abc"}
	tests := []struct {
		name string
		meta metav1.ObjectMeta
		want string
	}{
		{"opensandbox id label present", metav1.ObjectMeta{Labels: osbLabels}, CreatorOpenSandbox},
		{"label present but empty is not a creator", metav1.ObjectMeta{Labels: map[string]string{"opensandbox.io/id": ""}}, CreatorUnknown},
		{"a different creator's labels", metav1.ObjectMeta{Labels: map[string]string{"app": "x", "swe-instance-id": "y"}}, CreatorUnknown},
		{"no labels at all", metav1.ObjectMeta{}, CreatorUnknown},
		{"made for a sandbox claim", metav1.ObjectMeta{OwnerReferences: controlledBy("extensions.agents.x-k8s.io/v1alpha1", "SandboxClaim")}, CreatorAgentSandbox},
		{"made for a warm pool", metav1.ObjectMeta{OwnerReferences: controlledBy("extensions.agents.x-k8s.io/v1alpha1", "SandboxWarmPool")}, CreatorAgentSandbox},
		{"opensandbox wins over the controller it runs on", metav1.ObjectMeta{Labels: osbLabels, OwnerReferences: controlledBy("extensions.agents.x-k8s.io/v1alpha1", "SandboxClaim")}, CreatorOpenSandbox},
		{"another controller's owner reference", metav1.ObjectMeta{OwnerReferences: controlledBy("batch/v1", "Job")}, CreatorUnknown},
		{"a non-controller owner from the extensions group", metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{APIVersion: "extensions.agents.x-k8s.io/v1alpha1", Kind: "SandboxClaim", Name: "x"}}}, CreatorUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, creatorFor(&tc.meta))
		})
	}
}
