package server

import (
	"context"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventEntry struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Reason  string    `json:"reason"`
	Message string    `json:"message"`
	Source  string    `json:"source"`
	Count   int32     `json:"count"`
}

// eventsForWindow returns up to `limit` events for the given object in the given
// namespace whose timestamp is no older than `window` before `now`. Sorted newest-first.
func eventsForWindow(ctx context.Context, reader client.Reader, namespace, name string, limit int, window time.Duration, now time.Time) ([]EventEntry, error) {
	var all corev1.EventList
	if err := reader.List(ctx, &all, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	cutoff := now.Add(-window)
	out := make([]EventEntry, 0, limit)
	for i := range all.Items {
		ev := &all.Items[i]
		if ev.InvolvedObject.Name != name {
			continue
		}
		ts := eventTime(ev)
		if ts.Before(cutoff) {
			continue
		}
		out = append(out, EventEntry{
			Time: ts, Type: ev.Type, Reason: ev.Reason, Message: ev.Message,
			Source: ev.Source.Component, Count: ev.Count,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// eventsFor is the default-window convenience wrapper used by detail handlers.
// Returns up to `limit` events from the last hour, newest-first.
func eventsFor(ctx context.Context, reader client.Reader, namespace, name string, limit int) ([]EventEntry, error) {
	return eventsForWindow(ctx, reader, namespace, name, limit, time.Hour, time.Now())
}

func eventTime(ev *corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.FirstTimestamp.Time
}
