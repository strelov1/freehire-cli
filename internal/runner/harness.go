package runner

import "sort"

// Harness is a program this runner is willing to start, defined entirely here.
//
// The server names a harness; it never describes one. No field of it comes from
// the wire, so a compromised server cannot turn a session into arbitrary
// execution on this machine. Adding a harness is a release of this binary — a
// deliberate cost, paid so the set stays something the user can audit.
type Harness struct {
	// Command is the executable, looked up on PATH.
	Command string
	// Args is the fixed argument vector. Never extended from the wire.
	Args []string
	// Dir is where the process runs. Filled in per session by this runner, not
	// by the server: the server's workspace path does not exist on this
	// machine, and a harness handed one dies opening it.
	Dir string
	// EnvRemove are variables stripped from the inherited environment. The
	// harness runs with the user's env, which may contain things that make it
	// refuse to start.
	EnvRemove []string
}

// harnesses is the entire allowlist. Keys are the identifiers the server may
// use; nothing else resolves.
var harnesses = map[string]Harness{
	"claude": {
		Command: "claude-code-acp",
		// Claude Code refuses to start inside another Claude Code session,
		// and someone running this from a Claude Code terminal inherits the
		// marker. Stripping it is what the server's local spawn already does.
		EnvRemove: []string{"CLAUDECODE"},
	},
}

// LookupHarness resolves a server-supplied identifier. The match is exact: no
// trimming, no case folding, no path handling — those would each turn a
// near-miss into a match, and a near-miss here is an attack.
func LookupHarness(name string) (Harness, bool) {
	h, ok := harnesses[name]
	return h, ok
}

// SupportedHarnesses lists what this device advertises at registration, sorted
// for a stable handshake. It is derived from the same map LookupHarness reads,
// so the two cannot drift.
func SupportedHarnesses() []string {
	names := make([]string, 0, len(harnesses))
	for n := range harnesses {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
