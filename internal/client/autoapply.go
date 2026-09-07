package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Auto-apply review endpoints (openspec/changes/auto-apply-review-tracking). Starting a
// NEW attempt (POST /jobs/:slug/auto-apply) is cookie-only on the server, deliberately: a
// fresh attempt can end in a real submitted application, and the browser is the only
// place a candidate can watch it happen and undo it — so it has no client method here and
// never will. What IS reachable by a full-scope API key is everything downstream of an
// attempt already queued from the website: its status, tailoring it, and reviewing it.

// AutoApplyStatus returns a tracked job's application detail, including the caller's live
// auto-apply attempt when one exists (GET /me/tracking/:slug). The embedded `auto_apply`
// field carries a richer six-value status (tailoring/pending_review/approved/blocked/
// declined/failed) than the job-detail overlay's own three-value one, plus the queue id
// TailorAutoApplyQueueEntry and ReviewAutoApplyQueueEntry take.
func (c *Client) AutoApplyStatus(ctx context.Context, jobSlug string) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodGet, "/api/v1/me/tracking/"+url.PathEscape(jobSlug), nil)
	return env.Data, err
}

// TailorAutoApplyQueueEntry starts (or continues) tailoring the CV for an already-queued
// auto-apply attempt (POST /me/auto-apply/:queueId/tailor). queueID comes from
// AutoApplyStatus's embedded auto_apply.queue_id.
func (c *Client) TailorAutoApplyQueueEntry(ctx context.Context, queueID int64) (json.RawMessage, error) {
	env, err := c.do(ctx, http.MethodPost, autoApplyQueuePath(queueID)+"/tailor", nil)
	return env.Data, err
}

// ReviewAutoApplyQueueEntry records the candidate's approve/decline decision for a
// tailored, not-yet-reviewed queue entry (POST /me/auto-apply/:queueId/review). decision
// must be "approved" or "declined" — the server validates it either way, and a decision
// already recorded comes back as a 409 APIError.
func (c *Client) ReviewAutoApplyQueueEntry(ctx context.Context, queueID int64, decision string) (json.RawMessage, error) {
	body, err := json.Marshal(struct {
		Decision string `json:"decision"`
	}{Decision: decision})
	if err != nil {
		return nil, err
	}
	env, err := c.do(ctx, http.MethodPost, autoApplyQueuePath(queueID)+"/review", bytes.NewReader(body))
	return env.Data, err
}

func autoApplyQueuePath(queueID int64) string {
	return "/api/v1/me/auto-apply/" + strconv.FormatInt(queueID, 10)
}
