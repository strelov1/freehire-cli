package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContributeSendsTheLinkAndTagsTheSurface(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"public_slug":"senior-go-acme","status":"imported"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "contribute", "https://acme.recruitee.com/o/senior-go", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("contribute: %v", err)
	}
	if gotBody["url"] != "https://acme.recruitee.com/o/senior-go" {
		t.Errorf("body url = %v, want the link", gotBody["url"])
	}
	// Attribution: the server records which door an intake came through, so abuse of an
	// endpoint that makes it fetch arbitrary URLs is visible per channel, not just per user.
	if gotBody["surface"] != "cli" {
		t.Errorf("surface = %v, want cli", gotBody["surface"])
	}
	if !strings.Contains(out, "new to us") || !strings.Contains(out, "/jobs/senior-go-acme") {
		t.Errorf("output = %q, want the imported message and a link to the posting", out)
	}
}

func TestContributeMessagePerOutcome(t *testing.T) {
	slug := "senior-go-acme"
	cases := []struct {
		name        string
		in          resolvedLink
		wantContain []string
	}{
		{
			"already carried",
			resolvedLink{Status: "found", PublicSlug: &slug},
			[]string{"Already in the catalogue", "/jobs/senior-go-acme"},
		},
		{
			"company already crawled",
			resolvedLink{Status: "tracked", PublicSlug: &slug, CompanySlug: "acme"},
			[]string{"already crawl", "/jobs/senior-go-acme", "/companies/acme"},
		},
		{
			"new company",
			resolvedLink{Status: "imported", PublicSlug: &slug},
			[]string{"new to us", "AI credit"},
		},
		{
			// No slug: there is nothing to link to, and the line must not dangle a bare host.
			"unreadable page",
			resolvedLink{Status: "queued"},
			[]string{"couldn't read", "not credited"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := contributeMessage(c.in, "https://freehire.me")
			for _, want := range c.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("message = %q, want it to mention %q", got, want)
				}
			}
			if c.in.PublicSlug == nil && strings.Contains(got, "https://freehire.me") {
				t.Errorf("message = %q, want no link when there is no posting", got)
			}
		})
	}
}

func TestContributionsListsMine(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/contributions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte(`{"data":[
			{"url":"https://jobs.ashbyhq.com/blitzy","source":"ashby","board":"blitzy","status":"pending","surface":"cli"},
			{"url":"https://example.com/careers/1","source":"","board":"","status":"review","surface":"web"}
		]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "contributions", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("contributions: %v", err)
	}
	if !strings.Contains(out, "blitzy (ashby)") {
		t.Errorf("output = %q, want the recorded board", out)
	}
	// A review row has no board, so it must be shown by its URL rather than as a blank entry.
	if !strings.Contains(out, "https://example.com/careers/1") || !strings.Contains(out, "under review") {
		t.Errorf("output = %q, want the review row shown by its URL", out)
	}
}

func TestContributionsJSONPassesThrough(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/contributions", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"board":"blitzy","source":"ashby","status":"pending","surface":"cli"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "contributions", "--json", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("contributions --json: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("--json output is not JSON: %v (%q)", err, out)
	}
	if len(rows) != 1 || rows[0]["board"] != "blitzy" {
		t.Errorf("rows = %v, want the raw API data", rows)
	}
}
