package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cvFakeAPI mimics the freehire CV-tailoring endpoints behind a Bearer=good gate.
func cvFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/cvs/3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76/tailor-context", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("tailor-context method = %s, want GET", r.Method)
		}
		w.Write([]byte(`{"data":{"verdict":"Good Fit","missing_gap":[{"text":"Kubernetes"}]}}`))
	})
	mux.HandleFunc("/api/v1/me/cvs/3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76/pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.7 fake"))
	})
	mux.HandleFunc("/api/v1/me/cvs/3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`{"data":{"id":5,"document":{"summary":"eng"}}}`))
		case http.MethodPatch:
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"ops"`) || !strings.Contains(string(b), `"path"`) {
				t.Errorf("edit body did not carry path operations: %s", b)
			}
			w.Write([]byte(`{"data":{"id":5,"title":"Tailored"}}`))
		default:
			t.Errorf("unexpected method %s on /me/cvs/5", r.Method)
		}
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

// testCV is an id the API would hand out: opaque, and nothing the CLI parses.
const testCV = "3f2a9c14-7b6e-4a58-9d21-8e4c5f0b1a76"

func TestClient_TailorCVContext(t *testing.T) {
	srv := cvFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.TailorCVContext(context.Background(), testCV)
	if err != nil {
		t.Fatalf("TailorCVContext: %v", err)
	}
	if !strings.Contains(string(data), "Good Fit") {
		t.Errorf("context data = %s", data)
	}
}

func TestClient_GetCV(t *testing.T) {
	srv := cvFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.GetCV(context.Background(), testCV)
	if err != nil {
		t.Fatalf("GetCV: %v", err)
	}
	if !strings.Contains(string(data), `"document"`) {
		t.Errorf("cv data = %s", data)
	}
}

func TestClient_EditCV(t *testing.T) {
	srv := cvFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	body := json.RawMessage(`{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"Led migration"}]}`)
	data, err := c.EditCV(context.Background(), testCV, body)
	if err != nil {
		t.Fatalf("EditCV: %v", err)
	}
	if !strings.Contains(string(data), "Tailored") {
		t.Errorf("edit result = %s", data)
	}
}

func TestClient_RenderCV(t *testing.T) {
	srv := cvFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	pdf, err := c.RenderCV(context.Background(), testCV)
	if err != nil {
		t.Fatalf("RenderCV: %v", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF") {
		t.Errorf("render did not return a PDF: %q", pdf[:min(8, len(pdf))])
	}
}

func TestClient_RenderCV_unauthorized(t *testing.T) {
	srv := cvFakeAPI(t)
	c := New(srv.URL, "bad", srv.Client())
	_, err := c.RenderCV(context.Background(), testCV)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Errorf("RenderCV unauth err = %v, want APIError 401", err)
	}
}
