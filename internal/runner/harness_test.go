package runner

import (
	"strings"
	"testing"
)

func TestKnownHarnessesResolveToFixedCommands(t *testing.T) {
	// These mirror the server's AcpConfig constructors: a harness must behave
	// the same whether the server spawns it or this runner does.
	for name, want := range map[string]string{
		"claude":   "claude-code-acp",
		"gemini":   "gemini",
		"opencode": "opencode",
		"codex":    "codex-acp",
		"pi":       "pi-acp",
	} {
		h, ok := LookupHarness(name)
		if !ok {
			t.Errorf("%s must be a known harness", name)
			continue
		}
		if h.Command != want {
			t.Errorf("%s → %q, want %q", name, h.Command, want)
		}
	}
}

func TestHarnessesThatNeedFlagsCarryThem(t *testing.T) {
	// gemini needs --acp to speak the agent protocol at all, and opencode
	// needs its `acp` subcommand. Getting these wrong looks like a hung
	// handshake rather than a misconfiguration.
	if h, _ := LookupHarness("gemini"); len(h.Args) == 0 || h.Args[0] != "--acp" {
		t.Errorf("gemini must be launched with --acp, got %v", h.Args)
	}
	if h, _ := LookupHarness("opencode"); len(h.Args) == 0 || h.Args[0] != "acp" {
		t.Errorf("opencode must be launched with its acp subcommand, got %v", h.Args)
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
