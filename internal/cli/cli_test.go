package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire-cli/internal/config"
)

// fakeAPI mimics the freehire API; any request without "Bearer good" is 401.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"id":7,"email":"agent@example.test"}}`))
	})
	mux.HandleFunc("/api/v1/jobs/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":[{"public_slug":"go-dev","title":"Go Dev","company":"Acme","location":"Remote"}],"meta":{"total":42}}`))
	})
	mux.HandleFunc("/api/v1/jobs/go-dev/apply", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"job_id":1,"applied_at":"2026-06-13T00:00:00Z"}}`))
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

// run executes the root command fresh with args, capturing stdout+stderr.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	return func() (string, error) { err := root.Execute(); return out.String(), err }()
}

func TestAuthLoginValidatesAndWritesCreds(t *testing.T) {
	srv := fakeAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "")
	t.Setenv("FREEHIRE_API_URL", "")

	out, err := run(t, "auth", "login", "--token", "good", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out, "agent@example.test") {
		t.Errorf("login output = %q, want it to show the email", out)
	}
	got, _ := config.Load()
	if got.Token != "good" || got.APIURL != srv.URL {
		t.Errorf("stored creds = %+v", got)
	}
}

func TestAuthLoginRejectsBadKey(t *testing.T) {
	srv := fakeAPI(t)
	t.Setenv("HOME", t.TempDir())
	if _, err := run(t, "auth", "login", "--token", "bad", "--api-url", srv.URL); err == nil {
		t.Error("login with a bad key should error and not store creds")
	}
	if got, _ := config.Load(); got.Token != "" {
		t.Errorf("bad-key login must not write creds, got %+v", got)
	}
}

func TestSearchUsesEnvToken(t *testing.T) {
	srv := fakeAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "search", "golang", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(out, "Go Dev") || !strings.Contains(out, "go-dev") {
		t.Errorf("search output = %q", out)
	}
}

func TestSearchJSONPassthrough(t *testing.T) {
	srv := fakeAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "--json", "search", "golang", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("search --json: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &arr); err != nil {
		t.Fatalf("--json output is not a JSON array: %v (%q)", err, out)
	}
	if len(arr) != 1 || arr[0]["public_slug"] != "go-dev" {
		t.Errorf("--json array = %v", arr)
	}
}

func TestApplyConfirms(t *testing.T) {
	srv := fakeAPI(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "apply", "go-dev", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out, "Marked applied: go-dev") {
		t.Errorf("apply output = %q", out)
	}
}

func TestSearchWithoutTokenErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "")
	if _, err := run(t, "search", "golang"); err == nil {
		t.Error("expected an error when no token is configured")
	}
}
