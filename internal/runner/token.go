package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FetchToken exchanges the user's freehire API key for a short-lived session
// token for the assistant server.
//
// The user should never have to know their own account id. They already
// authenticated once with `freehire auth login`; the server asks freehire who
// that key belongs to and mints a token for exactly that account. Anything
// else would put an internal identifier in the user's hands and let a typo
// route their sessions to someone else's machine.
func FetchToken(ctx context.Context, serverURL, apiKey string) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("not authenticated: run `freehire auth login`")
	}
	endpoint := strings.TrimRight(serverURL, "/") + "/auth/runner-token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("asking %s for a token: %w", serverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error != "" {
			return "", fmt.Errorf("%s (%s)", e.Error, resp.Status)
		}
		return "", fmt.Errorf("token request failed: %s", resp.Status)
	}

	var out struct {
		Token  string `json:"token"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decoding the token response: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("the server returned an empty token")
	}
	return out.Token, nil
}
