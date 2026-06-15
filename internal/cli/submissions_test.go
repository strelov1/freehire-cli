package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmitCreatesSubmission(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/submissions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":5,"status":"pending","title":"Go Dev","company":"Acme"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "submit", "--url", "https://x/1", "--title", "Go Dev",
		"--company", "Acme", "--remote", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if gotBody["url"] != "https://x/1" || gotBody["title"] != "Go Dev" || gotBody["company"] != "Acme" {
		t.Errorf("submit body = %v", gotBody)
	}
	if gotBody["remote"] != true {
		t.Errorf("remote = %v, want true", gotBody["remote"])
	}
	if !strings.Contains(out, "Go Dev") || !strings.Contains(out, "pending") {
		t.Errorf("submit output = %q", out)
	}
}

func TestSubmissionsListsMine(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/submissions", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":[{"id":5,"status":"approved","title":"Go Dev","company":"Acme"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "submissions", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("submissions: %v", err)
	}
	if !strings.Contains(out, "Go Dev") || !strings.Contains(out, "approved") {
		t.Errorf("submissions output = %q", out)
	}
}

func TestSubmissionsPendingShowsSubmitter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/submissions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.Write([]byte(`{"data":[{"id":5,"status":"pending","title":"Go Dev","company":"Acme","submitter_email":"u1@example.test"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "submissions", "pending", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("submissions pending: %v", err)
	}
	if !strings.Contains(out, "Go Dev") || !strings.Contains(out, "u1@example.test") {
		t.Errorf("pending output = %q", out)
	}
}

func TestSubmissionsApprove(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/submissions/5/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Write([]byte(`{"data":{"id":5,"status":"approved","title":"Go Dev"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "submissions", "approve", "5", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !strings.Contains(out, "Approved") || !strings.Contains(out, "5") {
		t.Errorf("approve output = %q", out)
	}
}

func TestSubmissionsRejectSendsReason(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/submissions/5/reject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"data":{"id":5,"status":"rejected","review_reason":"dup"}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FREEHIRE_TOKEN", "good")

	out, err := run(t, "submissions", "reject", "5", "--reason", "dup", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if gotBody["reason"] != "dup" {
		t.Errorf("reject body reason = %v, want dup", gotBody["reason"])
	}
	if !strings.Contains(out, "Rejected") {
		t.Errorf("reject output = %q", out)
	}
}
