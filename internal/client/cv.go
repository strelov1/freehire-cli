package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// CV-tailoring endpoints (beta-gated on the server). These act as the authenticated
// user — the agent drives them with its minted session key.

// TailorCVContext returns the cached fit-analysis context for a tailored CV: the
// verdict, recommendation, dimension comments, and the missing-have / missing-gap
// requirement split the honest wall turns on (GET /me/cvs/:id/tailor-context).
func (c *Client) TailorCVContext(ctx context.Context, cvID int64) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, cvPath(cvID)+"/tailor-context", nil)
	return env.Data, err
}

// GetCV fetches a CV with its full document (GET /me/cvs/:id).
func (c *Client) GetCV(ctx context.Context, cvID int64) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, cvPath(cvID), nil)
	return env.Data, err
}

// PatchCV applies one field-level patch to a CV (PATCH /me/cvs/:id). patch is the raw
// cv.Patch JSON (op + address + payload); the server sanitizes and validates it, so a
// malformed patch comes back as a 422 APIError.
func (c *Client) PatchCV(ctx context.Context, cvID int64, patch json.RawMessage) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodPatch, cvPath(cvID), bytes.NewReader(patch))
	return env.Data, err
}

// RenderCV downloads a CV rendered to PDF (GET /me/cvs/:id/pdf). Unlike the other
// endpoints this returns raw PDF bytes, not the JSON envelope, so it bypasses do().
func (c *Client) RenderCV(ctx context.Context, cvID int64) ([]byte, error) {
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

func cvPath(cvID int64) string {
	return "/api/v1/me/cvs/" + strconv.FormatInt(cvID, 10)
}
