package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/strelov1/freehire-cli/internal/config"
)

// fakeOAuthAPI mimics the freehire backend's OAuth + /auth/me surface: dynamic
// registration, the PKCE token exchange (any code/verifier pair is accepted —
// the CLI's own oauthlogin package already covers the real PKCE check), and the
// identity read the login command validates with afterward.
func fakeOAuthAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/oauth/register", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "fhcl_test123"})
	})
	mux.HandleFunc("/api/v1/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fhm_test-access-token"})
	})
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fhm_test-access-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 7, "email": "oauth-user@example.test"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthLoginOAuthValidatesAndWritesCreds(t *testing.T) {
	srv := fakeOAuthAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "")
	t.Setenv("FREEHIRE_API_URL", "")

	// Stand in for a human: "visit" the authorize URL by driving the local
	// callback directly, the same way a real browser does once the real
	// authorize screen's "Allow" click redirects it.
	original := openBrowser
	openBrowser = func(rawURL string) error {
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		q := u.Query()
		cbURL := q.Get("redirect_uri") + "?code=the-fake-code&state=" + q.Get("state")
		go func() { _, _ = http.Get(cbURL) }()
		return nil
	}
	t.Cleanup(func() { openBrowser = original })

	out, err := run(t, "auth", "login", "--oauth", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out, "oauth-user@example.test") {
		t.Errorf("login output = %q, want it to show the email", out)
	}
	got, _ := config.Load()
	if got.Token != "fhm_test-access-token" || got.APIURL != srv.URL {
		t.Errorf("stored creds = %+v", got)
	}
}
