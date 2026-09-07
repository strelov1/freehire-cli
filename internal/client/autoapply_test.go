package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// autoApplyFakeAPI mimics the freehire auto-apply review endpoints behind a Bearer=good
// gate.
func autoApplyFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/tracking/go-dev-acme", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("status method = %s, want GET", r.Method)
		}
		w.Write([]byte(`{"data":{"job":{"public_slug":"go-dev-acme"},"auto_apply":{"status":"pending_review","queue_id":42,"resolved_preview":{"fields":[{"label":"Email","value":"a@b.com"}]}}}}`))
	})
	mux.HandleFunc("/api/v1/me/tracking/no-attempt", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"job":{"public_slug":"no-attempt"},"auto_apply":null}}`))
	})
	mux.HandleFunc("/api/v1/me/auto-apply/42/tailor", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("tailor method = %s, want POST", r.Method)
		}
		w.Write([]byte(`{"data":{"session_id":"s1"}}`))
	})
	mux.HandleFunc("/api/v1/me/auto-apply/42/review", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("review method = %s, want POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"decision":"approved"`) {
			t.Errorf("review body = %s, want decision=approved", b)
		}
		w.Write([]byte(`{"data":{"decision":"approved"}}`))
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

func TestClient_AutoApplyStatus(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.AutoApplyStatus(context.Background(), "go-dev-acme")
	if err != nil {
		t.Fatalf("AutoApplyStatus: %v", err)
	}
	if !strings.Contains(string(data), `"pending_review"`) {
		t.Errorf("status data = %s", data)
	}
}

func TestClient_AutoApplyStatus_none(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.AutoApplyStatus(context.Background(), "no-attempt")
	if err != nil {
		t.Fatalf("AutoApplyStatus: %v", err)
	}
	if !strings.Contains(string(data), `"auto_apply":null`) {
		t.Errorf("status data = %s, want a null auto_apply", data)
	}
}

func TestClient_TailorAutoApplyQueueEntry(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.TailorAutoApplyQueueEntry(context.Background(), 42)
	if err != nil {
		t.Fatalf("TailorAutoApplyQueueEntry: %v", err)
	}
	if !strings.Contains(string(data), `"session_id"`) {
		t.Errorf("tailor data = %s", data)
	}
}

func TestClient_ReviewAutoApplyQueueEntry(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.ReviewAutoApplyQueueEntry(context.Background(), 42, "approved")
	if err != nil {
		t.Fatalf("ReviewAutoApplyQueueEntry: %v", err)
	}
	if !strings.Contains(string(data), `"approved"`) {
		t.Errorf("review data = %s", data)
	}
}

func TestClient_AutoApplyStatus_unauthorized(t *testing.T) {
	srv := autoApplyFakeAPI(t)
	c := New(srv.URL, "bad", srv.Client())
	_, err := c.AutoApplyStatus(context.Background(), "go-dev-acme")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Errorf("AutoApplyStatus unauth err = %v, want APIError 401", err)
	}
}
