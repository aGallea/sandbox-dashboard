package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

func TestScheme_RegistersAllCRDs(t *testing.T) {
	s := NewScheme()

	require.True(t, s.Recognizes(v1alpha1.GroupVersion.WithKind("Sandbox")),
		"scheme should recognize Sandbox")
	require.True(t, s.Recognizes(extv1alpha1.GroupVersion.WithKind("SandboxClaim")),
		"scheme should recognize SandboxClaim")
	require.True(t, s.Recognizes(extv1alpha1.GroupVersion.WithKind("SandboxTemplate")),
		"scheme should recognize SandboxTemplate")
	require.True(t, s.Recognizes(extv1alpha1.GroupVersion.WithKind("SandboxWarmPool")),
		"scheme should recognize SandboxWarmPool")
}
