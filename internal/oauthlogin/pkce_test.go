package oauthlogin

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateVerifier(t *testing.T) {
	v1, err := generateVerifier()
	if err != nil {
		t.Fatalf("generateVerifier: %v", err)
	}
	if len(v1) < 43 {
		t.Errorf("verifier length = %d, want at least 43 (RFC 7636)", len(v1))
	}
	if strings.ContainsAny(v1, "+/=") {
		t.Errorf("verifier %q is not unpadded base64url", v1)
	}
	v2, err := generateVerifier()
	if err != nil {
		t.Fatalf("generateVerifier: %v", err)
	}
	if v1 == v2 {
		t.Error("two calls produced the same verifier")
	}
}

func TestChallengeFor(t *testing.T) {
	verifier := "a-high-entropy-verifier-the-client-generated-1234567890"
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])

	if got := challengeFor(verifier); got != want {
		t.Errorf("challengeFor(%q) = %q, want %q", verifier, got, want)
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := generateState()
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}
	s2, err := generateState()
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}
	if s1 == s2 {
		t.Error("two calls produced the same state")
	}
	if len(s1) < 16 {
		t.Errorf("state length = %d, want a reasonably unguessable length", len(s1))
	}
}
