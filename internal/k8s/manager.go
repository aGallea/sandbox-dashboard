// Package k8s wires up the controller-runtime manager used by the dashboard.
package k8s

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
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

// NewManager builds a read-only controller-runtime manager with cluster-scoped informers
// for the agent-sandbox CRDs. It disables leader election and the built-in metrics server
// (the dashboard exposes its own /metrics via the HTTP router).
func NewManager(cfg *rest.Config) (manager.Manager, error) {
	return ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 NewScheme(),
		LeaderElection:         false,
		Metrics:                server.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
}
