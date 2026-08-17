package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Experience-bank endpoints. GET reads the caller's whole bank, the POSTs create, and the
// PUTs correct — all reachable with a full-scope key, same as everywhere else this CLI
// authenticates.
//
// Removing is here too, minus the cascade: the server refuses a keyed DELETE of an
// employment that still holds achievements (409), because that delete would take them with
// it and the bank has no undo. Move them first, then remove the empty place. The atom merge
// stays cookie-only.
//
// A keyed correction cannot move a claim's provenance — the server keeps whatever the row
// already carried, so an agent cannot promote its own inference into something citable on
// a CV. Only the candidate's own browser edit stamps `manual`.

// ListExperience returns the caller's whole bank, grouped by employment
// (GET /me/experience).
func (c *Client) ListExperience(ctx context.Context) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/experience", nil)
	return env.Data, err
}

// CreateEmploymentParams is the body for recording a new place — a job or a project.
type CreateEmploymentParams struct {
	Kind     string   `json:"kind"`
	Company  string   `json:"company,omitempty"`
	Role     string   `json:"role,omitempty"`
	Location string   `json:"location,omitempty"`
	Start    string   `json:"start,omitempty"`
	End      string   `json:"end,omitempty"`
	Current  bool     `json:"current,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Stack    []string `json:"stack,omitempty"`
	// Link is not settable by any command; it rides along on an update so that correcting
	// a company name does not silently drop a project's URL, since the server replaces the
	// whole row.
	Link string `json:"link,omitempty"`
}

// CreateEmployment records a new employment under the caller (POST /me/experience/employments).
func (c *Client) CreateEmployment(ctx context.Context, p CreateEmploymentParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/me/experience/employments", bytes.NewReader(body))
	return env.Data, err
}

// CreateAtomParams is the body for recording a new achievement. Provenance is not a
// field here: the server always stamps a POST-created atom `manual`, regardless of what
// is sent, because there is no chat transcript behind a plain HTTP call to check a
// stronger claim against.
type CreateAtomParams struct {
	Claim        string   `json:"claim"`
	Context      string   `json:"context,omitempty"`
	Metrics      []string `json:"metrics,omitempty"`
	Skills       []string `json:"skills,omitempty"`
	EmploymentID string   `json:"employment_id,omitempty"`
}

// CreateAtom records a new achievement under the caller (POST /me/experience/atoms). A
// claim already in the bank, under any spelling, comes back as an APIError with status 409
// rather than a duplicate row.
func (c *Client) CreateAtom(ctx context.Context, p CreateAtomParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, "/api/v1/me/experience/atoms", bytes.NewReader(body))
	return env.Data, err
}

// UpdateAtom replaces an owned achievement (PUT /me/experience/atoms/:id).
//
// It is a REPLACE, not a patch: every field the caller omits is written as empty. Callers
// must send the whole atom — see the update commands, which read the bank first and change
// only what was named.
func (c *Client) UpdateAtom(ctx context.Context, id string, p CreateAtomParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPut, "/api/v1/me/experience/atoms/"+url.PathEscape(id), bytes.NewReader(body))
	return env.Data, err
}

// DeleteAtom removes one owned achievement (DELETE /me/experience/atoms/:id). It takes
// nothing else with it — the row named is the only row removed — and there is no undo.
func (c *Client) DeleteAtom(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/api/v1/me/experience/atoms/"+url.PathEscape(id), nil)
	return err
}

// DeleteEmployment removes one owned place (DELETE /me/experience/employments/:id).
//
// The server refuses this with 409 while achievements still hang off the place, because the
// row's foreign key cascades and would delete them too. That refusal is the API's, not this
// client's: move the achievements with UpdateAtom first, then remove the empty place.
func (c *Client) DeleteEmployment(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/api/v1/me/experience/employments/"+url.PathEscape(id), nil)
	return err
}

// UpdateEmployment replaces an owned employment (PUT /me/experience/employments/:id), with
// the same whole-row replace semantics as UpdateAtom.
func (c *Client) UpdateEmployment(ctx context.Context, id string, p CreateEmploymentParams) (json.RawMessage, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPut, "/api/v1/me/experience/employments/"+url.PathEscape(id), bytes.NewReader(body))
	return env.Data, err
}
