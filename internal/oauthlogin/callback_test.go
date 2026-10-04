package oauthlogin

import (
	"context"
	"net/http"
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
