package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchTokenSendsTheApiKeyAndReturnsTheToken(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"token":"jwt-here","expires_in":43200,"user_id":"1"}`))
	}))
	defer srv.Close()

	tok, err := FetchToken(context.Background(), srv.URL, "fhk_abc")
	if err != nil {
		t.Fatalf("FetchToken: %v", err)
	}
	if tok != "jwt-here" {
		t.Fatalf("token = %q", tok)
	}
	if gotAuth != "Bearer fhk_abc" {
		t.Fatalf("api key must travel as a bearer token, got %q", gotAuth)
	}
	if gotPath != "/auth/runner-token" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestFetchTokenSurfacesTheServersReason(t *testing.T) {
	// "no assistant account for this key" has to reach the user; a bare 403
	// leaves them guessing.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"no assistant account for this key"}`))
	}))
	defer srv.Close()

	_, err := FetchToken(context.Background(), srv.URL, "fhk_abc")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "no assistant account") {
		t.Fatalf("the server's reason must survive, got: %v", err)
	}
}

func TestFetchTokenRefusesAnEmptyKey(t *testing.T) {
	_, err := FetchToken(context.Background(), "https://example.invalid", "  ")
	if err == nil || !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("should point at `freehire auth login`, got: %v", err)
	}
}
