package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// experienceFakeAPI mimics the experience-bank endpoints behind a Bearer=good gate.
func experienceFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/experience", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"employments":[{"id":"11111111-1111-4111-8111-111111111111","company":"RingCentral","atoms":[]}],"unplaced":[{"id":"22222222-2222-4222-8222-222222222222","claim":"Cut latency 20s to 1s","provenance":"manual"}]}}`))
	})
	mux.HandleFunc("/api/v1/me/experience/employments", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"company":"RingCentral"`) {
			t.Errorf("employment body = %s, want company RingCentral", b)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"11111111-1111-4111-8111-111111111111","kind":"job","company":"RingCentral"}}`))
	})
	mux.HandleFunc("/api/v1/me/experience/atoms", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"claim":"Cut latency 20s to 1s"`) {
			t.Errorf("atom body = %s, want the claim", b)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"22222222-2222-4222-8222-222222222222","claim":"Cut latency 20s to 1s","provenance":"manual"}}`))
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

func experienceEnv(t *testing.T, srv *httptest.Server) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")
	t.Setenv("FREEHIRE_API_URL", srv.URL)
}

func TestExperienceList(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	out, err := run(t, "experience", "list")
	if err != nil {
		t.Fatalf("experience list: %v", err)
	}
	if !strings.Contains(out, "RingCentral") || !strings.Contains(out, "Cut latency 20s to 1s") {
		t.Errorf("list output = %q, want the employment and the unplaced atom", out)
	}
}

func TestExperienceEmploymentsAdd(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	out, err := run(t, "experience", "employments", "add", "--company", "RingCentral", "--role", "SWE")
	if err != nil {
		t.Fatalf("experience employments add: %v", err)
	}
	if !strings.Contains(out, "11111111-1111-4111-8111-111111111111") {
		t.Errorf("output = %q, want the new employment's id", out)
	}
}

func TestExperienceEmploymentsAddRequiresCompanyOrRole(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	if _, err := run(t, "experience", "employments", "add"); err == nil {
		t.Error("employments add with neither --company nor --role should error before the request")
	}
}

func TestExperienceAtomsAdd(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	out, err := run(t, "experience", "atoms", "add", "--claim", "Cut latency 20s to 1s",
		"--metric", "20s->1s", "--skill", "go", "--skill", "kubernetes")
	if err != nil {
		t.Fatalf("experience atoms add: %v", err)
	}
	if !strings.Contains(out, "22222222-2222-4222-8222-222222222222") {
		t.Errorf("output = %q, want the new atom's id", out)
	}
}

func TestExperienceAtomsAddRequiresClaim(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	if _, err := run(t, "experience", "atoms", "add"); err == nil {
		t.Error("atoms add with no --claim should error before the request")
	}
}

func TestExperienceAtomsAddConflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/experience/atoms", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"experience: this claim is already in the bank"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	experienceEnv(t, srv)

	if _, err := run(t, "experience", "atoms", "add", "--claim", "Cut latency 20s to 1s"); err == nil {
		t.Error("re-adding an already-banked claim should surface the server's error, not succeed silently")
	}
}
