package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceIDIsCreatedOnceAndReused(t *testing.T) {
	// The server binds a session to a device id and stores it next to the
	// agent's resume cursor, which is only valid on the machine that produced
	// it. A device id that changed per run would strand every session.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	first, err := DeviceID()
	if err != nil {
		t.Fatalf("DeviceID: %v", err)
	}
	if first == "" {
		t.Fatal("device id must not be empty")
	}

	second, err := DeviceID()
	if err != nil {
		t.Fatalf("DeviceID (second call): %v", err)
	}
	if first != second {
		t.Fatalf("device id must be stable across calls: %q then %q", first, second)
	}
}

func TestDeviceIDFileIsOwnerOnly(t *testing.T) {
	// It is not a secret, but it lives beside creds.json and identifies the
	// machine; keep the whole directory owner-only rather than making an
	// exception nobody will remember.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if _, err := DeviceID(); err != nil {
		t.Fatalf("DeviceID: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, ".freehire", "runner-id"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("device id file mode = %o, want 600", perm)
	}
}

func TestDeviceIDRejectsGarbageAndRegenerates(t *testing.T) {
	// A truncated or hand-edited file must not become a device id: the server
	// namespaces it but still routes by it, so a blank or whitespace value
	// would produce a device nobody can address.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	path := filepath.Join(dir, ".freehire", "runner-id")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := DeviceID()
	if err != nil {
		t.Fatalf("DeviceID: %v", err)
	}
	if strings.TrimSpace(id) == "" {
		t.Fatal("a blank stored id must be replaced, not returned")
	}
}
