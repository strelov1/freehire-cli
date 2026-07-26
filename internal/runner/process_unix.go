//go:build !windows

package runner

import (
	"errors"
	"os/exec"
	"syscall"
)

// setProcessGroup puts the harness in its own process group.
//
// claude-code-acp spawns children of its own. Signalling only the parent would
// leave them running with nobody reading their output and the user's model
// quota still ticking, so the group is what we create and later signal.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup signals the whole group. The negative pid is the POSIX way
// to address a group rather than a single process.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		// The group may already be gone, or we may not have created one;
		// fall back to the process itself rather than leaving it running.
		_ = cmd.Process.Kill()
	}
}

func asExitError(err error, target **exec.ExitError) bool {
	return errors.As(err, target)
}
