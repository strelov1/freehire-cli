package oauthlogin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// loginTimeout bounds how long Login waits for the browser round trip —
// long enough for a human to actually look at the consent screen, short
// enough that an abandoned terminal does not hang forever.
const loginTimeout = 5 * time.Minute

// Login runs the full OAuth 2.1 + PKCE authorization-code flow against
// baseURL and returns the resulting bearer access token: register a public
// client, open openBrowser on the authorize URL, wait for the local
// loopback callback, then exchange the code for a token. out receives the
// "open this URL" fallback line, for a terminal where openBrowser could not
// actually launch anything.
func Login(ctx context.Context, hc *http.Client, baseURL string, openBrowser func(string) error, out io.Writer) (string, error) {
	cb, err := listenForCallback()
	if err != nil {
		return "", fmt.Errorf("start the local callback listener: %w", err)
	}
	defer func() { _ = cb.Close() }()

	clientID, err := registerClient(ctx, hc, baseURL, cb.redirectURI)
	if err != nil {
		return "", fmt.Errorf("register this CLI as an OAuth client: %w", err)
	}

	verifier, err := generateVerifier()
	if err != nil {
		return "", err
	}
	state, err := generateState()
	if err != nil {
		return "", err
	}
	authorizeURL := baseURL + "/api/v1/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {cb.redirectURI},
		"state":                 {state},
		"code_challenge":        {challengeFor(verifier)},
		"code_challenge_method": {"S256"},
	}.Encode()

	_, _ = fmt.Fprintf(out, "Opening your browser to sign in. If it does not open, visit:\n  %s\n", authorizeURL)
	if err := openBrowser(authorizeURL); err != nil {
		_, _ = fmt.Fprintf(out, "Could not open a browser automatically (%v) — visit the URL above.\n", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()
	result, err := cb.wait(waitCtx)
	if err != nil {
		return "", err
	}
	if result.state != state {
		return "", fmt.Errorf("the sign-in callback carried an unexpected state — refusing it")
	}
	if result.errorParam != "" {
		return "", fmt.Errorf("sign-in was not completed: %s", result.errorParam)
	}

	token, err := exchangeCode(ctx, hc, baseURL, clientID, cb.redirectURI, result.code, verifier)
	if err != nil {
		return "", fmt.Errorf("exchange the authorization code for a token: %w", err)
	}
	return token, nil
}
