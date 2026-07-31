package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// cvFakeAPI mimics the CV-tailoring endpoints behind a Bearer=good gate.
func cvFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/cvs/3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76/tailor-context", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"verdict":"Good Fit","missing_gap":[{"text":"Kubernetes"}]}}`))
	})
	mux.HandleFunc("/api/v1/me/cvs/3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76/pdf", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.7 fake"))
	})
	mux.HandleFunc("/api/v1/me/cvs/3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"ops"`) || !strings.Contains(string(b), `"kind"`) {
				t.Errorf("edit body is not a batch of path operations: %s", b)
			}
		}
		w.Write([]byte(`{"data":{"id":5,"title":"Tailored"}}`))
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

// cvEnv points the CLI at the fake server as the authenticated user.
func cvEnv(t *testing.T, srv *httptest.Server) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")
	t.Setenv("FREEHIRE_API_URL", srv.URL)
}

// testCV is an id the API would hand out: opaque, and nothing the CLI parses.
const testCV = "3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76"

func TestCVContext(t *testing.T) {
	srv := cvFakeAPI(t)
	cvEnv(t, srv)
	out, err := run(t, "cv", "context", testCV)
	if err != nil {
		t.Fatalf("cv context: %v", err)
	}
	if !strings.Contains(out, "Good Fit") || !strings.Contains(out, "missing_gap") {
		t.Errorf("context output = %q", out)
	}
}

func TestCVEditWithOpsFlag(t *testing.T) {
	srv := cvFakeAPI(t)
	cvEnv(t, srv)
	out, err := run(t, "cv", "edit", testCV, "--ops",
		`[{"kind":"insert","path":"experience[0].bullets[0]","value":"Led migration"}]`)
	if err != nil {
		t.Fatalf("cv edit: %v", err)
	}
	if !strings.Contains(out, "CV "+testCV+" updated") {
		t.Errorf("edit output = %q", out)
	}
}

func TestCVRenderWritesFile(t *testing.T) {
	srv := cvFakeAPI(t)
	cvEnv(t, srv)
	out := t.TempDir() + "/out.pdf"
	msg, err := run(t, "cv", "render", testCV, "--out", out)
	if err != nil {
		t.Fatalf("cv render: %v", err)
	}
	if !strings.Contains(msg, "wrote "+out) {
		t.Errorf("render message = %q", msg)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.HasPrefix(string(b), "%PDF") {
		t.Errorf("rendered file = %q (err %v)", b, err)
	}
}

func TestCVInvalidID(t *testing.T) {
	srv := cvFakeAPI(t)
	cvEnv(t, srv)
	if _, err := run(t, "cv", "get", "abc"); err == nil {
		t.Error("cv get with a non-numeric id should error")
	}
}

// Three shapes reach the same body, because all three are things a caller has in hand: one
// edit on the command line, a bare array, or a single edit object piped in.
func TestReadEditsAcceptsEveryShape(t *testing.T) {
	shorthand := newCVEditCmd()
	if err := shorthand.Flags().Set("set", "experience[0].bullets[1]=Cut p99 latency"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	body, err := readEdits(shorthand)
	if err != nil {
		t.Fatalf("readEdits(--set): %v", err)
	}
	for _, want := range []string{`"kind":"set"`, `"path":"experience[0].bullets[1]"`, "Cut p99 latency"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body %s is missing %s", body, want)
		}
	}

	piped := newCVEditCmd()
	piped.SetIn(strings.NewReader(`{"kind":"set","path":"summary","value":"x"}`))
	body, err = readEdits(piped)
	if err != nil {
		t.Fatalf("readEdits(stdin): %v", err)
	}
	if !strings.Contains(string(body), `"ops"`) {
		t.Errorf("a single edit was not wrapped into a batch: %s", body)
	}

	array := newCVEditCmd()
	if err := array.Flags().Set("ops", `[{"kind":"remove","path":"skills[0]"}]`); err != nil {
		t.Fatalf("ops flag: %v", err)
	}
	if body, err = readEdits(array); err != nil {
		t.Fatalf("readEdits(--ops): %v", err)
	} else if !strings.Contains(string(body), `"ops"`) {
		t.Errorf("an array was not wrapped into a batch: %s", body)
	}
}

func TestReadEditsRefusesAnAmbiguousOrEmptyCall(t *testing.T) {
	both := newCVEditCmd()
	_ = both.Flags().Set("set", "summary=x")
	_ = both.Flags().Set("remove", "skills[0]")
	if _, err := readEdits(both); err == nil {
		t.Error("two shorthand edits in one call should error rather than pick one")
	}

	malformed := newCVEditCmd()
	_ = malformed.Flags().Set("set", "summary")
	if _, err := readEdits(malformed); err == nil {
		t.Error("--set without a value should error")
	}

	empty := newCVEditCmd()
	empty.SetIn(strings.NewReader("   "))
	if _, err := readEdits(empty); err == nil {
		t.Error("an empty edit should error")
	}
}

// A claim about the candidate needs its citation carried through to the server, which is
// where the rule is enforced.
func TestReadEditsCarriesTheEvidenceId(t *testing.T) {
	cmd := newCVEditCmd()
	_ = cmd.Flags().Set("set", "experience[0].bullets[0]=Ran the cluster")
	_ = cmd.Flags().Set("evidence", "a71f-…")
	_ = cmd.Flags().Set("note", "under the Kubernetes requirement")

	body, err := readEdits(cmd)
	if err != nil {
		t.Fatalf("readEdits: %v", err)
	}
	for _, want := range []string{`"evidence_id":"a71f-…"`, `"note":"under the Kubernetes requirement"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body %s is missing %s", body, want)
		}
	}
}
