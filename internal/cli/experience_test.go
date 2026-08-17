package cli

import (
	"encoding/json"
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
		w.Write([]byte(`{"data":{"employments":[
			{"id":"11111111-1111-4111-8111-111111111111","kind":"job","company":"RingCentral","role":"SWE","start":"Mar 2021","atoms":[]},
			{"id":"33333333-3333-4333-8333-333333333333","kind":"job","company":"Informa","atoms":[{"id":"44444444-4444-4444-8444-444444444444","claim":"Led the migration to serverless","provenance":"cv_import"}]}
		],"unplaced":[{"id":"22222222-2222-4222-8222-222222222222","claim":"Cut latency 20s to 1s","context":"the checkout path","metrics":["20s->1s"],"skills":["go","kubernetes"],"provenance":"manual"}]}}`))
	})
	mux.HandleFunc("/api/v1/me/experience/atoms/22222222-2222-4222-8222-222222222222", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("atom update method = %s, want PUT", r.Method)
		}
		// The server replaces the whole row, so anything the caller did not name has to be
		// carried over from what is already banked — otherwise correcting a typo silently
		// deletes the numbers that made the achievement worth citing.
		//
		// Decoded rather than substring-matched: encoding/json escapes `>` (and `<`, `&`)
		// to a \u sequence by default, so a literal search for a metric like "20s->1s"
		// fails on a body that is in fact correct.
		var sent struct {
			Claim   string   `json:"claim"`
			Context string   `json:"context"`
			Metrics []string `json:"metrics"`
			Skills  []string `json:"skills"`
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("atom update body is not JSON: %v", err)
		}
		if sent.Claim != "Cut latency 20s to 900ms" {
			t.Errorf("claim = %q, want the correction", sent.Claim)
		}
		if sent.Context != "the checkout path" {
			t.Errorf("context = %q, want it carried over", sent.Context)
		}
		if strings.Join(sent.Metrics, ",") != "20s->1s" {
			t.Errorf("metrics = %v, want them carried over", sent.Metrics)
		}
		if strings.Join(sent.Skills, ",") != "go,kubernetes" {
			t.Errorf("skills = %v, want them carried over", sent.Skills)
		}
		w.Write([]byte(`{"data":{"id":"22222222-2222-4222-8222-222222222222","claim":"Cut latency 20s to 900ms","provenance":"manual"}}`))
	})
	mux.HandleFunc("/api/v1/me/experience/employments/11111111-1111-4111-8111-111111111111", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("employment update method = %s, want PUT", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		for _, want := range []string{`"company":"RingCentral Inc"`, `"role":"SWE"`, `"start":"Mar 2021"`} {
			if !strings.Contains(string(b), want) {
				t.Errorf("employment update body %s is missing %s", b, want)
			}
		}
		w.Write([]byte(`{"data":{"id":"11111111-1111-4111-8111-111111111111","company":"RingCentral Inc"}}`))
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

// Correcting one field must not cost the others. The server's PUT replaces the whole row,
// so the command reads what is banked and sends it back with only the named flags changed;
// the fake asserts the metrics, skills and context survive a claim-only correction.
func TestExperienceAtomsUpdateKeepsUnnamedFields(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	out, err := run(t, "experience", "atoms", "update", "22222222-2222-4222-8222-222222222222",
		"--claim", "Cut latency 20s to 900ms")
	if err != nil {
		t.Fatalf("experience atoms update: %v", err)
	}
	if !strings.Contains(out, "Cut latency 20s to 900ms") {
		t.Errorf("output = %q, want the corrected claim", out)
	}
}

func TestExperienceEmploymentsUpdateKeepsUnnamedFields(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	out, err := run(t, "experience", "employments", "update", "11111111-1111-4111-8111-111111111111",
		"--company", "RingCentral Inc")
	if err != nil {
		t.Fatalf("experience employments update: %v", err)
	}
	if !strings.Contains(out, "RingCentral Inc") {
		t.Errorf("output = %q, want the corrected company", out)
	}
}

// An id that is not in the caller's bank must fail before the write, not send a PUT that
// would be refused anyway — the message can then name what to do instead.
func TestExperienceUpdateUnknownID(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	_, err := run(t, "experience", "atoms", "update", "33333333-3333-4333-8333-333333333333", "--claim", "x")
	if err == nil {
		t.Fatal("updating an atom that is not in the bank should error")
	}
	if !strings.Contains(err.Error(), "experience list") {
		t.Errorf("error %q should point at `experience list`", err)
	}
}

// Naming no field is a no-op dressed as a write; refuse it rather than round-trip the row.
func TestExperienceUpdateRequiresAField(t *testing.T) {
	srv := experienceFakeAPI(t)
	experienceEnv(t, srv)
	if _, err := run(t, "experience", "atoms", "update", "22222222-2222-4222-8222-222222222222"); err == nil {
		t.Error("atoms update with no field flags should error")
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
