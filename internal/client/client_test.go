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
