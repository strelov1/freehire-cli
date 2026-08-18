// Package client is a thin HTTP client for the freehire API. It attaches an API
// key (Authorization: Bearer) when one is configured — omitting it for public
// read endpoints — and returns the raw `data` field of each response, so callers
// can print it verbatim (--json) or decode it.
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
// token when one is set (an empty token means unauthenticated public reads).
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

// IgnoredParam is a query param the API did not read, reported back in `meta`.
// DidYouMean carries the real facet name when the sent one was only its
// singular — the common miss, since most facets are plural.
type IgnoredParam struct {
	Param      string `json:"param"`
	DidYouMean string `json:"did_you_mean"`
}

// Doc is a single-item result: the raw `data` object plus any params the API
// ignored. The single-item sibling of Page — the filtered endpoints that answer
// one object rather than a list report the same warning, and losing it on the
// way through would leave a count or a percentage looking authoritative when it
// answered a wider question than the caller asked.
type Doc struct {
	Data    json.RawMessage
	Ignored []IgnoredParam
}

// Page is a slice of list results: the raw `data` array, the total match count
// from `meta`, and any params the API ignored. Returned by Search and MyJobs.
//
// Ignored matters more than it looks: an unread filter does not fail the
// request, it widens it, so the count comes back larger and reads as a real
// answer. Callers surface it rather than dropping it.
type Page struct {
	Data    json.RawMessage
	Total   int
	Ignored []IgnoredParam
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
		Total         int            `json:"total"`
		IgnoredParams []IgnoredParam `json:"ignored_params"`
	} `json:"meta"`
	Error string `json:"error"`
}

// Me returns the authenticated user (GET /auth/me). It works by API key, so it
// is the CLI's whoami.
func (c *Client) Me(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil)
	return env.Data, err
}

// Search runs a keyword job search with optional facet filters
// (GET /agent/jobs/search). That endpoint runs the same query as the web's
// /jobs/search but, for programmatic consumers, replaces the index's truncated
// preview with each job's full description — so a caller reads a result set
// without a follow-up GetJob per hit. Markdown keeps the posting's lists and
// headings readable both in an agent's context and in the terminal.
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
	// The endpoint always returns full descriptions, so only the rendering needs
	// asking for. It used to also send semantic_ratio=0 and
	// include_description=true; the first died with the hybrid index and the
	// second was never read, and the API now reports unread params as warnings —
	// so sending them would hand the user two they cannot act on.
	q.Set("description_format", "markdown")
	env, err := c.do(ctx, http.MethodGet, "/api/v1/agent/jobs/search?"+q.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	return Page{Data: env.Data, Total: env.Meta.Total, Ignored: env.Meta.IgnoredParams}, nil
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
func (c *Client) Coverage(ctx context.Context, p CoverageParams) (Doc, error) {
	body, err := json.Marshal(struct {
		Skills []string `json:"skills"`
	}{Skills: p.Skills})
	if err != nil {
		return Doc{}, err
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
	return Doc{Data: env.Data, Ignored: env.Meta.IgnoredParams}, err
}

// Facets returns the market's facet-value distributions under an optional filter
// (GET /jobs/facets): each facet's live values with counts, plus numeric stats. It
// is the vocabulary an agent reads to know which filter values and skills exist.
func (c *Client) Facets(ctx context.Context, facets url.Values) (Doc, error) {
	q := url.Values{}
	for k, vs := range facets {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	path := "/api/v1/jobs/facets"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	env, err := c.do(ctx, http.MethodGet, path, nil)
	return Doc{Data: env.Data, Ignored: env.Meta.IgnoredParams}, err
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

// MyJobs lists the caller's tracked jobs (GET /me/tracking), filtered by
// all/viewed/saved/applied.
func (c *Client) MyJobs(ctx context.Context, filter string, limit, offset int) (Page, error) {
	q := url.Values{}
	if filter != "" {
		q.Set("filter", filter)
	}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/tracking?"+q.Encode(), nil)
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
//
// appliedOn is the day the application was actually sent, as a plain calendar date
// (YYYY-MM-DD), for recording a history after the fact — the server stores that day rather than
// today, and it overrides a date already recorded. Empty means today, and sends no body at all:
// that is the request every existing caller makes, and it must stay byte-for-byte what it was.
//
// The date is not validated here. The server refuses one in the future or older than a year,
// and a second copy of that window in the CLI would answer differently the day the server's
// moved.
func (c *Client) Apply(ctx context.Context, slug, appliedOn string) (json.RawMessage, error) {
	path := "/api/v1/jobs/" + url.PathEscape(slug) + "/apply"
	if appliedOn == "" {
		env, err := c.do(ctx, http.MethodPost, path, nil)
		return env.Data, err
	}
	body, err := json.Marshal(map[string]string{"applied_on": appliedOn})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, path, bytes.NewReader(body))
	return env.Data, err
}

// ReportGhost files the caller's claim that they applied to slug and were never
// answered (POST /jobs/:slug/ghost-report). It is evidence for the posting's
// "possibly inactive" signal and reaches no moderator: nothing here closes a job.
//
// appliedOn is a plain calendar date (YYYY-MM-DD) because the reporter is stating a
// day, not an instant — a timezone-bearing timestamp would read as a different day
// either side of a border. The server refuses a future date, one older than a year,
// a second live claim on the same posting, an unverified address, and a closed job.
func (c *Client) ReportGhost(ctx context.Context, slug, appliedOn string) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]string{"applied_on": appliedOn})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost,
		"/api/v1/jobs/"+url.PathEscape(slug)+"/ghost-report", bytes.NewReader(body))
	return env.Data, err
}

// RetractGhostReport withdraws the caller's claim about slug
// (DELETE /jobs/:slug/ghost-report). The API answers 204 with no body, so there is
// nothing to return but an error; a claim that is absent or already withdrawn is a 404.
func (c *Client) RetractGhostReport(ctx context.Context, slug string) error {
	_, err := c.do(ctx, http.MethodDelete,
		"/api/v1/jobs/"+url.PathEscape(slug)+"/ghost-report", nil)
	return err
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

// Contribute hands one job link to freehire (POST /jobs/resolve) — the same intake the
// website, the Telegram bot and the browser extension use. The server checks the catalog,
// imports the vacancy when anything can read it, and records the board behind it for
// onboarding either way; the answer says which of those happened.
func (c *Client) Contribute(ctx context.Context, link string) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]string{"url": link, "surface": "cli"})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/jobs/resolve", bytes.NewReader(body))
	return env.Data, err
}

// MyContributions lists the boards the caller has contributed (GET /me/contributions).
func (c *Client) MyContributions(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/contributions", nil)
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
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

// GetProfile reads the caller's saved job-search profile: their specializations,
// skills, excluded skills and location preferences, plus their CV projected without
// its contact fields. Returns a null payload when the caller has saved none.
func (c *Client) GetProfile(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/profile", nil)
	return env.Data, err
}
