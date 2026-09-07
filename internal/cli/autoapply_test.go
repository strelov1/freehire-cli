package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// autoApplyFakeAPI mimics the auto-apply review endpoints behind a Bearer=good gate.
func autoApplyFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/tracking/go-dev-acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"job":{"public_slug":"go-dev-acme"},"auto_apply":{
			"status":"pending_review","queue_id":42,
			"resolved_preview":{"fields":[{"label":"Email","value":"a@b.com"},{"label":"Phone","value":"+1 555 0100"}]}
		}}}`))
	})
	mux.HandleFunc("/api/v1/me/tracking/blocked-job", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"job":{"public_slug":"blocked-job"},"auto_apply":{
			"status":"blocked","queue_id":43,
			"unmapped":[{"label":"Current State of Residence","required":true,"reason":"no known answer source"}]
		}}}`))
	})
	mux.HandleFunc("/api/v1/me/tracking/no-attempt", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"job":{"public_slug":"no-attempt"},"auto_apply":null}}`))
	})
	mux.HandleFunc("/api/v1/me/auto-apply/42/tailor", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("tailor method = %s, want POST", r.Method)
		}
		w.Write([]byte(`{"data":{"session_id":"s1"}}`))
	})
	mux.HandleFunc("/api/v1/me/auto-apply/42/review", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"decision":"approved"`) {
			w.Write([]byte(`{"data":{"decision":"approved"}}`))
			return
		}
		if strings.Contains(string(b), `"decision":"declined"`) {
			w.Write([]byte(`{"data":{"decision":"declined"}}`))
			return
		}
		t.Errorf("unexpected review body: %s", b)
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func autoApplyEnv(t *testing.T, srv *httptest.Server) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")
	t.Setenv("FREEHIRE_API_URL", srv.URL)
}

func TestAutoApplyStatus_pendingReview(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	out, err := run(t, "auto-apply", "status", "go-dev-acme")
	if err != nil {
		t.Fatalf("auto-apply status: %v", err)
	}
	for _, want := range []string{"pending_review", "42", "Email", "a@b.com", "auto-apply review 42"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output %q is missing %q", out, want)
		}
	}
}

func TestAutoApplyStatus_blocked(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	out, err := run(t, "auto-apply", "status", "blocked-job")
	if err != nil {
		t.Fatalf("auto-apply status: %v", err)
	}
	for _, want := range []string{"blocked", "Current State of Residence", "no known answer source"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output %q is missing %q", out, want)
		}
	}
}

func TestAutoApplyStatus_none(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	out, err := run(t, "auto-apply", "status", "no-attempt")
	if err != nil {
		t.Fatalf("auto-apply status: %v", err)
	}
	if !strings.Contains(out, "No auto-apply attempt") {
		t.Errorf("status output %q should say there is no attempt", out)
	}
}

func TestAutoApplyStatus_JSONPassesThrough(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	out, err := run(t, "auto-apply", "status", "go-dev-acme", "--json")
	if err != nil {
		t.Fatalf("auto-apply status --json: %v", err)
	}
	if !strings.Contains(out, `"resolved_preview"`) {
		t.Errorf("--json should print the whole payload, got %q", out)
	}
}

func TestAutoApplyTailor(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	out, err := run(t, "auto-apply", "tailor", "42")
	if err != nil {
		t.Fatalf("auto-apply tailor: %v", err)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("tailor output %q does not mention the queue id", out)
	}
}

func TestAutoApplyTailor_invalidID(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	if _, err := run(t, "auto-apply", "tailor", "not-a-number"); err == nil {
		t.Error("auto-apply tailor with a non-numeric id should error")
	}
}

func TestAutoApplyReview_approve(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	out, err := run(t, "auto-apply", "review", "42", "--approve")
	if err != nil {
		t.Fatalf("auto-apply review --approve: %v", err)
	}
	if !strings.Contains(out, "approved") {
		t.Errorf("review output %q does not confirm approval", out)
	}
}

func TestAutoApplyReview_decline(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	out, err := run(t, "auto-apply", "review", "42", "--decline")
	if err != nil {
		t.Fatalf("auto-apply review --decline: %v", err)
	}
	if !strings.Contains(out, "declined") {
		t.Errorf("review output %q does not confirm decline", out)
	}
}

func TestAutoApplyReview_neitherFlagErrors(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	if _, err := run(t, "auto-apply", "review", "42"); err == nil {
		t.Error("auto-apply review with neither --approve nor --decline should error")
	}
}

func TestAutoApplyReview_bothFlagsErrors(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	autoApplyEnv(t, srv)
	if _, err := run(t, "auto-apply", "review", "42", "--approve", "--decline"); err == nil {
		t.Error("auto-apply review with both flags should error")
	}
}

// There is deliberately no `auto-apply run`/`start` — see newAutoApplyCmd's own doc
// comment for why. Matches every other command group in this CLI (e.g. `cv`): an
// unrecognized subcommand shows help rather than erroring, so the assertion is on the
// registered subcommand set itself, not on shell-level exit behavior.
func TestAutoApplyHasNoRunCommand(t *testing.T) {
	banned := map[string]bool{"run": true, "start": true}
	for _, sub := range newAutoApplyCmd().Commands() {
		if banned[sub.Name()] {
			t.Errorf("auto-apply has a %q subcommand, want none", sub.Name())
		}
	}
}
