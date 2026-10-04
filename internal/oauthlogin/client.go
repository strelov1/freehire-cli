package oauthlogin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// clientName is this CLI's display name on the consent screen and in a
// user's "Connected devices" list.
const clientName = "freehire CLI"

// registerClient performs RFC 7591 dynamic client registration against
// baseURL, returning the server-assigned client_id. Called once per login —
// re-registering on every run costs one request and needs no cache file; the
// server does not require (or benefit from) client_id reuse.
func registerClient(ctx context.Context, hc *http.Client, baseURL, redirectURI string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"client_name":   clientName,
		"redirect_uris": []string{redirectURI},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/oauth/register", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return "", apiError(resp)
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ClientID, nil
}

// exchangeCode redeems a single-use authorization code for a bearer access
// token via the PKCE token endpoint, returning the plaintext token.
func exchangeCode(ctx context.Context, hc *http.Client, baseURL, clientID, redirectURI, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", apiError(resp)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.AccessToken, nil
}

// apiError renders a non-2xx response as a Go error, reading the `error`
// field freehire's OAuth endpoints answer with when one is present.
func apiError(resp *http.Response) error {
	var body struct {
		Error string `json:"error"`
	}
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &body)
	if body.Error != "" {
		return fmt.Errorf("%s: %s", resp.Status, body.Error)
	}
	return fmt.Errorf("%s", resp.Status)
}
