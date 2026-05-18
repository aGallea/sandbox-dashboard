package server

import (
	"context"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EventEntry is the per-event DTO returned by detail endpoints.
type EventEntry struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"` // Normal | Warning
	Reason  string    `json:"reason"`
	Message string    `json:"message"`
	Source  string    `json:"source"` // controller name, e.g. agent-sandbox-controller
	Count   int32     `json:"count"`
}

// eventsFor returns up to `limit` events for the given object in the given namespace,
// sorted newest-first.
func eventsFor(ctx context.Context, reader client.Reader, namespace, name string, limit int) ([]EventEntry, error) {
	var all corev1.EventList
	if err := reader.List(ctx, &all, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	out := make([]EventEntry, 0, limit)
	for i := range all.Items {
		ev := &all.Items[i]
		if ev.InvolvedObject.Name != name {
			continue
		}
		out = append(out, EventEntry{
			Time:    eventTime(ev),
			Type:    ev.Type,
			Reason:  ev.Reason,
			Message: ev.Message,
			Source:  ev.Source.Component,
			Count:   ev.Count,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// eventTime picks the most representative timestamp from an Event:
// LastTimestamp if set, otherwise EventTime, otherwise FirstTimestamp.
func eventTime(ev *corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.FirstTimestamp.Time
}
