package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// InboxParams filters the inbox listing. Source narrows to one account
// (gmail|hosted|external), Status to one classified label. WithBody asks for each
// message's readable body inline — the agent's read path, which returns a whole
// page to classify without a per-message fetch, and marks nothing read.
// Unclassified narrows to mail still awaiting a verdict: the agent's work queue.
type InboxParams struct {
	Query        string
	Source       string
	Status       string
	Unread       bool
	Unclassified bool
	WithBody     bool
	Limit        int
	Offset       int
}

// Inbox lists the caller's mail, newest first (GET /me/inbox).
func (c *Client) Inbox(ctx context.Context, p InboxParams) (Page, error) {
	q := url.Values{}
	if p.Query != "" {
		q.Set("q", p.Query)
	}
	if p.Source != "" {
		q.Set("source", p.Source)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Unread {
		q.Set("unread", "1")
	}
	if p.Unclassified {
		q.Set("unclassified", "1")
	}
	if p.WithBody {
		q.Set("body", "1")
	}
	q.Set("limit", strconv.Itoa(p.Limit))
	q.Set("offset", strconv.Itoa(p.Offset))
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/inbox?"+q.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	return Page{Data: env.Data, Total: env.Meta.Total}, nil
}

// GetEmail fetches one message in full (GET /me/emails/:id). Opening a message
// marks it read, unlike listing with bodies — reach for the listing when sweeping
// a backlog, and for this when a human asked to read one thing.
func (c *Client) GetEmail(ctx context.Context, id int64) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/emails/"+strconv.FormatInt(id, 10), nil)
	return env.Data, err
}

// IngestMessage is one message the caller's own mail client fetched. ExternalID is
// the deduplication key — the message's Message-ID header in practice — and is the
// only field the API requires: re-pushing the same id updates that message rather
// than storing a second copy.
type IngestMessage struct {
	ExternalID string `json:"external_id"`
	ThreadID   string `json:"thread_id,omitempty"`
	FromAddr   string `json:"from_addr,omitempty"`
	FromName   string `json:"from_name,omitempty"`
	Subject    string `json:"subject,omitempty"`
	BodyText   string `json:"body_text,omitempty"`
	BodyHTML   string `json:"body_html,omitempty"`
	ReceivedAt string `json:"received_at,omitempty"`
}

// IngestResult reports how a pushed batch landed, so a syncing agent can tell new
// mail from a re-run of the same window.
type IngestResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
}

// PushEmails uploads a batch of messages the caller fetched themselves
// (POST /me/emails). The server stores them under source 'external' and never
// classifies them — that is the caller's agent's job.
func (c *Client) PushEmails(ctx context.Context, msgs []IngestMessage) (IngestResult, error) {
	body, err := json.Marshal(struct {
		Messages []IngestMessage `json:"messages"`
	}{Messages: msgs})
	if err != nil {
		return IngestResult{}, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/me/emails", bytes.NewReader(body))
	if err != nil {
		return IngestResult{}, err
	}
	var out IngestResult
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return IngestResult{}, fmt.Errorf("decode ingest result: %w", err)
	}
	return out, nil
}

// TriageParams is an agent's verdict for one message. Slug is optional: omitting
// it classifies without touching the link, and clearing a link stays the separate
// `unlink` action. Confidence is optional and gates nothing.
type TriageParams struct {
	Signal     string
	Slug       string
	Confidence *float64
}

// Triage records an agent's verdict for one message (POST /me/emails/:id/triage):
// what the message is, and optionally which application it belongs to. The server
// writes the classification and the link together and advances the application's
// stage when the signal implies forward progress.
func (c *Client) Triage(ctx context.Context, id int64, p TriageParams) (json.RawMessage, error) {
	body, err := json.Marshal(struct {
		Signal     string   `json:"signal"`
		Slug       string   `json:"slug,omitempty"`
		Confidence *float64 `json:"confidence,omitempty"`
	}{Signal: p.Signal, Slug: p.Slug, Confidence: p.Confidence})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, emailPath(id, "triage"), bytes.NewReader(body))
	return env.Data, err
}

// LinkEmail attaches a message to one of the caller's applications
// (POST /me/emails/:id/link).
func (c *Client) LinkEmail(ctx context.Context, id int64, slug string) (json.RawMessage, error) {
	body, err := json.Marshal(struct {
		Slug string `json:"slug"`
	}{Slug: slug})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, emailPath(id, "link"), bytes.NewReader(body))
	return env.Data, err
}

// UnlinkEmail clears a message's application link, leaving its classification
// intact (POST /me/emails/:id/unlink).
func (c *Client) UnlinkEmail(ctx context.Context, id int64) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodPost, emailPath(id, "unlink"), nil)
	return env.Data, err
}

// DeleteEmail soft-deletes a message: it leaves the listing but is retained and
// can be restored (POST /me/emails/:id/delete).
func (c *Client) DeleteEmail(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodPost, emailPath(id, "delete"), nil)
	return err
}

// RestoreEmail undoes a soft-delete (POST /me/emails/:id/restore).
func (c *Client) RestoreEmail(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodPost, emailPath(id, "restore"), nil)
	return err
}

// MarkAllRead marks every unread message matching the given filters as read and
// returns how many it marked (POST /me/inbox/read-all). The unread filter is
// implicit server-side, so it is not sent.
func (c *Client) MarkAllRead(ctx context.Context, p InboxParams) (int, error) {
	q := url.Values{}
	if p.Source != "" {
		q.Set("source", p.Source)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if p.Query != "" {
		q.Set("q", p.Query)
	}
	path := "/api/v1/me/inbox/read-all"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	env, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		Marked int `json:"marked"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return 0, fmt.Errorf("decode marked count: %w", err)
	}
	return out.Marked, nil
}

// emailPath builds a per-message action path.
func emailPath(id int64, action string) string {
	return "/api/v1/me/emails/" + strconv.FormatInt(id, 10) + "/" + action
}
