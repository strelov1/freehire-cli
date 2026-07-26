//go:build windows

package runner

import (
	"errors"
	"os/exec"
)

// Windows has no process groups in the POSIX sense; the runner is not targeted
// at it, but keeping the build green costs two functions.
func setProcessGroup(*exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func asExitError(err error, target **exec.ExitError) bool {
	return errors.As(err, target)
}
