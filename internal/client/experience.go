package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// Experience-bank endpoints. GET reads the caller's whole bank (cookie or full-scope key,
// same as everywhere else this CLI authenticates); the two POSTs create rather than
// correct — PUT/DELETE on an existing entry are cookie-only on the server and stay out of
// reach of a key, on purpose.

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
