package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// CV-tailoring endpoints (beta-gated on the server). These act as the authenticated
// user — the agent drives them with its minted session key.

// ListCVs returns the caller's tailored CVs, newest edit first, each with the vacancy it
// was written for (GET /me/cvs). This is where a CV id comes from: every other command in
// this group is addressed by one, and the id is opaque, so it has to be read rather than
// constructed.
func (c *Client) ListCVs(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/cvs", nil)
	return env.Data, err
}

// TailorCV starts (or reopens) tailoring for a vacancy and returns the tailored CV's id, the
// base it was copied from, and the bound agent session (POST /me/cvs/tailor). Idempotent per
// vacancy: calling it again for the same slug returns the copy that already exists rather
// than making a second one.
//
// It debits the caller's AI credits the first time it creates the copy — a 402 comes back
// when the balance will not cover it — and 409s when there is no résumé to seed a base CV
// from. It never calls the LLM itself.
func (c *Client) TailorCV(ctx context.Context, jobSlug string) (json.RawMessage, error) {
	body, err := json.Marshal(struct {
		JobSlug string `json:"job_slug"`
	}{JobSlug: jobSlug})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/me/cvs/tailor", bytes.NewReader(body))
	return env.Data, err
}

// TailorCVContext returns the cached fit-analysis context for a tailored CV: the
// verdict, recommendation, dimension comments, and the missing-have / missing-gap
// requirement split the honest wall turns on (GET /me/cvs/:id/tailor-context).
func (c *Client) TailorCVContext(ctx context.Context, cvID string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, cvPath(cvID)+"/tailor-context", nil)
	return env.Data, err
}

// GetCV fetches a CV with its full document (GET /me/cvs/:id).
func (c *Client) GetCV(ctx context.Context, cvID string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, cvPath(cvID), nil)
	return env.Data, err
}

// EditCV applies a batch of path operations to a CV (PATCH /me/cvs/:id). body is the raw
// request JSON — `{"ops":[…]}`, each operation a kind (set/insert/remove/move) and a path
// into the document. The server validates and sanitizes, so a malformed batch comes back as
// a 422 APIError and nothing is applied.
//
// A key edits as the tailoring agent, which the server holds to the agent's rules: the
// candidate's own header fields are refused, and an operation stating something about them
// needs the `evidence_id` of a banked achievement.
func (c *Client) EditCV(ctx context.Context, cvID string, body json.RawMessage) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodPatch, cvPath(cvID), bytes.NewReader(body))
	return env.Data, err
}

// RenderCV downloads a CV rendered to PDF (GET /me/cvs/:id/pdf). Unlike the other
// endpoints this returns raw PDF bytes, not the JSON envelope, so it bypasses do().
func (c *Client) RenderCV(ctx context.Context, cvID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+cvPath(cvID)+"/pdf", nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var env envelope
		_ = json.Unmarshal(b, &env) // best-effort: a non-JSON error body just leaves Message empty
		return nil, &APIError{Status: resp.StatusCode, Message: env.Error}
	}
	return b, nil
}

func cvPath(cvID string) string {
	return "/api/v1/me/cvs/" + url.PathEscape(cvID)
}
