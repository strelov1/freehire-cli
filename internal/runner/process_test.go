package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// echoHarness is a stand-in for a real harness: it reads lines and echoes them
// back, which is enough to exercise the plumbing without claude-code-acp.
func echoHarness() Harness {
	return Harness{
		Command: "sh",
		Args:    []string{"-c", `while IFS= read -r l; do echo "echo:$l"; done`},
	}
}

func TestProcessRoundTripsLines(t *testing.T) {
	p, err := startProcess(echoHarness(), nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Kill()

	if err := p.WriteLine(`{"jsonrpc":"2.0"}`); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case line := <-p.Lines():
		if line != `echo:{"jsonrpc":"2.0"}` {
			t.Fatalf("got %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no line came back from the process")
	}
}

func TestProcessClosesItsLinesWhenItExits(t *testing.T) {
	// The session layer ends a stream when this channel closes; if it stayed
	// open the server would wait on a turn that can never finish.
	p, err := startProcess(Harness{Command: "sh", Args: []string{"-c", "exit 3"}}, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case _, ok := <-p.Lines():
		if ok {
			t.Fatal("expected the channel to close, not to yield")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lines channel never closed after the process exited")
	}
	if code := p.Wait(); code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

func TestProcessAppliesEnvOnTopOfTheCurrent(t *testing.T) {
	// The harness needs the user's PATH and HOME to find its own tools and
	// credentials; the tunnel's env adds to that rather than replacing it.
	t.Setenv("RUNNER_TEST_MARKER", "inherited")
	p, err := startProcess(
		Harness{Command: "sh", Args: []string{"-c", `echo "$RUNNER_TEST_MARKER/$FREEHIRE_TOKEN"`}},
		map[string]string{"FREEHIRE_TOKEN": "sk-minted"},
	)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Kill()

	select {
	case line := <-p.Lines():
		if line != "inherited/sk-minted" {
			t.Fatalf("env not merged: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no output")
	}
}

func TestStartingAMissingBinaryFailsWithAnActionableReason(t *testing.T) {
	_, err := startProcess(Harness{Command: "definitely-not-a-real-binary-xyz"}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	// This string travels to the server and on to the user, so it has to name
	// the binary rather than just say "spawn failed".
	if !strings.Contains(err.Error(), "definitely-not-a-real-binary-xyz") {
		t.Fatalf("error should name the binary, got: %v", err)
	}
}

func TestKillStopsTheWholeProcessGroup(t *testing.T) {
	// claude-code-acp spawns children. Killing only the parent would leave
	// them holding the user's model quota with nobody reading their output.
	if runtime.GOOS == "windows" {
		t.Skip("process groups are POSIX-specific")
	}
	// The shell backgrounds a child that outlives it unless the group is
	// signalled, then writes the child's pid so the test can check.
	p, err := startProcess(Harness{
		Command: "sh",
		Args:    []string{"-c", `sleep 300 & echo $!; wait`},
	}, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var childPID string
	select {
	case childPID = <-p.Lines():
	case <-time.After(5 * time.Second):
		t.Fatal("no child pid")
	}

	p.Kill()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if !pidAlive(t, childPID) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %s survived Kill — the process group was not signalled", childPID)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// pidAlive reports whether the pid still exists. Signal 0 performs the
// permission and existence checks without delivering anything.
func pidAlive(t *testing.T, pid string) bool {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimSpace(pid))
	if err != nil {
		t.Fatalf("bad pid %q: %v", pid, err)
	}
	proc, err := os.FindProcess(n)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func TestStrippedVariablesDoNotReachTheHarness(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	p, err := startProcess(Harness{
		Command:   "sh",
		Args:      []string{"-c", `echo "[${CLAUDECODE:-unset}]"`},
		EnvRemove: []string{"CLAUDECODE"},
	}, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Kill()

	select {
	case line := <-p.Lines():
		if line != "[unset]" {
			t.Fatalf("CLAUDECODE reached the harness: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no output")
	}
}

func TestTheHarnessFindsTheSameFreehireTheRunnerIs(t *testing.T) {
	// The agent's only tool is `freehire`, resolved from PATH. A user with an
	// older copy earlier in PATH — a stale `go install` next to a fresh
	// install.sh, say — gets an agent whose CLI lacks half its subcommands,
	// and the failure reads as "the CLI has no cv command" rather than "you
	// have two binaries". Pin it: our own directory goes first.
	p, err := startProcess(Harness{
		Command: "sh",
		Args:    []string{"-c", `echo "$PATH"`},
	}, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Kill()

	self, err := os.Executable()
	if err != nil {
		t.Skip("no executable path in this environment")
	}
	want := filepath.Dir(self)

	select {
	case line := <-p.Lines():
		first := strings.SplitN(line, string(os.PathListSeparator), 2)[0]
		if first != want {
			t.Fatalf("harness PATH starts with %q, want the runner's own dir %q", first, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no output")
	}
}
