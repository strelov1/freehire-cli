package oauthlogin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRegisterClient(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/oauth/register" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s, want POST /api/v1/oauth/register", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "fhcl_test123"})
	}))
	defer srv.Close()

	clientID, err := registerClient(context.Background(), srv.Client(), srv.URL, "http://127.0.0.1:54321/callback")
	if err != nil {
		t.Fatalf("registerClient: %v", err)
	}
	if clientID != "fhcl_test123" {
		t.Errorf("clientID = %q, want fhcl_test123", clientID)
	}
	if gotBody["client_name"] != "freehire CLI" {
		t.Errorf("client_name = %v, want \"freehire CLI\"", gotBody["client_name"])
	}
	redirectURIs, _ := gotBody["redirect_uris"].([]any)
	if len(redirectURIs) != 1 || redirectURIs[0] != "http://127.0.0.1:54321/callback" {
		t.Errorf("redirect_uris = %v", gotBody["redirect_uris"])
	}
}

func TestRegisterClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := registerClient(context.Background(), srv.Client(), srv.URL, "http://127.0.0.1:1/callback"); err == nil {
		t.Error("expected an error for a 500 response")
	}
}

func TestExchangeCode(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/oauth/token" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s, want POST /api/v1/oauth/token", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fhm_test-access-token", "token_type": "Bearer", "expires_in": 2592000,
		})
	}))
	defer srv.Close()

	token, err := exchangeCode(context.Background(), srv.Client(), srv.URL, "fhcl_test123", "http://127.0.0.1:54321/callback", "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}
	if token != "fhm_test-access-token" {
		t.Errorf("token = %q, want fhm_test-access-token", token)
	}
	want := map[string]string{
		"grant_type": "authorization_code", "code": "the-code", "client_id": "fhcl_test123",
		"redirect_uri": "http://127.0.0.1:54321/callback", "code_verifier": "the-verifier",
	}
	for k, v := range want {
		if got := gotForm.Get(k); got != v {
			t.Errorf("form[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestExchangeCode_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "code is invalid, expired, or already used"})
	}))
	defer srv.Close()

	if _, err := exchangeCode(context.Background(), srv.Client(), srv.URL, "c", "r", "bad-code", "v"); err == nil {
		t.Error("expected an error for a rejected code")
	}
}
