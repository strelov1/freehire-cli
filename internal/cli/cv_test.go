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
	mux.HandleFunc("/api/v1/me/cvs/5/tailor-context", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"verdict":"Good Fit","missing_gap":[{"text":"Kubernetes"}]}}`))
	})
	mux.HandleFunc("/api/v1/me/cvs/5/pdf", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.7 fake"))
	})
	mux.HandleFunc("/api/v1/me/cvs/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"op":"add_bullet"`) {
				t.Errorf("patch body missing op: %s", b)
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

func TestCVContext(t *testing.T) {
	srv := cvFakeAPI(t)
	cvEnv(t, srv)
	out, err := run(t, "cv", "context", "5")
	if err != nil {
		t.Fatalf("cv context: %v", err)
	}
	if !strings.Contains(out, "Good Fit") || !strings.Contains(out, "missing_gap") {
		t.Errorf("context output = %q", out)
	}
}

func TestCVEditWithPatchFlag(t *testing.T) {
	srv := cvFakeAPI(t)
	cvEnv(t, srv)
	out, err := run(t, "cv", "edit", "5", "--patch", `{"op":"add_bullet","experience":0,"value":"Led migration"}`)
	if err != nil {
		t.Fatalf("cv edit: %v", err)
	}
	if !strings.Contains(out, "CV 5 updated") {
		t.Errorf("edit output = %q", out)
	}
}

func TestCVRenderWritesFile(t *testing.T) {
	srv := cvFakeAPI(t)
	cvEnv(t, srv)
	out := t.TempDir() + "/out.pdf"
	msg, err := run(t, "cv", "render", "5", "--out", out)
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

func TestReadPatchFromStdin(t *testing.T) {
	cmd := newCVEditCmd()
	cmd.SetIn(strings.NewReader(`{"op":"set_summary","value":"x"}`))
	patch, err := readPatch(cmd)
	if err != nil {
		t.Fatalf("readPatch stdin: %v", err)
	}
	if !strings.Contains(string(patch), "set_summary") {
		t.Errorf("patch = %s", patch)
	}

	empty := newCVEditCmd()
	empty.SetIn(strings.NewReader("   "))
	if _, err := readPatch(empty); err == nil {
		t.Error("empty patch should error")
	}
}
