package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestSearchFiltersMapToFacets(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got = r.URL.Query()
		w.Write([]byte(`{"data":[],"meta":{"total":0}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	_, err := run(t, "search", "golang",
		"--remote", "--region", "eu", "--region", "us", "--company", "acme",
		"--api-url", srv.URL)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got.Get("work_mode") != "remote" {
		t.Errorf("work_mode = %q, want remote", got.Get("work_mode"))
	}
	if r := got["regions"]; len(r) != 2 || r[0] != "eu" || r[1] != "us" {
		t.Errorf("regions = %v, want [eu us]", r)
	}
	if got.Get("company_slug") != "acme" {
		t.Errorf("company_slug = %q, want acme", got.Get("company_slug"))
	}
}

func TestSaveAndMy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev/save", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"job_id":1,"saved_at":"2026-06-13T00:00:00Z"}}`))
	})
	mux.HandleFunc("/api/v1/me/tracking", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") != "saved" {
			t.Errorf("filter = %q, want saved", r.URL.Query().Get("filter"))
		}
		w.Write([]byte(`{"data":[{"job":{"public_slug":"go-dev","title":"Go Dev","company":"Acme"},"saved_at":"2026-06-13T00:00:00Z","applied_at":null}],"meta":{"total":1}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "save", "go-dev", "--api-url", srv.URL)
	if err != nil || !strings.Contains(out, "Saved: go-dev") {
		t.Errorf("save: err=%v out=%q", err, out)
	}
	out, err = run(t, "my", "--filter", "saved", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("my: %v", err)
	}
	if !strings.Contains(out, "Go Dev") || !strings.Contains(out, "go-dev") {
		t.Errorf("my out = %q", out)
	}
}

func TestStageCmd(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev/track", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"data":{"job_id":1,"stage":"interview"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "stage", "go-dev", "interview", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if gotBody["stage"] != "interview" {
		t.Errorf("body stage = %v, want interview", gotBody["stage"])
	}
	if !strings.Contains(out, "go-dev") || !strings.Contains(out, "interview") {
		t.Errorf("stage out = %q", out)
	}
}

func TestNoteCmd(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev/track", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"data":{"job_id":1,"notes":"call back monday"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	// Multi-word note without quoting: trailing args join into the note text.
	out, err := run(t, "note", "go-dev", "call", "back", "monday", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	if gotBody["notes"] != "call back monday" {
		t.Errorf("body notes = %v, want 'call back monday'", gotBody["notes"])
	}
	if _, ok := gotBody["stage"]; ok {
		t.Errorf("stage should be omitted, body = %v", gotBody)
	}
	if !strings.Contains(out, "go-dev") {
		t.Errorf("note out = %q", out)
	}
}

func TestCompanyCmd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/companies/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"company":{"slug":"acme","name":"Acme Inc"},"jobs":[{"public_slug":"go-dev","title":"Go Dev","location":"Remote"}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "company", "acme", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("company: %v", err)
	}
	if !strings.Contains(out, "Acme Inc") || !strings.Contains(out, "go-dev") {
		t.Errorf("company out = %q", out)
	}
}

func TestMyShowsStageAndNote(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/tracking", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":[{"job":{"public_slug":"go-dev","title":"Go Dev","company":"Acme"},"applied_at":"2026-06-13T00:00:00Z","stage":"interview","notes":"recruiter call"}],"meta":{"total":1}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "my", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("my: %v", err)
	}
	// The stage replaces the coarse state, and the note appears as a snippet.
	if !strings.Contains(out, "interview") {
		t.Errorf("my out missing stage: %q", out)
	}
	if !strings.Contains(out, "recruiter call") {
		t.Errorf("my out missing note: %q", out)
	}
}

func TestJobsAddAndEdit(t *testing.T) {
	var addBody, editBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&addBody)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"public_slug":"go-dev-acme-abcd1234","title":"Go Dev"}}`))
	})
	mux.HandleFunc("/api/v1/jobs/go-dev-acme-abcd1234", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&editBody)
		w.Write([]byte(`{"data":{"public_slug":"go-dev-acme-abcd1234","title":"Staff Dev"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "jobs", "add", "--api-url", srv.URL,
		"--url", "https://acme.example/jobs/1", "--source", "workatastartup",
		"--title", "Go Dev", "--company", "Acme", "--remote")
	if err != nil {
		t.Fatalf("jobs add: %v", err)
	}
	if !strings.Contains(out, "go-dev-acme-abcd1234") {
		t.Errorf("add out = %q, want the created slug", out)
	}
	if addBody["url"] != "https://acme.example/jobs/1" || addBody["remote"] != true {
		t.Errorf("add body = %v", addBody)
	}
	if addBody["source"] != "workatastartup" {
		t.Errorf("add body source = %v, want workatastartup", addBody["source"])
	}

	out, err = run(t, "jobs", "edit", "go-dev-acme-abcd1234", "--api-url", srv.URL, "--title", "Staff Dev")
	if err != nil {
		t.Fatalf("jobs edit: %v", err)
	}
	if !strings.Contains(out, "Job updated") {
		t.Errorf("edit out = %q", out)
	}
	if editBody["title"] != "Staff Dev" {
		t.Errorf("edit body = %v, want only title", editBody)
	}
	if _, ok := editBody["company"]; ok {
		t.Errorf("edit body should omit unset fields, got %v", editBody)
	}
}

func TestJobsEditRequiresAField(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")
	if _, err := run(t, "jobs", "edit", "some-slug", "--api-url", "http://unused.test"); err == nil {
		t.Error("jobs edit with no field flags should error")
	}
}

func TestMarketFitSendsSkillsBodyAndFacetQuery(t *testing.T) {
	var gotMethod string
	var gotQuery url.Values
	var gotBody struct {
		Skills []string `json:"skills"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		gotMethod = r.Method
		gotQuery = r.URL.Query()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"data":{"total":500,"covered":300,"coverage_percent":60,"must_have_total":5,"must_have_covered":3,"stack_match_percent":55,"gaps":[{"name":"kubernetes","new_vacancies":80,"unlock_percent":16}]}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "market-fit",
		"--skills", "go,docker", "--skills", "react",
		"--category", "backend", "--country", "BR", "--facet", "source=greenhouse",
		"--api-url", srv.URL)
	if err != nil {
		t.Fatalf("market-fit: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	// Skills (comma-split + repeated) go in the body, not the query.
	if len(gotBody.Skills) != 3 || gotBody.Skills[0] != "go" || gotBody.Skills[2] != "react" {
		t.Errorf("body skills = %v, want [go docker react]", gotBody.Skills)
	}
	if gotQuery.Has("skills") {
		t.Errorf("skills must not leak into the market filter query: %v", gotQuery["skills"])
	}
	// Named facet, new named facet, and the generic --facet all reach the query.
	if gotQuery.Get("category") != "backend" || gotQuery.Get("countries") != "BR" || gotQuery.Get("source") != "greenhouse" {
		t.Errorf("query facets = %v", gotQuery)
	}
	if !strings.Contains(out, "Coverage: 60%") || !strings.Contains(out, "kubernetes") {
		t.Errorf("human output = %q", out)
	}
}

func TestMarketFitRequiresSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")
	if _, err := run(t, "market-fit", "--api-url", "http://unused"); err == nil {
		t.Fatal("market-fit without --skills should error")
	}
}

func TestFacetsListsValuesAndStats(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		gotQuery = r.URL.Query()
		w.Write([]byte(`{"data":{"total":1234,"facets":{"category":{"backend":300,"frontend":200},"skills":{"go":180,"react":90}},"stats":{"salary_min":{"min":0,"max":400000}}}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "facets", "--category", "backend", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("facets: %v", err)
	}
	if gotQuery.Get("category") != "backend" {
		t.Errorf("query = %v, want category=backend", gotQuery)
	}
	// Values are shown count-descending with counts, and the numeric stat range.
	if !strings.Contains(out, "backend (300)") || !strings.Contains(out, "go (180)") {
		t.Errorf("human output missing facet values: %q", out)
	}
	if !strings.Contains(out, "1234 open vacancies") || !strings.Contains(out, "salary_min:") {
		t.Errorf("human output missing total/stats: %q", out)
	}
}
