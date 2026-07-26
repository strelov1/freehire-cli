// Package runner runs a coding harness on this machine on behalf of the
// freehire assistant, so the model-provider credential never leaves it.
//
// The server holds the session — journal, scheduling, UI — and reaches the
// harness here through a tunnel. What crosses that tunnel is the agent
// protocol, verbatim; what does not cross it is any say in *what* runs. The
// server names a harness from a set this runner knows; the runner alone decides
// which binary that is, with which arguments, under which confinement.
package runner

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirName        = ".freehire"
	deviceFileName = "runner-id"
)

// DevicePath returns the path of the stored device id (~/.freehire/runner-id).
func DevicePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dirName, deviceFileName), nil
}

// DeviceID returns this machine's device id, generating and storing one on
// first use.
//
// It must be stable across restarts: the server stores it alongside the
// session's resume cursor, and that cursor is only meaningful on the machine
// that produced it. An id that changed per run would leave every session
// pointing at a device that no longer exists.
//
// A stored value that is blank or whitespace is treated as absent and replaced —
// a truncated or hand-edited file should not become an unaddressable device.
func DeviceID() (string, error) {
	path, err := DevicePath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if id := strings.TrimSpace(string(b)); id != "" {
		return id, nil
	}

	id, err := newDeviceID()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func newDeviceID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
