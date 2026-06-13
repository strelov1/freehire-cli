// Package client is a thin HTTP client for the freehire API. It authenticates
// with an API key (Authorization: Bearer) and returns the raw `data` field of
// each response, so callers can print it verbatim (--json) or decode it.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Client talks to the freehire API at baseURL, sending the API key as a bearer
// token on every request.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New returns a Client for baseURL using token. A nil hc uses http.DefaultClient.
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: hc}
}

// APIError is a non-2xx API response, carrying the HTTP status so callers can
// branch on it (e.g. 401 → prompt to log in).
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("api error %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("api error %d", e.Status)
}

// SearchResult is a page of search hits: the raw `data` array plus the total
// match count from `meta`.
type SearchResult struct {
	Data  json.RawMessage
	Total int
}

// envelope is the shared API response wrapper: {data, meta, error}.
type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Error string `json:"error"`
}

// Me returns the authenticated user (GET /auth/me). It works by API key, so it
// is the CLI's whoami.
func (c *Client) Me(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil)
	return env.Data, err
}

// Search runs a keyword job search (GET /jobs/search).
func (c *Client) Search(ctx context.Context, query string, limit, offset int) (SearchResult, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("semantic_ratio", "0") // keyword search, matching the web client
	env, err := c.do(ctx, http.MethodGet, "/api/v1/jobs/search?"+q.Encode(), nil)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Data: env.Data, Total: env.Meta.Total}, nil
}

// GetJob fetches a single job by its public slug (GET /jobs/:slug).
func (c *Client) GetJob(ctx context.Context, slug string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/jobs/"+url.PathEscape(slug), nil)
	return env.Data, err
}

// Apply marks a job applied for the authenticated user (POST /jobs/:slug/apply).
func (c *Client) Apply(ctx context.Context, slug string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodPost, "/api/v1/jobs/"+url.PathEscape(slug)+"/apply", nil)
	return env.Data, err
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (envelope, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return envelope{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return envelope{}, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return envelope{}, err
	}

	var env envelope
	if len(b) > 0 {
		// A malformed body on a 2xx is unexpected; surface it. On a non-2xx an
		// unparseable body just leaves env.Error empty (the status still carries).
		if uerr := json.Unmarshal(b, &env); uerr != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return envelope{}, fmt.Errorf("decode response: %w", uerr)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return envelope{}, &APIError{Status: resp.StatusCode, Message: env.Error}
	}
	return env, nil
}
