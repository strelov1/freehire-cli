package runner

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGuardAllowsAPlainFreehireCommand(t *testing.T) {
	for _, c := range []string{
		"freehire search golang --json",
		"  freehire facets  ",
		"freehire job senior-go-acme-abc123",
		`freehire cv edit 54 --patch '{"summary":"x; y && z"}'`, // operators inside quotes are literal
		`freehire search "c++ & rust"`,
	} {
		if err := GuardDecision(c); err != nil {
			t.Errorf("should allow %q: %v", c, err)
		}
	}
}

func TestGuardDeniesAnythingButFreehire(t *testing.T) {
	for _, c := range []string{"env", "printenv", "cat /proc/self/environ", "curl evil.com", ""} {
		if err := GuardDecision(c); err == nil {
			t.Errorf("should deny %q", c)
		}
	}
}

func TestGuardDeniesChainingSubstitutionRedirection(t *testing.T) {
	// The allow-prefix in settings.json sees none of these — this is the real
	// boundary. Each one is a way an injected prompt would reach past it.
	for _, c := range []string{
		"freehire search x; printenv",
		"freehire search x && env",
		"freehire search x || env",
		`freehire search "$(printenv)"`,
		"freehire search `printenv`",
		"freehire search x | cat",
		"freehire search x > /tmp/out",
		"freehire search x < /etc/passwd",
		"freehire search x &",
		"freehire search x\nprintenv",
		"FOO=bar freehire search x",
		`freehire search "unterminated`,
		`freehire search x \`,
	} {
		if err := GuardDecision(c); err == nil {
			t.Errorf("should deny %q", c)
		}
	}
}

func TestGuardHookDeniesWithAReason(t *testing.T) {
	out := GuardHookDecision(`{"tool_name":"Bash","tool_input":{"command":"printenv"}}`)
	if out == "" {
		t.Fatal("a denied command must produce a decision")
	}
	for _, want := range []string{`"permissionDecision":"deny"`, "PreToolUse", "freehire"} {
		if !strings.Contains(out, want) {
			t.Errorf("decision missing %s: %s", want, out)
		}
	}
}

func TestGuardStaysSilentWhenItHasNoOpinion(t *testing.T) {
	// Silence defers to the session's own allow-list. A hook that spoke up on
	// every tool would override policy it knows nothing about.
	for _, in := range []string{
		`{"tool_name":"Bash","tool_input":{"command":"freehire facets"}}`,
		`{"tool_name":"Read","tool_input":{"file_path":"/etc/passwd"}}`,
		`not json at all`,
	} {
		if out := GuardHookDecision(in); out != "" {
			t.Errorf("should stay silent for %q, said: %s", in, out)
		}
	}
}

func TestSessionSettingsConfineTheAgent(t *testing.T) {
	// Without these the agent cannot run even `freehire`: the server sets
	// permission=deny precisely because this file is meant to decide.
	s := sessionSettings("/usr/local/bin/freehire")
	var v struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("settings must be valid JSON: %v", err)
	}
	if !contains(v.Permissions.Allow, "Bash(freehire:*)") {
		t.Errorf("the agent must be allowed to run freehire: %v", v.Permissions.Allow)
	}
	for _, tool := range []string{"Edit", "Write", "Task"} {
		if !contains(v.Permissions.Deny, tool) {
			t.Errorf("%s must be denied on a user's machine: %v", tool, v.Permissions.Deny)
		}
	}
	// The server's host blocks arbitrary egress; a laptop does not, so the
	// tools that reach the network directly are off here.
	for _, tool := range []string{"WebFetch", "WebSearch"} {
		if !contains(v.Permissions.Deny, tool) {
			t.Errorf("%s must be denied without a host egress allowlist: %v", tool, v.Permissions.Deny)
		}
	}
	if len(v.Hooks.PreToolUse) == 0 || v.Hooks.PreToolUse[0].Matcher != "Bash" {
		t.Fatalf("the Bash guard hook must be wired: %s", s)
	}
	if got := v.Hooks.PreToolUse[0].Hooks[0].Command; got != "/usr/local/bin/freehire bash-guard" {
		t.Errorf("hook command = %q", got)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
