package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := Creds{Token: "fhk_abc", APIURL: "https://example.test"}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".freehire", "creds.json"))
	if err != nil {
		t.Fatalf("stat creds: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("creds perms = %o, want 600", perm)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestLoadMissingIsZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Load()
	if err != nil {
		t.Fatalf("Load (missing): %v", err)
	}
	if got != (Creds{}) {
		t.Errorf("Load (missing) = %+v, want zero value", got)
	}
}

func TestResolvePrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save(Creds{Token: "file-token", APIURL: "https://file.test"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	env := map[string]string{EnvToken: "env-token", EnvAPIURL: "https://env.test"}
	got, err := Resolve(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Token != "env-token" || got.APIURL != "https://env.test" {
		t.Errorf("env should win: got %+v", got)
	}

	got, err = Resolve(func(string) string { return "" })
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Token != "file-token" || got.APIURL != "https://file.test" {
		t.Errorf("file fallback: got %+v", got)
	}
}

func TestResolveDefaultURLAndNoToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := Resolve(func(string) string { return "" }); !errors.Is(err, ErrNoToken) {
		t.Errorf("no token anywhere: err = %v, want ErrNoToken", err)
	}

	got, err := Resolve(func(k string) string {
		if k == EnvToken {
			return "fhk_x"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.APIURL != DefaultAPIURL {
		t.Errorf("APIURL = %q, want default %q", got.APIURL, DefaultAPIURL)
	}
}

func TestRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save(Creds{Token: "x"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got, _ := Load(); got != (Creds{}) {
		t.Errorf("after Remove, Load = %+v, want zero", got)
	}
	// Removing a missing file is not an error.
	if err := Remove(); err != nil {
		t.Errorf("Remove (missing): %v", err)
	}
}
