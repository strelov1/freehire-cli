package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// The Bash guard for a locally-run assistant session.
//
// The session's settings.json pre-approves `Bash(freehire:*)`, but Claude
// Code's allow-prefix match is not a security boundary: it does not constrain
// what runs inside `$(…)`, after `;`/`&&`, through a pipe, or via redirection.
// This guard is the boundary — a PreToolUse hook that denies any Bash command
// which is not a single clean `freehire …` invocation.
//
// It matters more here than on the server. freehire's own host restricts what
// an agent can reach on the network; on a user's laptop there is no such layer,
// and the agent reads untrusted text (job postings) for a living. This is a
// port of the server's `roy bash-guard`, deliberately identical in behaviour —
// two implementations of one rule is already one too many, so it must not
// drift.

// unquotedForbidden are metacharacters that enable chaining, substitution,
// redirection or backgrounding when unquoted. A freehire-only shell needs none
// of them there. Inside quotes they are ordinary argument text — a JSON patch,
// a query like `c++ & rust` — so the scan is quote-aware rather than a blunt
// character ban.
const unquotedForbidden = ";&|<>(){}$`"

// doubleQuotedForbidden stay dangerous inside double quotes, where the shell
// still expands `$…` and “ `…` “. Single quotes suppress all of it.
const doubleQuotedForbidden = "$`"

// GuardDecision reports whether a Bash command may run: nil to allow.
func GuardDecision(command string) error {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return fmt.Errorf("empty command")
	}
	if err := scanQuoting(cmd); err != nil {
		return err
	}
	// The program token never needs quoting, so a plain split names it.
	// scanQuoting has already rejected every injection route.
	prog := strings.Fields(cmd)[0]
	if prog != "freehire" {
		return fmt.Errorf("only the `freehire` CLI may run in this session, got `%s`", prog)
	}
	return nil
}

// scanQuoting walks the command tracking quote state, rejecting any operator
// active in the context it appears in, an unterminated quote, a dangling
// escape, or a raw newline.
func scanQuoting(cmd string) error {
	const (
		none = iota
		single
		double
	)
	quote := none
	escaped := false

	for _, c := range cmd {
		if c == '\n' || c == '\r' {
			return fmt.Errorf("newline is not allowed — only a single `freehire …` command may run")
		}
		switch quote {
		case single:
			// Everything inside single quotes is literal.
			if c == '\'' {
				quote = none
			}
		case double:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				quote = none
			case strings.ContainsRune(doubleQuotedForbidden, c):
				return forbidden(c)
			}
		default:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '\'':
				quote = single
			case c == '"':
				quote = double
			case strings.ContainsRune(unquotedForbidden, c):
				return forbidden(c)
			}
		}
	}
	if quote != none {
		return fmt.Errorf("unterminated quote — quote the whole argument")
	}
	if escaped {
		return fmt.Errorf("dangling backslash is not allowed")
	}
	return nil
}

func forbidden(c rune) error {
	return fmt.Errorf(
		"shell metacharacter '%c' is not allowed here — only a single `freehire …` command may run", c)
}

// GuardHookDecision turns a PreToolUse payload into a deny decision, or "" to
// stay silent.
//
// Silence defers to the session's own allow-list, which is why a non-Bash tool,
// unparseable input and a clean command all produce nothing: a hook that spoke
// up on every tool would override policy it knows nothing about.
func GuardHookDecision(stdinJSON string) string {
	var v struct {
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal([]byte(stdinJSON), &v); err != nil {
		return ""
	}
	if v.ToolName != "Bash" {
		return ""
	}
	err := GuardDecision(v.ToolInput.Command)
	if err == nil {
		return ""
	}
	out, _ := json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": err.Error(),
		},
	})
	return string(out)
}

// RunGuard is the `freehire bash-guard` entry point: read the hook payload,
// print a deny decision if there is one, and always succeed. A hook that exits
// non-zero is treated as non-blocking, so a failure here must not become an
// accidental allow-all.
func RunGuard(stdin io.Reader, stdout io.Writer) {
	payload, err := io.ReadAll(stdin)
	if err != nil {
		return
	}
	if out := GuardHookDecision(string(payload)); out != "" {
		fmt.Fprintln(stdout, out)
	}
}

// sessionSettings is the Claude Code project config written into a session's
// working directory. It mirrors what the server writes for a server-hosted
// session, with one deliberate difference.
//
// The server keeps WebFetch and WebSearch: its host allowlists outbound
// traffic, so even a successful prompt injection reaches nothing. A user's
// laptop has no such layer, so those tools are denied here — the compensating
// control the spec calls for. `freehire` remains the only way out.
//
// This file is why the server sets permission=deny for the session: policy is
// decided here, by the machine the agent runs on, not by whoever asked for it.
func sessionSettings(freehireBin string) string {
	out, _ := json.Marshal(map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Bash(freehire:*)", "Skill", "Read(.claude/skills/**)"},
			"deny": []string{
				"Edit", "Write", "MultiEdit", "NotebookEdit",
				"Task", "Glob", "Grep", "LS",
				"WebFetch", "WebSearch",
			},
		},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": freehireBin + " bash-guard"},
					},
				},
			},
		},
	})
	return string(out)
}
