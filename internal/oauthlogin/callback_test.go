package oauthlogin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCallbackServer_CapturesCodeAndState(t *testing.T) {
	cb, err := listenForCallback()
	if err != nil {
		t.Fatalf("listenForCallback: %v", err)
	}
	defer func() { _ = cb.Close() }()

	go func() {
		_, _ = http.Get(cb.redirectURI + "?code=the-code&state=the-state")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := cb.wait(ctx)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.code != "the-code" || result.state != "the-state" {
		t.Errorf("result = %+v, want code=the-code state=the-state", result)
	}
}

func TestCallbackServer_CapturesTheErrorParam(t *testing.T) {
	cb, err := listenForCallback()
	if err != nil {
		t.Fatalf("listenForCallback: %v", err)
	}
	defer func() { _ = cb.Close() }()

	go func() {
		_, _ = http.Get(cb.redirectURI + "?error=access_denied&state=the-state")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := cb.wait(ctx)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.errorParam != "access_denied" {
		t.Errorf("errorParam = %q, want access_denied", result.errorParam)
	}
}

func TestCallbackServer_TimesOutWithNoVisit(t *testing.T) {
	cb, err := listenForCallback()
	if err != nil {
		t.Fatalf("listenForCallback: %v", err)
	}
	defer func() { _ = cb.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := cb.wait(ctx); err == nil {
		t.Error("expected a timeout error when nothing visits the callback")
	}
}

func TestCallbackServer_RedirectURIIsLoopback(t *testing.T) {
	cb, err := listenForCallback()
	if err != nil {
		t.Fatalf("listenForCallback: %v", err)
	}
	defer func() { _ = cb.Close() }()

	if cb.redirectURI == "" {
		t.Fatal("redirectURI is empty")
	}
}

// A refusal must not land on a page telling the person sign-in is finishing.
func TestCallbackServer_PageMatchesTheOutcome(t *testing.T) {
	cb, err := listenForCallback()
	if err != nil {
		t.Fatalf("listenForCallback: %v", err)
	}
	defer func() { _ = cb.Close() }()

	for query, want := range map[string]string{
		"?code=c&state=s":              "Return to the terminal",
		"?error=access_denied&state=s": "Connection cancelled",
	} {
		resp, err := http.Get(cb.redirectURI + query)
		if err != nil {
			t.Fatalf("GET %s: %v", query, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if !strings.Contains(string(body), want) {
			t.Errorf("GET %s: page lacks %q", query, want)
		}
	}
}
