package runner

import (
	"strings"
	"testing"
)

func TestKnownHarnessResolvesToAFixedCommand(t *testing.T) {
	h, ok := LookupHarness("claude")
	if !ok {
		t.Fatal("claude must be a known harness")
	}
	if h.Command != "claude-code-acp" {
		t.Fatalf("command = %q, want claude-code-acp", h.Command)
	}
}

func TestUnknownHarnessIsRefused(t *testing.T) {
	// The whole point of the allowlist: the server picks from a set this
	// binary knows, it does not describe a process. Anything else must be
	// refused before a process exists.
	for _, name := range []string{
		"",
		"bash",
		"/bin/sh",
		"claude-code-acp",         // the binary name is not a harness name
		"claude; rm -rf ~",        // shell metacharacters
		"../../../usr/bin/whoami", // traversal
		"CLAUDE",                  // case must not be normalised into a match
		"claude ",                 // nor whitespace
	} {
		if _, ok := LookupHarness(name); ok {
			t.Errorf("LookupHarness(%q) must be refused", name)
		}
	}
}

func TestSupportedHarnessesAreAdvertisedForRegistration(t *testing.T) {
	// The server is told what this device can start; it must match what
	// LookupHarness will actually accept, or a spawn would be routed here and
	// then refused.
	names := SupportedHarnesses()
	if len(names) == 0 {
		t.Fatal("at least one harness must be advertised")
	}
	for _, n := range names {
		if _, ok := LookupHarness(n); !ok {
			t.Errorf("advertised harness %q is not resolvable", n)
		}
	}
}

func TestHarnessArgsAreOwnedByTheRunner(t *testing.T) {
	// A harness definition must be self-contained: nothing the server sends
	// contributes to argv. If this ever needs a parameter, it belongs in a
	// named field with a validated type, not in a free-form string.
	h, _ := LookupHarness("claude")
	for _, a := range h.Args {
		if strings.ContainsAny(a, ";|&$`<>\n") {
			t.Errorf("argument %q contains shell metacharacters", a)
		}
	}
}

func TestClaudeStripsTheAntiRecursionGuard(t *testing.T) {
	// claude-code-acp refuses to start when CLAUDECODE is set, and a developer
	// running the runner from inside Claude Code inherits it. Found exactly
	// that way: the harness died with "cannot be launched inside another
	// Claude Code session". The server's local spawn already strips it; so
	// must we.
	h, _ := LookupHarness("claude")
	found := false
	for _, k := range h.EnvRemove {
		if k == "CLAUDECODE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("claude must strip CLAUDECODE, got %v", h.EnvRemove)
	}
}
