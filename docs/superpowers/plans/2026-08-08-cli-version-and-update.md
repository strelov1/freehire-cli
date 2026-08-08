# CLI version + self-update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `freehire` a `--version`/`-v` flag and a `freehire update` command that checks GitHub for a newer release and installs it in place.

**Architecture:** A package-level `Version` var in `internal/cli`, wired to cobra's built-in version flag and stamped at build time via `-ldflags -X`. A new, self-contained `internal/selfupdate` package talks to the GitHub REST API and replaces the running binary; `internal/cli/update.go` is a thin cobra command over it. A new `Makefile` stamps `Version` for the four release platforms.

**Tech Stack:** Go 1.25, `github.com/spf13/cobra` (already a dependency — no new dependencies added), Go stdlib `net/http`/`encoding/json`/`os`.

**Spec:** `docs/superpowers/specs/2026-08-08-cli-version-and-update-design.md`

## Global Constraints

- Module path: `github.com/strelov1/freehire-cli` — all imports/ldflags paths below use it verbatim.
- No new third-party dependencies (spec: version comparison is hand-rolled major.minor.patch, no semver library).
- Supported OS/arch stays `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64` — matches `install.sh` and its `freehire_<os>_<arch>` asset naming. No Windows handling.
- No checksum/signature verification of downloaded binaries (no checksums are published today) — explicitly out of scope per spec.
- `--json` output, where present, must be the sole stdout content (existing convention: `printJSON`/`wantJSON` in `internal/cli/root.go`).
- Follow existing style: doc comments on every exported identifier explaining *why*, not just what; table-driven tests where the existing files already do (e.g. `TestTruncCountsRunesNotBytes` in `internal/cli/cli_test.go`).
- Test isolation: every CLI test that touches config must `t.Setenv("HOME", t.TempDir())` (see `internal/cli/cli_test.go`'s existing tests) so it can't read the real machine's `~/.freehire`.

---

### Task 1: `--version` / `-v` flag

**Files:**
- Create: `internal/cli/version.go`
- Modify: `internal/cli/root.go:26-35` (add `root.Version = Version`)
- Create: `internal/cli/version_test.go`

**Interfaces:**
- Produces: `var Version string` (package `cli`, default `"dev"`) — read by Task 4's `update.go` as the "current version" and written directly by tests to stub a stamped build.

- [ ] **Step 1: Write the failing tests**

`internal/cli/version_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

func TestVersionFlagPrintsStampedVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = old })

	out, err := run(t, "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !strings.Contains(out, "freehire version v9.9.9") {
		t.Errorf("--version output = %q, want it to contain %q", out, "freehire version v9.9.9")
	}
}

func TestVersionShortFlagPrintsStampedVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = old })

	out, err := run(t, "-v")
	if err != nil {
		t.Fatalf("-v: %v", err)
	}
	if !strings.Contains(out, "freehire version v9.9.9") {
		t.Errorf("-v output = %q, want it to contain %q", out, "freehire version v9.9.9")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/... -run TestVersion -v`
Expected: build failure — `undefined: Version` (the var doesn't exist yet).

- [ ] **Step 3: Add the `Version` var and wire it into the root command**

`internal/cli/version.go`:

```go
package cli

// Version is the CLI's version, stamped at build time via
// `-ldflags "-X github.com/strelov1/freehire-cli/internal/cli.Version=vX.Y.Z"`
// (see the Makefile's build target). debug.ReadBuildInfo() cannot do this job:
// it always reports "(devel)" for the main module under a plain `go build`,
// even from a tagged git checkout — a real semver there only appears via
// `go install module@vX.Y.Z`, which is not how release binaries are built
// here. A build that skips the ldflags — `go run`, a bare `go build` during
// development — reports "dev".
var Version = "dev"
```

Modify `internal/cli/root.go` — add `root.Version = Version` right after the
`root := &cobra.Command{...}` literal (so it lands before
`root.PersistentFlags()...`):

```go
	root := &cobra.Command{
		Use:   "freehire",
		Short: "Search and track jobs from the terminal via the freehire API",
		Long: "freehire is a CLI over the freehire API. Authenticate once with " +
			"`freehire auth login`, then search, open, and apply to jobs. Pass --json " +
			"for machine-readable output (handy for agents).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Version = Version
	root.PersistentFlags().Bool("json", false, "output raw JSON from the API")
```

Cobra's `InitDefaultVersionFlag` (called automatically on `Execute()`) sees a
non-empty `Version` and registers `--version` — and `-v`, since no existing
flag on this root command already claims that shorthand — printing
`freehire version <Version>` via its default template. No new subcommand is
added (confirmed with the user: flag only).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/... -run TestVersion -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/version.go internal/cli/root.go internal/cli/version_test.go
git commit -m "feat(cli): add --version/-v flag, stamped at build time"
```

---

### Task 2: `internal/selfupdate` — fetch the latest GitHub release, compare versions

**Files:**
- Create: `internal/selfupdate/selfupdate.go`
- Create: `internal/selfupdate/selfupdate_test.go`

**Interfaces:**
- Consumes: nothing from this repo (talks directly to `api.github.com`).
- Produces:
  - `type Release struct { Tag string; Assets map[string]string }` — `Assets` keyed by asset filename (e.g. `"freehire_darwin_arm64"`), valued by its download URL.
  - `func LatestRelease(ctx context.Context) (Release, error)`
  - `func IsNewer(current, latest string) bool`
  - Both are consumed by Task 4's `internal/cli/update.go`.

- [ ] **Step 1: Write the failing tests**

`internal/selfupdate/selfupdate_test.go`:

```go
package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.16.0", "v0.16.0", false},
		{"v0.16.0", "v0.15.9", false},
		{"v0.16.0", "v0.17.0", true},
		{"v0.16.0", "v1.0.0", true},
		{"v0.9.2", "v0.9.10", true}, // numeric compare, not lexical ("10" < "9" as strings)
		{"dev", "v0.1.0", true},     // an unstamped build never wins
		{"v0.16.0", "not-a-version", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func withFakeGitHubAPI(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	old := repoAPIURL
	repoAPIURL = srv.URL
	t.Cleanup(func() { repoAPIURL = old })
	return srv
}

func TestLatestReleaseParsesTagAndAssets(t *testing.T) {
	withFakeGitHubAPI(t, `{"tag_name":"v0.17.0","assets":[
		{"name":"freehire_darwin_arm64","browser_download_url":"https://example.test/freehire_darwin_arm64"},
		{"name":"freehire_linux_amd64","browser_download_url":"https://example.test/freehire_linux_amd64"}
	]}`, http.StatusOK)

	rel, err := LatestRelease(context.Background())
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if rel.Tag != "v0.17.0" {
		t.Errorf("Tag = %q, want v0.17.0", rel.Tag)
	}
	if rel.Assets["freehire_darwin_arm64"] != "https://example.test/freehire_darwin_arm64" {
		t.Errorf("asset URL = %q", rel.Assets["freehire_darwin_arm64"])
	}
}

func TestLatestReleaseSurfacesNon200(t *testing.T) {
	withFakeGitHubAPI(t, "", http.StatusNotFound)

	if _, err := LatestRelease(context.Background()); err == nil {
		t.Error("LatestRelease with a 404 should error")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/selfupdate/... -v`
Expected: build failure — package `internal/selfupdate` does not exist yet.

- [ ] **Step 3: Implement**

`internal/selfupdate/selfupdate.go`:

```go
// Package selfupdate checks GitHub for newer freehire-cli releases and
// installs them over the running binary. It talks to the GitHub REST API
// directly, not the freehire API, so it has no dependency on internal/client.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// repoAPIURL is the GitHub API endpoint for this repo's latest release.
// A var, not a const, so tests can point it at an httptest.Server.
var repoAPIURL = "https://api.github.com/repos/strelov1/freehire-cli/releases/latest"

// Release is a GitHub release: its tag, and its assets by filename.
type Release struct {
	Tag    string
	Assets map[string]string // asset filename -> download URL
}

// ghAsset and ghRelease decode the subset of the GitHub releases API
// response this package needs.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// LatestRelease fetches this repo's latest GitHub release.
func LatestRelease(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, repoAPIURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("checking latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("checking latest release: unexpected status %d", resp.StatusCode)
	}

	var gh ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return Release{}, fmt.Errorf("checking latest release: %w", err)
	}

	assets := make(map[string]string, len(gh.Assets))
	for _, a := range gh.Assets {
		assets[a.Name] = a.BrowserDownloadURL
	}
	return Release{Tag: gh.TagName, Assets: assets}, nil
}

// IsNewer reports whether latest is a newer version than current. Both are
// read as vMAJOR.MINOR.PATCH (a leading "v" is stripped from each) and
// compared numerically component by component — a plain string compare would
// rank "v0.9.10" below "v0.9.2". A non-numeric or missing component reads as
// 0, so a malformed tag never outranks a well-formed one.
func IsNewer(current, latest string) bool {
	c := versionParts(current)
	l := versionParts(latest)
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func versionParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	fields := strings.SplitN(v, ".", 3)
	var parts [3]int
	for i := 0; i < len(fields) && i < 3; i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			continue // leaves parts[i] at its zero value
		}
		parts[i] = n
	}
	return parts
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/selfupdate/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/selfupdate.go internal/selfupdate/selfupdate_test.go
git commit -m "feat(selfupdate): fetch the latest GitHub release and compare versions"
```

---

### Task 3: `internal/selfupdate` — download and install over the running binary

**Files:**
- Modify: `internal/selfupdate/selfupdate.go` (add imports + `AssetName`, `Apply`, `apply`, `permissionHint`)
- Modify: `internal/selfupdate/selfupdate_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `func AssetName() string` — consumed by Task 4 to look up `Release.Assets[AssetName()]`.
  - `func Apply(ctx context.Context, downloadURL string) error` — consumed by Task 4 (or a test double of the same signature).

- [ ] **Step 1: Write the failing tests**

Append to `internal/selfupdate/selfupdate_test.go` (add `"fmt"`, `"os"`,
`"path/filepath"`, `"runtime"`, `"strings"` to the import block):

```go
func TestAssetNameMatchesRuntime(t *testing.T) {
	want := fmt.Sprintf("freehire_%s_%s", runtime.GOOS, runtime.GOARCH)
	if got := AssetName(); got != want {
		t.Errorf("AssetName() = %q, want %q", got, want)
	}
}

func TestApplyReplacesTheBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new binary contents"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	execPath := filepath.Join(dir, "freehire")
	if err := os.WriteFile(execPath, []byte("old binary contents"), 0o755); err != nil {
		t.Fatalf("seed exec: %v", err)
	}

	if err := apply(context.Background(), execPath, srv.URL); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("read updated binary: %v", err)
	}
	if string(got) != "new binary contents" {
		t.Errorf("binary contents = %q, want %q", got, "new binary contents")
	}
	info, err := os.Stat(execPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("updated binary is not executable: mode %v", info.Mode())
	}
}

func TestApplySurfacesPermissionErrorWithHint(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission checks never fail")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new binary contents"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	execPath := filepath.Join(dir, "freehire")
	if err := os.WriteFile(execPath, []byte("old binary contents"), 0o755); err != nil {
		t.Fatalf("seed exec: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { // read+execute, no write
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) }) // let t.TempDir's own cleanup remove it

	err := apply(context.Background(), execPath, srv.URL)
	if err == nil {
		t.Fatal("apply into a read-only dir should error")
	}
	if !strings.Contains(err.Error(), "sudo") {
		t.Errorf("error = %q, want a sudo hint", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/selfupdate/... -run 'TestAssetName|TestApply' -v`
Expected: build failure — `undefined: AssetName`, `undefined: apply`.

- [ ] **Step 3: Implement**

Add to the import block in `internal/selfupdate/selfupdate.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)
```

Append to `internal/selfupdate/selfupdate.go`:

```go
// AssetName is the release asset this OS/arch expects, matching install.sh's
// naming (freehire_<os>_<arch>).
func AssetName() string {
	return fmt.Sprintf("freehire_%s_%s", runtime.GOOS, runtime.GOARCH)
}

// Apply downloads downloadURL and replaces the running binary with it.
func Apply(ctx context.Context, downloadURL string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return apply(ctx, exe, downloadURL)
}

// apply is Apply with the executable's path passed in explicitly, so tests
// can point it at a temp file instead of overwriting the real test binary.
func apply(ctx context.Context, execPath, downloadURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading update: unexpected status %d", resp.StatusCode)
	}

	// The temp file lives beside execPath so the final rename is same-filesystem
	// (a cross-device os.Rename fails outright).
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".freehire-update-*")
	if err != nil {
		return permissionHint(err, dir)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing update: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing update: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	// Overwriting a running executable's file is safe on the OSes install.sh
	// supports (linux, darwin): the OS keeps the old inode mapped to the
	// current process, and new invocations see the new file.
	if err := os.Rename(tmpPath, execPath); err != nil {
		return permissionHint(err, execPath)
	}
	return nil
}

// permissionHint wraps a permission error with a suggestion to re-run with
// sudo, mirroring install.sh's own fallback for an unwritable install dir.
func permissionHint(err error, path string) error {
	if os.IsPermission(err) {
		return fmt.Errorf("%w — re-run with sudo (freehire is installed at %s)", err, path)
	}
	return err
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/selfupdate/... -v`
Expected: PASS (all tests in the package, including Task 2's)

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/selfupdate.go internal/selfupdate/selfupdate_test.go
git commit -m "feat(selfupdate): download and install a release over the running binary"
```

---

### Task 4: `freehire update` command

**Files:**
- Create: `internal/cli/update.go`
- Modify: `internal/cli/root.go:38-42` (register `newUpdateCmd()`)
- Create: `internal/cli/update_test.go`

**Interfaces:**
- Consumes: `Version` (Task 1), `selfupdate.Release`, `selfupdate.LatestRelease`, `selfupdate.IsNewer`, `selfupdate.AssetName`, `selfupdate.Apply` (Tasks 2–3), `wantJSON`/`printJSON` (existing, `internal/cli/root.go`).
- Produces: `newUpdateCmd() *cobra.Command`, registered on root as `update`.

- [ ] **Step 1: Write the failing tests**

`internal/cli/update_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/... -run TestUpdate -v`
Expected: build failure — `undefined: latestRelease`, `undefined: applyUpdate` (the package doesn't compile yet, so no test actually runs).

- [ ] **Step 3: Implement**

`internal/cli/update.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/selfupdate"
)

// latestRelease and applyUpdate are package vars, not direct calls to
// selfupdate.LatestRelease/selfupdate.Apply, so tests can swap in fakes and
// never hit the network or overwrite a real binary.
var (
	latestRelease = selfupdate.LatestRelease
	applyUpdate   = selfupdate.Apply
)

// newUpdateCmd checks GitHub for a newer freehire release and, unless
// --check is set, downloads and installs it over the running binary.
func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update freehire to the latest release",
		Long: "Checks the latest GitHub release and, unless --check is set, downloads " +
			"and installs it in place of the running binary.",
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check")
			current := Version

			rel, err := latestRelease(cmd.Context())
			if err != nil {
				return fmt.Errorf("checking latest release: %w", err)
			}
			available := selfupdate.IsNewer(current, rel.Tag)
			updated := false

			if available && !checkOnly {
				url, ok := rel.Assets[selfupdate.AssetName()]
				if !ok {
					return fmt.Errorf("no release asset named %s in %s", selfupdate.AssetName(), rel.Tag)
				}
				if err := applyUpdate(cmd.Context(), url); err != nil {
					return err
				}
				updated = true
			}

			if wantJSON(cmd) {
				data, err := json.Marshal(map[string]any{
					"current": current,
					"latest":  rel.Tag,
					"updated": updated,
				})
				if err != nil {
					return err
				}
				printJSON(cmd, data)
				return nil
			}

			if current == "dev" {
				fmt.Fprintln(cmd.OutOrStdout(),
					"Note: this is an unstamped dev build; the version comparison below may be inaccurate.")
			}
			switch {
			case updated:
				fmt.Fprintf(cmd.OutOrStdout(), "Updated: %s -> %s\n", current, rel.Tag)
			case !available:
				fmt.Fprintf(cmd.OutOrStdout(), "Already up to date (%s).\n", current)
			default: // available && checkOnly
				fmt.Fprintf(cmd.OutOrStdout(),
					"Update available: %s -> %s. Run `freehire update` to install.\n", current, rel.Tag)
			}
			return nil
		},
	}
	cmd.Flags().Bool("check", false, "only check for an update, don't install it")
	return cmd
}
```

Modify `internal/cli/root.go`'s `AddCommand` call (lines 38-42) to add
`newUpdateCmd()`:

```go
	root.AddCommand(newAuthCmd(), newSearchCmd(), newJobCmd(), newApplyCmd(),
		newSaveCmd(), newUnsaveCmd(), newMyCmd(), newStageCmd(), newNoteCmd(),
		newCompanyCmd(), newJobsCmd(), newContributeCmd(), newContributionsCmd(), newSubmissionsCmd(),
		newMarketFitCmd(), newFacetsCmd(), newCVCmd(), newProfileCmd(),
		newInboxCmd(), newGhostCmd(), newExperienceCmd(), newUpdateCmd())
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/... -run TestUpdate -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS (nothing in Tasks 1–3 should have regressed)

- [ ] **Step 6: Commit**

```bash
git add internal/cli/update.go internal/cli/update_test.go internal/cli/root.go
git commit -m "feat(cli): add freehire update, checking and installing GitHub releases"
```

---

### Task 5: `Makefile` build target

**Files:**
- Create: `Makefile`

**Interfaces:**
- Consumes: `internal/cli.Version` (Task 1) by import path, via `-ldflags -X`.
- Produces: `make build`, populating `dist/freehire_<os>_<arch>` for the four supported platforms (already `.gitignore`d).

This task has no Go tests — a `Makefile` isn't Go code, and the CLI's
behavior isn't changing. It's verified by running the build and checking the
version stamp lands.

- [ ] **Step 1: Write the Makefile**

`Makefile`:

```makefile
BINARY := freehire
DIST := dist
VERSION := $(shell git describe --tags --dirty --always)
LDFLAGS := -X github.com/strelov1/freehire-cli/internal/cli.Version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: build
build:
	@mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "building $(DIST)/$(BINARY)_$${os}_$${arch}"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o $(DIST)/$(BINARY)_$${os}_$${arch} ./cmd/$(BINARY); \
	done
```

- [ ] **Step 2: Run it and verify the version stamp**

```bash
make build
GOOS=$(go env GOOS); GOARCH=$(go env GOARCH)
./dist/freehire_${GOOS}_${GOARCH} --version
```

Expected: prints `freehire version <git describe output>` — e.g.
`freehire version v0.16.0` on a clean checkout of that tag, or
`freehire version v0.16.0-3-gabc1234` / `...-dirty` otherwise. It must NOT
print `freehire version dev`.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add make build, stamping CLI version via ldflags"
```

---

## Post-plan note

`dist/` already holds four binaries built by the user's previous manual
process (git-ignored, so they're untouched by this plan). After Task 5 lands,
the user's next release build should go through `make build` instead, so the
binaries `install.sh` and `freehire update` serve actually carry a real
version rather than reporting `dev` to `IsNewer`.
