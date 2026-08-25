// Package k8s wires up the controller-runtime manager used by the dashboard.
package k8s

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
	extv1alpha1 "sigs.k8s.io/agent-sandbox/extensions/api/v1alpha1"
)

// NewScheme returns a runtime.Scheme with core k8s types and all agent-sandbox CRDs registered.
func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(v1alpha1.AddToScheme(s))
	utilruntime.Must(extv1alpha1.AddToScheme(s))
	return s
}

// cacheOptions scopes the informers to namespaces. An empty list means every
// namespace, which is what a ClusterRole grants.
//
// This must agree with the RBAC the install was given. A namespace-scoped Role
// with cluster-wide informers fails closed but late: the list calls 403, the
// cache never syncs, and /readyz stays 503 with the real cause buried in the
// manager's logs. The chart derives both from one value for that reason.
func cacheOptions(namespaces []string) cache.Options {
	if len(namespaces) == 0 {
		return cache.Options{}
	}
	byNamespace := make(map[string]cache.Config, len(namespaces))
	for _, ns := range namespaces {
		byNamespace[ns] = cache.Config{}
	}
	return cache.Options{DefaultNamespaces: byNamespace}
}

// NewManager builds a read-only controller-runtime manager with informers for the
// agent-sandbox CRDs, watching the given namespaces (all of them when empty). It
// disables leader election and the built-in metrics server (the dashboard exposes
// its own /metrics via the HTTP router).
func NewManager(cfg *rest.Config, namespaces []string) (manager.Manager, error) {
	return ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 NewScheme(),
		LeaderElection:         false,
		Metrics:                server.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Cache:                  cacheOptions(namespaces),
	})
}
