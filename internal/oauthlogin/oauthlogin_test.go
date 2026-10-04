package oauthlogin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeOAuthServer answers dynamic registration and the token exchange exactly
// as freehire's backend does, checking the PKCE verifier against whatever
// challenge rode the authorize URL the browser stub below was given. It does
// not implement /authorize — the stub "visits" the authorize screen by going
// straight to the local callback instead, exactly what a real browser does
// after a real authorize screen's "Allow" click redirects it.
func fakeOAuthServer(t *testing.T, challenge *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/oauth/register", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "fhcl_test123"})
	})
	mux.HandleFunc("/api/v1/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		verifier := r.PostForm.Get("code_verifier")
		if *challenge != "" && challengeFor(verifier) != *challenge {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "code_verifier does not match code_challenge"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fhm_test-access-token"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// browserStub stands in for a human completing the real authorize screen: it
// reads redirect_uri, state, and code_challenge off the URL Login() would
// have opened, records the challenge so fakeOAuthServer can check the later
// verifier, and drives the local callback with a fixed fake code.
func browserStub(challengeOut *string) func(string) error {
	return func(rawURL string) error {
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		q := u.Query()
		*challengeOut = q.Get("code_challenge")
		cbURL := q.Get("redirect_uri") + "?code=the-fake-code&state=" + q.Get("state")
		go func() { _, _ = http.Get(cbURL) }()
		return nil
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestLogin_FullRoundTrip(t *testing.T) {
	var challenge string
	srv := fakeOAuthServer(t, &challenge)

	token, err := Login(context.Background(), srv.Client(), srv.URL, browserStub(&challenge), discardWriter{})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token != "fhm_test-access-token" {
		t.Errorf("token = %q, want fhm_test-access-token", token)
	}
}

func TestLogin_RejectsAWrongState(t *testing.T) {
	var challenge string
	srv := fakeOAuthServer(t, &challenge)

	// A browser that redirects with someone else's state — a CSRF attempt
	// against the local callback.
	openBrowser := func(rawURL string) error {
		u, _ := url.Parse(rawURL)
		redirectURI := u.Query().Get("redirect_uri")
		go func() { _, _ = http.Get(redirectURI + "?code=the-fake-code&state=not-the-real-state") }()
		return nil
	}

	if _, err := Login(context.Background(), srv.Client(), srv.URL, openBrowser, discardWriter{}); err == nil {
		t.Error("expected an error for a mismatched state")
	}
}

func TestLogin_ReportsAccessDenied(t *testing.T) {
	var challenge string
	srv := fakeOAuthServer(t, &challenge)

	openBrowser := func(rawURL string) error {
		u, _ := url.Parse(rawURL)
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")
		go func() { _, _ = http.Get(redirectURI + "?error=access_denied&state=" + state) }()
		return nil
	}

	_, err := Login(context.Background(), srv.Client(), srv.URL, openBrowser, discardWriter{})
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("err = %v, want it to mention access_denied", err)
	}
}
