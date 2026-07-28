package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// profileAPI serves one canned profile, so the command's rendering can be tested
// without a server. The payload mirrors GET /api/v1/me/profile: the saved
// preferences plus the CV with its contact fields left out.
func profileAPI(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		if r.URL.Path != "/api/v1/me/profile" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const savedProfileJSON = `{"data":{
	"specializations":["backend"],
	"skills":["go","kubernetes"],
	"excluded_skills":["php"],
	"location_preferences":{"work_modes":["remote"],"remote":{"regions":["eu"]},"base":{"country":"pt","city":"Lisbon"},"relocation":{"open":false}},
	"cv":{"headline":"Staff Backend Engineer","total_years":11,"skills":["Go","Kafka"]},
	"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-07-01T00:00:00Z"}}`

func TestProfileShowsTheSavedPreferences(t *testing.T) {
	srv := profileAPI(t, savedProfileJSON)
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "--api-url", srv.URL, "profile")
	if err != nil {
		t.Fatalf("profile: %v (%s)", err, out)
	}
	for _, want := range []string{"backend", "go", "kubernetes", "php", "remote", "Staff Backend Engineer", "11"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestProfileJSONIsTheRawPayload(t *testing.T) {
	srv := profileAPI(t, savedProfileJSON)
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "--json", "--api-url", srv.URL, "profile")
	if err != nil {
		t.Fatalf("profile --json: %v (%s)", err, out)
	}
	if !strings.Contains(out, `"specializations"`) || !strings.Contains(out, `"cv"`) {
		t.Errorf("--json should print the API payload verbatim:\n%s", out)
	}
}

// A user who has not saved a profile gets a plain instruction, not an empty table
// and not an error — the same answer the in-app assistant gives.
func TestProfileWithoutOneSaysWhereToMakeOne(t *testing.T) {
	srv := profileAPI(t, `{"data":null}`)
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "--api-url", srv.URL, "profile")
	if err != nil {
		t.Fatalf("no profile is an answer, not a failure: %v (%s)", err, out)
	}
	if !strings.Contains(out, "/my/profile") {
		t.Errorf("output should say where to create one:\n%s", out)
	}
}
