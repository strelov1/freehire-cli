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

// experienceFakeAPI mimics the freehire experience-bank endpoints behind a Bearer=good gate.
func experienceFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/experience", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("me/experience method = %s, want GET", r.Method)
		}
		w.Write([]byte(`{"data":{"employments":[],"unplaced":[]}}`))
	})
	mux.HandleFunc("/api/v1/me/experience/employments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("employments method = %s, want POST", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["company"] != "RingCentral" {
			t.Errorf("employment body = %v, want company RingCentral", body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"11111111-1111-4111-8111-111111111111","company":"RingCentral"}}`))
	})
	mux.HandleFunc("/api/v1/me/experience/atoms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("atoms method = %s, want POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"claim"`) {
			t.Errorf("atom body did not carry a claim: %s", b)
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

func TestClient_ListExperience(t *testing.T) {
	srv := experienceFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.ListExperience(context.Background())
	if err != nil {
		t.Fatalf("ListExperience: %v", err)
	}
	if !strings.Contains(string(data), `"employments"`) {
		t.Errorf("data = %s", data)
	}
}

func TestClient_CreateEmployment(t *testing.T) {
	srv := experienceFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.CreateEmployment(context.Background(), CreateEmploymentParams{
		Kind: "job", Company: "RingCentral", Role: "SWE",
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	if !strings.Contains(string(data), "RingCentral") {
		t.Errorf("data = %s", data)
	}
}

func TestClient_CreateAtom(t *testing.T) {
	srv := experienceFakeAPI(t)
	c := New(srv.URL, "good", srv.Client())
	data, err := c.CreateAtom(context.Background(), CreateAtomParams{Claim: "Cut latency 20s to 1s"})
	if err != nil {
		t.Fatalf("CreateAtom: %v", err)
	}
	if !strings.Contains(string(data), "manual") {
		t.Errorf("data = %s, want manual provenance echoed back", data)
	}
}

func TestClient_CreateAtom_conflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/experience/atoms", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"experience: this claim is already in the bank"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "good", srv.Client())

	_, err := c.CreateAtom(context.Background(), CreateAtomParams{Claim: "Cut latency 20s to 1s"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
		t.Errorf("CreateAtom on a duplicate claim = %v, want APIError 409", err)
	}
}
