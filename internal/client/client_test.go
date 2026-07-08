package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeAPI is an httptest server mimicking the freehire API envelope. It rejects
// any request whose bearer token is not "good" with 401, so the client's auth
// header and error mapping are both exercised.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		io := `{"data":{"id":7,"email":"agent@example.test"}}`
		w.Write([]byte(io))
	})
	mux.HandleFunc("/api/v1/jobs/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("q") != "golang" {
			t.Errorf("search q = %q, want golang", q.Get("q"))
		}
		if q.Get("work_mode") != "remote" {
			t.Errorf("work_mode = %q, want remote", q.Get("work_mode"))
		}
		if regions := q["regions"]; len(regions) != 2 || regions[0] != "eu" || regions[1] != "us" {
			t.Errorf("regions = %v, want [eu us]", regions)
		}
		w.Write([]byte(`{"data":[{"public_slug":"go-dev","title":"Go Dev"}],"meta":{"total":42}}`))
	})
	mux.HandleFunc("/api/v1/jobs/go-dev/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("apply method = %s, want POST", r.Method)
		}
		w.Write([]byte(`{"data":{"job_id":1,"applied_at":"2026-06-13T00:00:00Z"}}`))
	})
	mux.HandleFunc("/api/v1/jobs/nope", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	mux.HandleFunc("/api/v1/jobs/go-dev", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"public_slug":"go-dev","title":"Go Dev","description":"Build things"}}`))
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

func TestClient_Me(t *testing.T) {
	srv := fakeAPI(t)
	c := New(srv.URL, "good", srv.Client())

	data, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	var u struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(data, &u); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if u.Email != "agent@example.test" {
		t.Errorf("email = %q", u.Email)
	}
}

func TestClient_Search(t *testing.T) {
	srv := fakeAPI(t)
	c := New(srv.URL, "good", srv.Client())

	res, err := c.Search(context.Background(), SearchParams{
		Query:  "golang",
		Limit:  20,
		Offset: 0,
		Facets: url.Values{"work_mode": {"remote"}, "regions": {"eu", "us"}},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 42 {
		t.Errorf("total = %d, want 42", res.Total)
	}
	var items []map[string]any
	if err := json.Unmarshal(res.Data, &items); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if len(items) != 1 || items[0]["public_slug"] != "go-dev" {
		t.Errorf("items = %v", items)
	}
}

func TestClient_GetJobAndApply(t *testing.T) {
	srv := fakeAPI(t)
	c := New(srv.URL, "good", srv.Client())

	job, err := c.GetJob(context.Background(), "go-dev")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if !strings.Contains(string(job), "Build things") {
		t.Errorf("job data = %s", job)
	}

	applied, err := c.Apply(context.Background(), "go-dev")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(string(applied), "applied_at") {
		t.Errorf("apply data = %s", applied)
	}
}

func TestClient_ErrorMapping(t *testing.T) {
	srv := fakeAPI(t)

	// Wrong token -> 401 APIError.
	bad := New(srv.URL, "bad", srv.Client())
	_, err := bad.Me(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want *APIError status 401", err)
	}

	// Known token, missing job -> 404 APIError.
	good := New(srv.URL, "good", srv.Client())
	_, err = good.GetJob(context.Background(), "nope")
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("err = %v, want *APIError status 404", err)
	}
}

func TestClient_SaveUnsaveMyJobs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev/save", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"job_id":1,"saved_at":"2026-06-13T00:00:00Z"}}`))
	})
	mux.HandleFunc("/api/v1/me/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") != "saved" {
			t.Errorf("filter = %q, want saved", r.URL.Query().Get("filter"))
		}
		w.Write([]byte(`{"data":[{"job":{"public_slug":"go-dev"}}],"meta":{"total":1}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	if _, err := c.Save(context.Background(), "go-dev"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := c.Unsave(context.Background(), "go-dev"); err != nil {
		t.Fatalf("Unsave: %v", err)
	}
	res, err := c.MyJobs(context.Background(), "saved", 20, 0)
	if err != nil {
		t.Fatalf("MyJobs: %v", err)
	}
	if res.Total != 1 {
		t.Errorf("total = %d, want 1", res.Total)
	}
}

func TestClient_Track(t *testing.T) {
	var gotMethod, gotCT string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev/track", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"data":{"job_id":1,"stage":"interview","notes":null}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	stage := "interview"
	data, err := c.Track(context.Background(), "go-dev", TrackParams{Stage: &stage})
	if err != nil {
		t.Fatalf("Track: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	if gotBody["stage"] != "interview" {
		t.Errorf("body stage = %v, want interview", gotBody["stage"])
	}
	// A nil field must be omitted so the server leaves that column unchanged.
	if _, ok := gotBody["notes"]; ok {
		t.Errorf("notes should be omitted when nil, body = %v", gotBody)
	}
	if !strings.Contains(string(data), "interview") {
		t.Errorf("data = %s", data)
	}
}

func TestClient_CreateJob(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"public_slug":"go-dev-acme-abcd1234","title":"Go Dev"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	data, err := c.CreateJob(context.Background(), CreateJobParams{
		URL:     "https://acme.example/jobs/1",
		Source:  "workatastartup",
		Title:   "Go Dev",
		Company: "Acme",
		Remote:  true,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotBody["url"] != "https://acme.example/jobs/1" || gotBody["company"] != "Acme" {
		t.Errorf("body = %v, want url+company set", gotBody)
	}
	if gotBody["source"] != "workatastartup" {
		t.Errorf("source = %v, want workatastartup", gotBody["source"])
	}
	if gotBody["remote"] != true {
		t.Errorf("remote = %v, want true", gotBody["remote"])
	}
	if _, ok := gotBody["posted_at"]; ok {
		t.Errorf("posted_at should be omitted when nil, body = %v", gotBody)
	}
	if !strings.Contains(string(data), "go-dev-acme-abcd1234") {
		t.Errorf("data = %s", data)
	}
}

func TestClient_EditJob(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/go-dev-acme-abcd1234", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"data":{"public_slug":"go-dev-acme-abcd1234","title":"Staff Dev"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	title := "Staff Dev"
	if _, err := c.EditJob(context.Background(), "go-dev-acme-abcd1234", EditJobParams{Title: &title}); err != nil {
		t.Fatalf("EditJob: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotBody["title"] != "Staff Dev" {
		t.Errorf("title = %v, want Staff Dev", gotBody["title"])
	}
	// Unset fields are omitted so the server leaves them unchanged (partial update).
	for _, f := range []string{"company", "location", "description", "remote", "posted_at"} {
		if _, ok := gotBody[f]; ok {
			t.Errorf("%s should be omitted when nil, body = %v", f, gotBody)
		}
	}
}

func TestClient_GetCompany(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/companies/acme", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.Write([]byte(`{"data":{"company":{"slug":"acme","name":"Acme"},"jobs":[{"public_slug":"go-dev","title":"Go Dev"}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	data, err := c.GetCompany(context.Background(), "acme")
	if err != nil {
		t.Fatalf("GetCompany: %v", err)
	}
	if !strings.Contains(string(data), `"name":"Acme"`) || !strings.Contains(string(data), "go-dev") {
		t.Errorf("company data = %s", data)
	}
}

func TestClient_Coverage(t *testing.T) {
	var gotMethod, gotCT string
	var gotQuery url.Values
	var gotBody struct {
		Skills []string `json:"skills"`
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/market/coverage", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotQuery = r.URL.Query()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"data":{"total":500,"covered":300,"coverage_percent":60,"gaps":[]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	data, err := c.Coverage(context.Background(), CoverageParams{
		Skills: []string{"go", "docker"},
		Facets: url.Values{"category": {"backend"}, "countries": {"BR"}},
	})
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	if len(gotBody.Skills) != 2 || gotBody.Skills[0] != "go" || gotBody.Skills[1] != "docker" {
		t.Errorf("body skills = %v, want [go docker]", gotBody.Skills)
	}
	if gotQuery.Get("category") != "backend" || gotQuery.Get("countries") != "BR" {
		t.Errorf("query facets = %v, want category=backend&countries=BR", gotQuery)
	}
	if !strings.Contains(string(data), `"coverage_percent":60`) {
		t.Errorf("coverage data = %s", data)
	}
}
