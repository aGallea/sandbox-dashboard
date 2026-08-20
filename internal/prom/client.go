package prom

import (
	"context"
	"fmt"
	"log/slog"
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
	api    v1.API
	logger *slog.Logger // nil means warnings are dropped silently
}

// Option configures a Client.
type Option func(*Client)

// WithLogger sets a structured logger for non-fatal warnings (partial data, lookback, etc.).
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.logger = l } }

// NewClient builds a Client against the given Prometheus base URL.
// Optional behaviour (logger, etc.) is supplied via Option arguments.
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	c, err := promapi.NewClient(promapi.Config{Address: baseURL})
	if err != nil {
		return nil, fmt.Errorf("prometheus client: %w", err)
	}
	client := &Client{api: v1.NewAPI(c)}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

// Sample is one instant-vector result: the labels identifying a series and its
// value at query time.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// Query runs an instant PromQL query and returns the vector result. Warnings
// are forwarded to the configured logger, as in QueryRange.
func (c *Client) Query(ctx context.Context, query string, at time.Time) ([]Sample, error) {
	val, warns, err := c.api.Query(ctx, query, at)
	if err != nil {
		return nil, err
	}
	if c.logger != nil {
		for _, w := range warns {
			c.logger.Warn("prometheus_warning", "query", query, "message", w)
		}
	}
	vector, ok := val.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected vector result, got %T", val)
	}
	out := make([]Sample, 0, len(vector))
	for _, s := range vector {
		labels := make(map[string]string, len(s.Metric))
		for k, v := range s.Metric {
			labels[string(k)] = string(v)
		}
		out = append(out, Sample{Labels: labels, Value: float64(s.Value)})
	}
	return out, nil
}

// QueryRange runs a PromQL query_range and returns the points from the first
// series of the matrix result. Warnings emitted by Prometheus are forwarded
// to the configured logger (if any) at WARN level.
func (c *Client) QueryRange(ctx context.Context, query string, r Range) ([]Point, error) {
	val, warns, err := c.api.QueryRange(ctx, query, v1.Range{
		Start: r.Start,
		End:   r.End,
		Step:  r.Step,
	})
	if err != nil {
		return nil, err
	}
	if c.logger != nil {
		for _, w := range warns {
			c.logger.Warn("prometheus_warning", "query", query, "message", w)
		}
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
		out = append(out, Point{Time: s.Timestamp.Time(), Value: float64(s.Value)})
	}
	return out, nil
}
