package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aGallea/sandbox-dashboard/internal/k8s"
	v1alpha1 "sigs.k8s.io/agent-sandbox/api/v1alpha1"
)

// sandboxPod builds a pod owned by the given Sandbox UID, the way the
// agent-sandbox controller does.
func sandboxPod(name, namespace string, ownerUID string, requests corev1.ResourceList) *corev1.Pod {
	yes := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "agents.x-k8s.io/v1alpha1", Kind: "Sandbox",
				Name: name, UID: types.UID(ownerUID), Controller: &yes,
			}},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Containers: []corev1.Container{{
				Name: "sandbox", Image: "alexgshaw/qemu-alpine-ssh:20251031",
				Resources: corev1.ResourceRequirements{Requests: requests},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func requests(cpu, mem string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
}

func TestSandboxList_JoinsThePodOwnedByEachSandbox(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-b", Namespace: "default", UID: types.UID("uid-b")}},
		sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi")),
		sandboxPod("pod-b", "default", "uid-b", requests("16", "16Gi")),
	}
	rows := listSandboxes(t, objs)

	a := rowNamed(t, rows, "sb-a")
	require.NotNil(t, a.Pod)
	require.Equal(t, "pod-a", a.Pod.Name)
	require.Equal(t, "Running", a.Pod.Phase)
	require.Equal(t, "node-a", a.Pod.Node)
	require.Equal(t, "alexgshaw/qemu-alpine-ssh:20251031", a.Pod.Image)
	require.Equal(t, int64(1000), a.Pod.CPUMillis)
	require.Equal(t, int64(2*1024*1024*1024), a.Pod.MemBytes)

	b := rowNamed(t, rows, "sb-b")
	require.NotNil(t, b.Pod)
	require.Equal(t, int64(16000), b.Pod.CPUMillis)
}

func TestSandboxList_SandboxWithoutAPodCarriesNoPodBlock(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
	}
	require.Nil(t, rowNamed(t, listSandboxes(t, objs), "sb-a").Pod)
}

// A pod that merely lives in the namespace must not be attributed to a
// sandbox: only a controller ownerReference of kind Sandbox joins.
func TestSandboxList_IgnoresPodsNotControlledByASandbox(t *testing.T) {
	no := false
	unowned := sandboxPod("stray", "default", "uid-a", requests("4", "8Gi"))
	unowned.OwnerReferences = nil

	notController := sandboxPod("adopted-elsewhere", "default", "uid-a", requests("4", "8Gi"))
	notController.OwnerReferences[0].Controller = &no

	otherKind := sandboxPod("replica", "default", "uid-a", requests("4", "8Gi"))
	otherKind.OwnerReferences[0].Kind = "ReplicaSet"

	for name, pod := range map[string]*corev1.Pod{
		"no owner references": unowned,
		"not the controller":  notController,
		"another kind":        otherKind,
	} {
		t.Run(name, func(t *testing.T) {
			objs := []client.Object{
				&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
				pod,
			}
			require.Nil(t, rowNamed(t, listSandboxes(t, objs), "sb-a").Pod)
		})
	}
}

func TestSandboxList_SumsRequestsAcrossContainersAndReadsGPUs(t *testing.T) {
	pod := sandboxPod("pod-a", "default", "uid-a", corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
		ResourceGPU:           resource.MustParse("2"),
	})
	// A sidecar's reservation is held for as long as the sandbox runs, so it
	// counts; an init container's is already released.
	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name: "proxy", Resources: corev1.ResourceRequirements{Requests: requests("500m", "256Mi")},
	})
	pod.Spec.InitContainers = []corev1.Container{{
		Name: "setup", Resources: corev1.ResourceRequirements{Requests: requests("8", "32Gi")},
	}}

	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		pod,
	}
	got := rowNamed(t, listSandboxes(t, objs), "sb-a").Pod
	require.NotNil(t, got)
	require.Equal(t, int64(2500), got.CPUMillis)
	require.Equal(t, int64(4*1024*1024*1024+256*1024*1024), got.MemBytes)
	require.Equal(t, int64(2), got.GPU)
}

func TestSandboxList_SurfacesRestartsAndTheWaitingReason(t *testing.T) {
	pod := sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi"))
	pod.Status.Phase = corev1.PodPending
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "sandbox", RestartCount: 7,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
	}}

	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		pod,
	}
	got := rowNamed(t, listSandboxes(t, objs), "sb-a").Pod
	require.NotNil(t, got)
	require.Equal(t, "Pending", got.Phase)
	require.Equal(t, int32(7), got.Restarts)
	require.Equal(t, "CrashLoopBackOff", got.WaitingReason)
}

// The overview page builds its grouping dimensions from whatever labels the
// fleet actually carries, so the raw labels have to reach the client.
func TestSandboxList_ExposesSandboxLabelsForGroupingDiscovery(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{
			Name: "sb-a", Namespace: "default", UID: types.UID("uid-a"),
			Labels: map[string]string{"policy.ai21.com/preemptible": "false", "session_id": "arc-agi__env"},
		}},
	}
	got := rowNamed(t, listSandboxes(t, objs), "sb-a")
	require.Equal(t, "false", got.Labels["policy.ai21.com/preemptible"])
	require.Equal(t, "arc-agi__env", got.Labels["session_id"])
}

func TestSandboxDetail_IncludesThePodView(t *testing.T) {
	objs := []client.Object{
		&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
		sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi")),
	}
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	rec := httptest.NewRecorder()
	New(Deps{Client: c, CacheSynced: func() bool { return true }}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes/default/sb-a", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got SandboxDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.NotNil(t, got.Summary.Pod)
	require.Equal(t, "pod-a", got.Summary.Pod.Name)
	require.Equal(t, int64(1000), got.Summary.Pod.CPUMillis)
}

// ----- helpers --------------------------------------------------------------

func listSandboxes(t *testing.T, objs []client.Object) []ResourceSummary {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(k8s.NewScheme()).WithObjects(objs...).Build()
	rec := httptest.NewRecorder()
	New(Deps{Client: c, CacheSynced: func() bool { return true }}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sandboxes", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var got sandboxListBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got.Items
}

func rowNamed(t *testing.T, rows []ResourceSummary, name string) ResourceSummary {
	t.Helper()
	for i := range rows {
		if rows[i].Name == name {
			return rows[i]
		}
	}
	t.Fatalf("no row named %q in %d rows", name, len(rows))
	return ResourceSummary{}
}

// While an init container runs, the app container reports the placeholder
// "PodInitializing" and nothing else. Measured on a real fleet, every pending
// sandbox reported exactly that — so an init container stuck pulling its image
// looked identical to one a second away from starting.
func TestSandboxList_PrefersTheInitContainerReasonOverThePlaceholder(t *testing.T) {
	waiting := func(reason string) corev1.ContainerState {
		return corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}
	}

	t.Run("reports what the init container is stuck on", func(t *testing.T) {
		pod := sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi"))
		pod.Status.Phase = corev1.PodPending
		pod.Status.InitContainerStatuses = []corev1.ContainerStatus{
			{Name: "installer", State: waiting("ImagePullBackOff")},
		}
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{
			{Name: "sandbox", State: waiting("PodInitializing")},
		}

		objs := []client.Object{
			&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
			pod,
		}
		got := rowNamed(t, listSandboxes(t, objs), "sb-a").Pod
		require.NotNil(t, got)
		require.Equal(t, "ImagePullBackOff", got.WaitingReason)
	})

	// An init container that has finished is not waiting on anything, so the app
	// container's own reason is the truthful one.
	t.Run("falls back to the app container once init has finished", func(t *testing.T) {
		pod := sandboxPod("pod-a", "default", "uid-a", requests("1", "2Gi"))
		pod.Status.Phase = corev1.PodPending
		pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
			Name:  "installer",
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}},
		}}
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{
			{Name: "sandbox", State: waiting("ContainerCreating")},
		}

		objs := []client.Object{
			&v1alpha1.Sandbox{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "default", UID: types.UID("uid-a")}},
			pod,
		}
		got := rowNamed(t, listSandboxes(t, objs), "sb-a").Pod
		require.NotNil(t, got)
		require.Equal(t, "ContainerCreating", got.WaitingReason)
	})
}
