package cli

import (
	"strings"
	"testing"
)

func TestVersionFlagPrintsStampedVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = old })

	out, err := run(t, "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !strings.Contains(out, "freehire version v9.9.9") {
		t.Errorf("--version output = %q, want it to contain %q", out, "freehire version v9.9.9")
	}
}

func TestVersionShortFlagPrintsStampedVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = old })

	out, err := run(t, "-v")
	if err != nil {
		t.Fatalf("-v: %v", err)
	}
	if !strings.Contains(out, "freehire version v9.9.9") {
		t.Errorf("-v output = %q, want it to contain %q", out, "freehire version v9.9.9")
	}
}
