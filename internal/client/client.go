// Package client is a thin HTTP client for the freehire API. It authenticates
// with an API key (Authorization: Bearer) and returns the raw `data` field of
// each response, so callers can print it verbatim (--json) or decode it.
package client

import (
	"bytes"
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

// Page is a slice of list results: the raw `data` array plus the total match
// count from `meta`. Returned by Search and MyJobs.
type Page struct {
	Data  json.RawMessage
	Total int
}

// SearchParams are the inputs to a job search: query text, pagination, and
// optional facet filters (work_mode, regions, company_slug, …) as query values.
type SearchParams struct {
	Query  string
	Limit  int
	Offset int
	Facets url.Values
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

// Search runs a keyword job search with optional facet filters (GET /jobs/search).
func (c *Client) Search(ctx context.Context, p SearchParams) (Page, error) {
	q := url.Values{}
	for k, vs := range p.Facets {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("q", p.Query)
	q.Set("limit", strconv.Itoa(p.Limit))
	q.Set("offset", strconv.Itoa(p.Offset))
	q.Set("semantic_ratio", "0") // keyword search, matching the web client
	env, err := c.do(ctx, http.MethodGet, "/api/v1/jobs/search?"+q.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	return Page{Data: env.Data, Total: env.Meta.Total}, nil
}

// CoverageParams is a market-coverage query: Skills is the measured skill list
// (sent in the request body), Facets narrows the market (sent as query params —
// the full facet vocabulary).
type CoverageParams struct {
	Skills []string
	Facets url.Values
}

// Coverage scores a skill list against the facet-filtered market
// (POST /market/coverage): how many open vacancies for the filter list at least
// one of the skills, plus ranked skill gaps and the role's top in-demand skills.
// One skill or many — a single-element Skills probes that skill's demand.
func (c *Client) Coverage(ctx context.Context, p CoverageParams) (json.RawMessage, error) {
	body, err := json.Marshal(struct {
		Skills []string `json:"skills"`
	}{Skills: p.Skills})
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	for k, vs := range p.Facets {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	path := "/api/v1/market/coverage"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	env, err := c.do(ctx, http.MethodPost, path, bytes.NewReader(body))
	return env.Data, err
}

// Save bookmarks a job (POST /jobs/:slug/save).
func (c *Client) Save(ctx context.Context, slug string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodPost, "/api/v1/jobs/"+url.PathEscape(slug)+"/save", nil)
	return env.Data, err
}

// Unsave removes a job's bookmark (DELETE /jobs/:slug/save).
func (c *Client) Unsave(ctx context.Context, slug string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodDelete, "/api/v1/jobs/"+url.PathEscape(slug)+"/save", nil)
	return env.Data, err
}

// MyJobs lists the caller's tracked jobs (GET /me/jobs), filtered by
// all/viewed/saved/applied.
func (c *Client) MyJobs(ctx context.Context, filter string, limit, offset int) (Page, error) {
	q := url.Values{}
	if filter != "" {
		q.Set("filter", filter)
	}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/jobs?"+q.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	return Page{Data: env.Data, Total: env.Meta.Total}, nil
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

// TrackParams are the optional fields of a track update. A nil field is omitted
// from the request body, so the server leaves that column unchanged (partial
// update); at least one must be set.
type TrackParams struct {
	Stage *string `json:"stage,omitempty"`
	Notes *string `json:"notes,omitempty"`
}

// Track sets a job's application stage and/or notes (PATCH /jobs/:slug/track).
func (c *Client) Track(ctx context.Context, slug string, p TrackParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPatch, "/api/v1/jobs/"+url.PathEscape(slug)+"/track", bytes.NewReader(body))
	return env.Data, err
}

// GetCompany fetches a company and its open jobs by slug (GET /companies/:slug).
func (c *Client) GetCompany(ctx context.Context, slug string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/companies/"+url.PathEscape(slug), nil)
	return env.Data, err
}

// CreateJobParams is the body for creating a moderator-authored job (POST /jobs).
// URL (the dedup key), Title, and Company are required by the server; the rest is
// optional. Source is the posting's real origin (the server defaults it to "manual"
// when omitted). PostedAt is an optional RFC3339 timestamp, omitted when nil.
type CreateJobParams struct {
	URL         string  `json:"url"`
	Source      string  `json:"source,omitempty"`
	Title       string  `json:"title"`
	Company     string  `json:"company"`
	Location    string  `json:"location,omitempty"`
	Remote      bool    `json:"remote"`
	Description string  `json:"description,omitempty"`
	PostedAt    *string `json:"posted_at,omitempty"`
}

// CreateJob creates a hand-curated job (POST /jobs, moderator only). Re-creating the
// same URL updates the posting (idempotent upsert on the server).
func (c *Client) CreateJob(ctx context.Context, p CreateJobParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/jobs", bytes.NewReader(body))
	return env.Data, err
}

// EditJobParams is the body for editing a manual job (PATCH /jobs/:slug). Every field
// is optional: a nil field is omitted, so the server leaves that column unchanged
// (partial update). The URL identity is not editable.
type EditJobParams struct {
	Title       *string `json:"title,omitempty"`
	Company     *string `json:"company,omitempty"`
	Location    *string `json:"location,omitempty"`
	Remote      *bool   `json:"remote,omitempty"`
	Description *string `json:"description,omitempty"`
	PostedAt    *string `json:"posted_at,omitempty"`
}

// EditJob partially updates a manual job (PATCH /jobs/:slug, moderator only).
func (c *Client) EditJob(ctx context.Context, slug string, p EditJobParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPatch, "/api/v1/jobs/"+url.PathEscape(slug), bytes.NewReader(body))
	return env.Data, err
}

// Submit queues a vacancy for moderation (POST /submissions). The body shape is the
// same as a moderator create; the server stores it as pending and returns it.
func (c *Client) Submit(ctx context.Context, p CreateJobParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/submissions", bytes.NewReader(body))
	return env.Data, err
}

// MySubmissions lists the caller's own submissions with their status (GET /me/submissions).
func (c *Client) MySubmissions(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/submissions", nil)
	return env.Data, err
}

// PendingSubmissions lists the moderator review queue (GET /submissions, moderator only).
func (c *Client) PendingSubmissions(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/submissions", nil)
	return env.Data, err
}

// ApproveSubmission approves a pending submission, minting a live job
// (POST /submissions/:id/approve, moderator only).
func (c *Client) ApproveSubmission(ctx context.Context, id int64) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodPost, "/api/v1/submissions/"+strconv.FormatInt(id, 10)+"/approve", nil)
	return env.Data, err
}

// RejectSubmission rejects a pending submission with an optional reason
// (POST /submissions/:id/reject, moderator only).
func (c *Client) RejectSubmission(ctx context.Context, id int64, reason string) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/submissions/"+strconv.FormatInt(id, 10)+"/reject", bytes.NewReader(body))
	return env.Data, err
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (envelope, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return envelope{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

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
