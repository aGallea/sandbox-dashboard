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

func TestCacheOptions_EmptyMeansEveryNamespace(t *testing.T) {
	// A nil DefaultNamespaces is what controller-runtime treats as cluster-wide,
	// which is the behaviour a ClusterRole grants and the chart's default.
	require.Nil(t, cacheOptions(nil).DefaultNamespaces)
	require.Nil(t, cacheOptions([]string{}).DefaultNamespaces)
}

func TestCacheOptions_ScopesToExactlyTheNamespacesGiven(t *testing.T) {
	opts := cacheOptions([]string{"default", "team-a"})

	require.Len(t, opts.DefaultNamespaces, 2)
	require.Contains(t, opts.DefaultNamespaces, "default")
	require.Contains(t, opts.DefaultNamespaces, "team-a")
	require.NotContains(t, opts.DefaultNamespaces, "kube-system")
}

func TestCacheOptions_IgnoresRepeatedNamespaces(t *testing.T) {
	// `--watch-namespaces=default,default` is a typo, not a reason to fail.
	require.Len(t, cacheOptions([]string{"default", "default"}).DefaultNamespaces, 1)
}
