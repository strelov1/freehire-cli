package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire-cli/internal/selfupdate"
)

// withFakeUpdate swaps latestRelease/applyUpdate for fakes for the duration
// of the test, restoring them on cleanup, and returns a pointer to the
// number of times the fake Apply ran.
func withFakeUpdate(t *testing.T, rel selfupdate.Release, relErr, applyErr error) *int {
	t.Helper()
	oldLatest, oldApply := latestRelease, applyUpdate
	calls := 0
	latestRelease = func(ctx context.Context) (selfupdate.Release, error) { return rel, relErr }
	applyUpdate = func(ctx context.Context, url string) error {
		calls++
		return applyErr
	}
	t.Cleanup(func() { latestRelease, applyUpdate = oldLatest, oldApply })
	return &calls
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "v0.16.0")
	calls := withFakeUpdate(t, selfupdate.Release{Tag: "v0.16.0"}, nil, nil)

	out, err := run(t, "update")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "up to date") {
		t.Errorf("output = %q, want an up-to-date message", out)
	}
	if *calls != 0 {
		t.Errorf("apply should not run when already up to date, got %d calls", *calls)
	}
}

func TestUpdateCheckDoesNotInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "v0.16.0")
	calls := withFakeUpdate(t, selfupdate.Release{
		Tag:    "v0.17.0",
		Assets: map[string]string{selfupdate.AssetName(): "https://example.test/asset"},
	}, nil, nil)

	out, err := run(t, "update", "--check")
	if err != nil {
		t.Fatalf("update --check: %v", err)
	}
	if !strings.Contains(out, "v0.16.0") || !strings.Contains(out, "v0.17.0") {
		t.Errorf("output = %q, want both versions named", out)
	}
	if *calls != 0 {
		t.Errorf("--check must not install, got %d calls", *calls)
	}
}

func TestUpdateInstallsWhenAvailable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "v0.16.0")
	calls := withFakeUpdate(t, selfupdate.Release{
		Tag:    "v0.17.0",
		Assets: map[string]string{selfupdate.AssetName(): "https://example.test/asset"},
	}, nil, nil)

	out, err := run(t, "update")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "Updated: v0.16.0 -> v0.17.0") {
		t.Errorf("output = %q", out)
	}
	if *calls != 1 {
		t.Errorf("apply should run exactly once, got %d calls", *calls)
	}
}

func TestUpdateMissingAssetErrorsWithoutInstalling(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "v0.16.0")
	calls := withFakeUpdate(t, selfupdate.Release{Tag: "v0.17.0", Assets: map[string]string{}}, nil, nil)

	if _, err := run(t, "update"); err == nil {
		t.Error("update with no matching release asset should error")
	}
	if *calls != 0 {
		t.Errorf("apply should not run without a matching asset, got %d calls", *calls)
	}
}

func TestUpdateDevBuildNotesItIsUnstamped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "dev")
	withFakeUpdate(t, selfupdate.Release{Tag: "v0.17.0"}, nil, nil)

	out, err := run(t, "update", "--check")
	if err != nil {
		t.Fatalf("update --check: %v", err)
	}
	if !strings.Contains(out, "unstamped") {
		t.Errorf("output = %q, want a note about the unstamped dev build", out)
	}
}

func TestUpdatePropagatesReleaseLookupError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	withFakeUpdate(t, selfupdate.Release{}, errors.New("network down"), nil)

	if _, err := run(t, "update"); err == nil {
		t.Error("update should surface a release-lookup failure")
	}
}

func TestUpdateJSONOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "v0.16.0")
	withFakeUpdate(t, selfupdate.Release{
		Tag:    "v0.17.0",
		Assets: map[string]string{selfupdate.AssetName(): "https://example.test/asset"},
	}, nil, nil)

	out, err := run(t, "--json", "update")
	if err != nil {
		t.Fatalf("update --json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("--json output is not JSON: %v (%q)", err, out)
	}
	if got["current"] != "v0.16.0" || got["latest"] != "v0.17.0" || got["updated"] != true {
		t.Errorf("json = %v, want current=v0.16.0 latest=v0.17.0 updated=true", got)
	}
}

func TestUpdateCheckJSONNeverSetsUpdated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "v0.16.0")
	withFakeUpdate(t, selfupdate.Release{
		Tag:    "v0.17.0",
		Assets: map[string]string{selfupdate.AssetName(): "https://example.test/asset"},
	}, nil, nil)

	out, err := run(t, "--json", "update", "--check")
	if err != nil {
		t.Fatalf("update --check --json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("--json output is not JSON: %v (%q)", err, out)
	}
	if got["updated"] != false {
		t.Errorf("updated = %v, want false", got["updated"])
	}
}
