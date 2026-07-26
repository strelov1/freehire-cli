package runner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// execProcess is a harness running as a child of this runner.
//
// Output is read line by line because that is the agent protocol's framing:
// one JSON object per line, which maps to one tunnel frame. Nothing here
// inspects the content — the runner is a pipe, and the server owns the
// conversation.
type execProcess struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string

	once sync.Once
	done chan struct{}
	code int
}

// startProcess launches a harness with env applied on top of the current
// environment.
//
// Merging rather than replacing is deliberate: the harness needs the user's
// PATH to find its own tools and HOME to find its credentials. The tunnel's env
// is a small allowlisted addition, not a sandbox.
func startProcess(h Harness, env map[string]string) (process, error) {
	cmd := exec.Command(h.Command, h.Args...)
	cmd.Dir = h.Dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// The harness's own diagnostics go to our stderr, where the user running
	// the runner can see them. They are not part of the protocol.
	cmd.Stderr = os.Stderr
	setProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		// Carries the binary name, because this reason travels to the server
		// and on to the user: "not found in $PATH" is actionable.
		return nil, fmt.Errorf("starting %s: %w", h.Command, err)
	}

	p := &execProcess{
		cmd:   cmd,
		stdin: stdin,
		lines: make(chan string, 32),
		done:  make(chan struct{}),
	}
	go p.readLines(stdout)
	return p, nil
}

func (p *execProcess) readLines(stdout io.ReadCloser) {
	sc := bufio.NewScanner(stdout)
	// Agent protocol lines can be large (a whole tool result), and the default
	// 64 KiB limit would silently truncate the conversation.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		p.lines <- sc.Text()
	}
	// Closing tells the session layer the harness is finished, which is what
	// lets the server end an in-flight turn instead of waiting forever.
	close(p.lines)

	err := p.cmd.Wait()
	p.code = exitCode(err)
	close(p.done)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok {
		return ee.ExitCode()
	}
	return -1
}

func (p *execProcess) WriteLine(s string) error {
	_, err := io.WriteString(p.stdin, s+"\n")
	return err
}

func (p *execProcess) Lines() <-chan string { return p.lines }

// Wait blocks until the harness has exited and returns its code.
func (p *execProcess) Wait() int {
	<-p.done
	return p.code
}

// Kill stops the harness and everything it started. Closing stdin first gives a
// well-behaved harness the chance to exit on its own; the signal covers the
// rest.
func (p *execProcess) Kill() {
	p.once.Do(func() {
		_ = p.stdin.Close()
		killProcessGroup(p.cmd)
	})
}
