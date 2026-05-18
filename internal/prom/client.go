package prom

import (
	"context"
	"fmt"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Point is one (time, value) sample for a single series.
type Point struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

// Client wraps the Prometheus HTTP API. It is safe for concurrent use.
type Client struct {
	api v1.API
}

// NewClient builds a Client against the given Prometheus base URL
// (e.g. http://prometheus.monitoring.svc:9090).
func NewClient(baseURL string) (*Client, error) {
	c, err := promapi.NewClient(promapi.Config{Address: baseURL})
	if err != nil {
		return nil, fmt.Errorf("prometheus client: %w", err)
	}
	return &Client{api: v1.NewAPI(c)}, nil
}

// QueryRange runs a PromQL query_range and returns the points from the first
// series of the matrix result. Multi-series queries are not supported here —
// each chart series gets its own QueryRange call so the BFF can label the
// returned points without parsing PromQL.
func (c *Client) QueryRange(ctx context.Context, query string, r Range) ([]Point, error) {
	val, _, err := c.api.QueryRange(ctx, query, v1.Range{
		Start: r.Start,
		End:   r.End,
		Step:  r.Step,
	})
	if err != nil {
		return nil, err
	}
	matrix, ok := val.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("expected matrix result, got %T", val)
	}
	if len(matrix) == 0 {
		return []Point{}, nil
	}
	stream := matrix[0]
	out := make([]Point, 0, len(stream.Values))
	for _, s := range stream.Values {
		out = append(out, Point{
			Time:  s.Timestamp.Time(),
			Value: float64(s.Value),
		})
	}
	return out, nil
}
