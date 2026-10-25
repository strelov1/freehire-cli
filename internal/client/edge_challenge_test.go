package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// challengingEdge mimics Cloudflare challenging a VPN caller: every /api/v1 path
// outside /api/v1/me/ answers 403 with cf-mitigated: challenge, and the signed-in
// door (/api/v1/me/api/...) is served — for a good bearer only — as the path behind it.
func challengingEdge(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Method+" "+r.URL.RequestURI())
		if !strings.HasPrefix(r.URL.Path, "/api/v1/me/") {
			w.Header().Set("Cf-Mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("<html>Just a moment...</html>"))
			return
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"not authenticated"}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		out, _ := json.Marshal(map[string]any{"data": map[string]string{
			"path": r.URL.Path, "q": r.URL.Query().Get("q"), "body": string(body),
		}})
		w.Write(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDo_SignedInRetriesAChallengeThroughTheDoor(t *testing.T) {
	var seen []string
	srv := challengingEdge(t, &seen)
	c := New(srv.URL, "good", srv.Client())

	env, err := c.do(context.Background(), http.MethodGet, "/api/v1/agent/jobs/search?q=go", nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if got := string(env.Data); !strings.Contains(got, `"path":"/api/v1/me/api/agent/jobs/search"`) || !strings.Contains(got, `"q":"go"`) {
		t.Errorf("data = %s, want the door path with the query intact", got)
	}
	want := []string{"GET /api/v1/agent/jobs/search?q=go", "GET /api/v1/me/api/agent/jobs/search?q=go"}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Errorf("requests = %v, want %v", seen, want)
	}
}

func TestDo_RetryResendsTheBody(t *testing.T) {
	var seen []string
	srv := challengingEdge(t, &seen)
	c := New(srv.URL, "good", srv.Client())

	env, err := c.do(context.Background(), http.MethodPost, "/api/v1/jobs/go-dev/save", strings.NewReader(`{"a":1}`))
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !strings.Contains(string(env.Data), `"body":"{\"a\":1}"`) {
		t.Errorf("data = %s, want the body re-sent on the retry", env.Data)
	}
}

func TestDo_ChallengeWithoutCredentialSaysWhy(t *testing.T) {
	var seen []string
	srv := challengingEdge(t, &seen)
	c := New(srv.URL, "", srv.Client())

	_, err := c.do(context.Background(), http.MethodGet, "/api/v1/agent/jobs/search?q=go", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
		t.Fatalf("err = %v, want a 403 APIError", err)
	}
	if !strings.Contains(apiErr.Message, "freehire auth login") {
		t.Errorf("message = %q, want it to point at signing in", apiErr.Message)
	}
	if len(seen) != 1 {
		t.Errorf("requests = %v, want no retry without a credential", seen)
	}
}

// A /me/ path is already the door's side of the edge; a challenge there is not
// something another address can fix, so it is reported, not retried in a loop.
func TestDo_ChallengeOnAMePathIsNotRetried(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	if _, err := c.do(context.Background(), http.MethodGet, "/api/v1/me/tracking", nil); err == nil {
		t.Fatal("do: want an error")
	}
	if len(seen) != 1 {
		t.Errorf("requests = %v, want exactly one", seen)
	}
}

func TestDo_SendsItsOwnUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.UserAgent()
		w.Write([]byte(`{"data":null}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := New(srv.URL, "", srv.Client()).do(context.Background(), http.MethodGet, "/api/v1/status", nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if !strings.HasPrefix(ua, "freehire-cli/") {
		t.Errorf("User-Agent = %q, want freehire-cli/<version>", ua)
	}
}
