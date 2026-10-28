package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSimilar_ListsNeighboursWithLimit(t *testing.T) {
	var gotPath, gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotLimit = r.URL.Path, r.URL.Query().Get("limit")
		w.Write([]byte(`{"data":[{"public_slug":"rust-dev-x1","title":"Rust Dev","company":"Acme","location":"Remote"}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FREEHIRE_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	out, err := run(t, "similar", "go-dev", "--limit", "3")
	if err != nil {
		t.Fatalf("similar: %v", err)
	}
	if gotPath != "/api/v1/jobs/go-dev/similar" || gotLimit != "3" {
		t.Errorf("request = %s limit=%s, want /api/v1/jobs/go-dev/similar limit=3", gotPath, gotLimit)
	}
	if !strings.Contains(out, "rust-dev-x1") {
		t.Errorf("out = %q, want the neighbour's slug", out)
	}
}

func TestSimilar_SaysWhyItIsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FREEHIRE_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	out, err := run(t, "similar", "brand-new")
	if err != nil {
		t.Fatalf("similar: %v", err)
	}
	if !strings.Contains(out, "no similar postings yet") {
		t.Errorf("out = %q, want the empty-state explanation", out)
	}
}

func TestJob_SaysWhenThePostingIsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"public_slug":"go-dev","title":"Go Dev","company":"Acme","closed_at":"2026-10-01T12:00:00Z","description":"<p>hi</p>"}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FREEHIRE_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_API_URL", srv.URL)

	out, err := run(t, "job", "go-dev")
	if err != nil {
		t.Fatalf("job: %v", err)
	}
	if !strings.Contains(out, "CLOSED on 2026-10-01") {
		t.Errorf("out = %q, want the closed line", out)
	}
}
