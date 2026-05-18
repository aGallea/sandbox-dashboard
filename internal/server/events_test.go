package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aGallea/agent-sandbox-dashboard/internal/k8s"
)

func TestEventsFor_FiltersByInvolvedObjectAndSortsNewestFirst(t *testing.T) {
	now := time.Now()
	mkEvent := func(name, objName, msg string, ago time.Duration) *corev1.Event {
		return &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns1"},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Sandbox",
				Namespace: "ns1",
				Name:      objName,
			},
			Type:          corev1.EventTypeNormal,
			Reason:        "Test",
			Message:       msg,
			LastTimestamp: metav1.NewTime(now.Add(-ago)),
		}
	}

	c := fake.NewClientBuilder().
		WithScheme(k8s.NewScheme()).
		WithObjects(
			mkEvent("e1", "match", "older", 10*time.Minute),
			mkEvent("e2", "match", "newer", 1*time.Minute),
			mkEvent("e3", "other", "unrelated", 1*time.Minute),
		).
		Build()

	got, err := eventsFor(context.Background(), c, "ns1", "match", 10)
	require.NoError(t, err)
	require.Len(t, got, 2, "should only include events for 'match'")
	require.Equal(t, "newer", got[0].Message, "should be sorted newest-first")
	require.Equal(t, "older", got[1].Message)
}
