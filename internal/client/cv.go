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
