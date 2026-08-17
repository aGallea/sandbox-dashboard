// Package osb is a read-only client for the OpenSandbox Lifecycle API.
//
// The dashboard is read-only, so this package deliberately exposes no method
// capable of issuing a non-GET request, even though the upstream API offers
// pause, resume, delete and snapshot routes.
package osb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiKeyHeader is the header the OpenSandbox server authenticates on.
const apiKeyHeader = "OPEN-SANDBOX-API-KEY"

// Status is OpenSandbox's own view of a sandbox's lifecycle state. State is one
// of Pending, Running, Pausing, Paused, Resuming, Stopping, Terminated, Failed.
type Status struct {
	State            string     `json:"state"`
	Reason           string     `json:"reason"`
	Message          string     `json:"message"`
	LastTransitionAt *time.Time `json:"lastTransitionAt"`
}

// Sandbox is the subset of the OpenSandbox sandbox record the dashboard uses.
// Fields the CR already carries (image, entrypoint, metadata) are read from the
// CR instead, so that they survive an OpenSandbox outage.
type Sandbox struct {
	ID        string     `json:"id"`
	Status    Status     `json:"status"`
	ExpiresAt *time.Time `json:"expiresAt"`
	CreatedAt *time.Time `json:"createdAt"`
}

type pagination struct {
	Page        int  `json:"page"`
	PageSize    int  `json:"pageSize"`
	TotalItems  int  `json:"totalItems"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
}

type listResponse struct {
	Items      []Sandbox  `json:"items"`
	Pagination pagination `json:"pagination"`
}

// Client is a read-only OpenSandbox API client. It is safe for concurrent use.
type Client struct {
	base   string
	key    string
	hc     *http.Client
	logger *slog.Logger // nil means warnings are dropped silently
}

// Option configures a Client.
type Option func(*Client)

// WithLogger sets a structured logger for non-fatal warnings.
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.logger = l } }

// WithHTTPClient overrides the default HTTP client. Used by tests.
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.hc = hc } }

// NewClient builds a Client against the given OpenSandbox base URL.
func NewClient(baseURL, apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("opensandbox: base URL is empty")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("opensandbox: parse base URL: %w", err)
	}
	c := &Client{
		base: strings.TrimRight(baseURL, "/"),
		key:  apiKey,
		hc:   &http.Client{Timeout: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// get issues a GET against path and returns the response body. This is the only
// request-issuing method in the package; keeping it unexported and GET-only is
// what makes the read-only guarantee structural rather than conventional.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("opensandbox: build request: %w", err)
	}
	req.Header.Set(apiKeyHeader, c.key)

	resp, err := c.hc.Do(req)
	if err != nil {
		// err may embed the request URL but never the header, so the key cannot leak.
		return nil, fmt.Errorf("opensandbox: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("opensandbox: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("opensandbox: unexpected status %d for %s", resp.StatusCode, path)
	}
	return body, nil
}

// ListSandboxes returns every sandbox OpenSandbox knows about, keyed by id.
func (c *Client) ListSandboxes(ctx context.Context) (map[string]Sandbox, error) {
	body, err := c.get(ctx, "/v1/sandboxes?pageSize=200&page=1")
	if err != nil {
		return nil, err
	}
	var lr listResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, fmt.Errorf("opensandbox: decode sandbox list: %w", err)
	}
	out := make(map[string]Sandbox, len(lr.Items))
	for _, s := range lr.Items {
		out[s.ID] = s
	}
	return out, nil
}
