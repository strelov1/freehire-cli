package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusShowsOperationalSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"overall":"operational","generated_at":"2026-08-18T12:00:00Z","last_job_added_at":"2026-08-18T11:55:00Z","providers":[{"provider":"greenhouse","status":"operational","total_boards":2,"healthy_boards":2}]}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "")

	out, err := run(t, "status", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{"Status: operational", "Generated at: 2026-08-18T12:00:00Z", "Last job added: 2026-08-18T11:55:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output = %q, want it to contain %q", out, want)
		}
	}
	if strings.Contains(out, "Problem providers") {
		t.Errorf("an all-operational fleet should not list problem providers: %q", out)
	}
}

func TestStatusListsProblemProvidersAndExitsNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"overall":"degraded","generated_at":"2026-08-18T12:00:00Z","last_job_added_at":null,"providers":[{"provider":"lever","status":"degraded","total_boards":10,"healthy_boards":4},{"provider":"greenhouse","status":"operational","total_boards":2,"healthy_boards":2}]}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "")

	out, err := run(t, "status", "--api-url", srv.URL)
	if err == nil {
		t.Error("a degraded fleet should exit non-zero, so a cron/monitoring caller can detect it")
	}
	if !strings.Contains(out, "Last job added: never") {
		t.Errorf("a null last_job_added_at should render as never: %q", out)
	}
	if !strings.Contains(out, "lever: degraded (4/10 healthy)") {
		t.Errorf("status output = %q, want the degraded provider listed", out)
	}
	if strings.Contains(out, "greenhouse:") {
		t.Errorf("an operational provider should not be listed as a problem: %q", out)
	}
}

func TestStatusJSONPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"overall":"operational","generated_at":"2026-08-18T12:00:00Z","last_job_added_at":"2026-08-18T11:55:00Z","providers":[]}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "")

	out, err := run(t, "--json", "status", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var s struct {
		Overall string `json:"overall"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &s); err != nil {
		t.Fatalf("--json output is not JSON: %v (%q)", err, out)
	}
	if s.Overall != "operational" {
		t.Errorf("overall = %q", s.Overall)
	}
}

func TestStatusRunsAnonymously(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.Write([]byte(`{"data":{"overall":"operational","generated_at":"2026-08-18T12:00:00Z","last_job_added_at":null,"providers":[]}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "")

	if _, err := run(t, "status", "--api-url", srv.URL); err != nil {
		t.Fatalf("anonymous status should not error: %v", err)
	}
	if hadAuth {
		t.Error("status must not send an Authorization header when no token is configured")
	}
}
