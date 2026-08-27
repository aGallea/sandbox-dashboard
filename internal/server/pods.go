package server

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceGPU is the extended resource an accelerator sandbox reserves. Only
// the NVIDIA key is read: it is the only device plugin present on the fleets
// this dashboard targets, and the value is reported as-is rather than summed
// with other vendors' keys, which would be a meaningless total.
const ResourceGPU = "nvidia.com/gpu"

// PodView is the pod-derived slice of a sandbox row — the state a Sandbox CR
// cannot report about itself: where it landed, how it is really doing, and how
// much of the cluster it is holding. Nil when the sandbox has no pod, which is
// both "not scheduled yet" and "already gone".
type PodView struct {
	Name  string `json:"name"`
	Phase string `json:"phase"` // Pending | Running | Succeeded | Failed | Unknown
	Node  string `json:"node,omitempty"`
	// Image is the first container's image, which identifies the workload far
	// better than the sandbox name does — those are UUIDs in practice.
	Image string `json:"image,omitempty"`
	// Containers are the regular containers' names, in spec order — what the
	// logs endpoint accepts as ?container=.
	Containers []string `json:"containers"`
	Restarts   int32    `json:"restarts"`
	// WaitingReason is the first container waiting reason, e.g. CrashLoopBackOff
	// or ImagePullBackOff. Empty once every container is running.
	WaitingReason string `json:"waitingReason,omitempty"`

	// Requests summed over regular containers — what the scheduler actually
	// reserved, which is what the cluster is paying for whether or not the
	// sandbox uses it. Init containers are excluded: their reservation is
	// released before the sandbox is up.
	CPUMillis int64 `json:"cpuMillis"`
	MemBytes  int64 `json:"memBytes"`
	GPU       int64 `json:"gpu"`
}

// podsBySandboxUID indexes sandbox pods by the UID of the Sandbox that owns
// them. The controller sets a controller ownerReference on every pod it creates
// and on every warm-pool pod it adopts, which makes the UID the one join key
// that cannot be fooled by the name rewriting OpenSandbox does.
//
// namespace scopes the List when the caller only needs one namespace; empty
// means cluster-wide.
func podsBySandboxUID(ctx context.Context, r client.Reader, namespace string) (map[types.UID]*corev1.Pod, error) {
	var pods corev1.PodList
	var opts []client.ListOption
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := r.List(ctx, &pods, opts...); err != nil {
		return nil, err
	}
	out := make(map[types.UID]*corev1.Pod, len(pods.Items))
	for i := range pods.Items {
		if owner, ok := sandboxOwnerUID(&pods.Items[i]); ok {
			out[owner] = &pods.Items[i]
		}
	}
	return out, nil
}

// sandboxOwnerUID returns the UID of the Sandbox controlling the pod.
func sandboxOwnerUID(pod *corev1.Pod) (types.UID, bool) {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "Sandbox" && ref.Controller != nil && *ref.Controller {
			return ref.UID, true
		}
	}
	return "", false
}

// podViewFor returns the view of the pod owned by uid, or nil when the index
// holds none. A nil index — the pod List failed — yields nil for every
// sandbox, degrading the row rather than failing the request.
func podViewFor(index map[types.UID]*corev1.Pod, uid types.UID) *PodView {
	pod, ok := index[uid]
	if !ok {
		return nil
	}
	return newPodView(pod)
}

func newPodView(pod *corev1.Pod) *PodView {
	v := &PodView{
		Name:       pod.Name,
		Phase:      string(pod.Status.Phase),
		Node:       pod.Spec.NodeName,
		Containers: make([]string, 0, len(pod.Spec.Containers)),
	}
	if len(pod.Spec.Containers) > 0 {
		v.Image = pod.Spec.Containers[0].Image
	}
	for i := range pod.Spec.Containers {
		v.Containers = append(v.Containers, pod.Spec.Containers[i].Name)
		req := pod.Spec.Containers[i].Resources.Requests
		v.CPUMillis += req.Cpu().MilliValue()
		v.MemBytes += req.Memory().Value()
		if gpu, ok := req[ResourceGPU]; ok {
			v.GPU += gpu.Value()
		}
	}
	for i := range pod.Status.ContainerStatuses {
		st := &pod.Status.ContainerStatuses[i]
		v.Restarts += st.RestartCount
		if v.WaitingReason == "" && st.State.Waiting != nil {
			v.WaitingReason = st.State.Waiting.Reason
		}
	}

	// Init containers block the app containers, and while they run the app
	// container reports nothing but the placeholder "PodInitializing". So an init
	// container stuck pulling an image reads the same as one about to finish
	// unless we look past the placeholder at what is actually waiting.
	for i := range pod.Status.InitContainerStatuses {
		st := &pod.Status.InitContainerStatuses[i]
		if st.State.Waiting == nil {
			continue
		}
		v.WaitingReason = st.State.Waiting.Reason
		break
	}
	return v
}
