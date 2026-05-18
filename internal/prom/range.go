// Package prom provides a thin Prometheus client used by the dashboard.
package prom

import (
	"fmt"
	"time"
)

// Range is a (start, end, step) triple consumed by the Prometheus query_range API.
type Range struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// ParseRange maps a short range token to a concrete Range, anchored at `now`.
// Empty input is treated as "1h".
func ParseRange(token string, now time.Time) (Range, error) {
	if token == "" {
		token = "1h"
	}
	var duration, step time.Duration
	switch token {
	case "15m":
		duration, step = 15*time.Minute, 15*time.Second
	case "1h":
		duration, step = time.Hour, 30*time.Second
	case "6h":
		duration, step = 6*time.Hour, 2*time.Minute
	case "24h":
		duration, step = 24*time.Hour, 5*time.Minute
	default:
		return Range{}, fmt.Errorf("unsupported range %q (allowed: 15m, 1h, 6h, 24h)", token)
	}
	return Range{
		Start: now.Add(-duration),
		End:   now,
		Step:  step,
	}, nil
}
