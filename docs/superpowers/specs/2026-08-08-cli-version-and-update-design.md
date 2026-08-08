# CLI version + self-update — design

## Problem

`freehire` has no way to report its own version, and no way to update itself.
Users install via `install.sh` (curl from the latest GitHub release) but have
no in-CLI path to check for or apply a newer release.

## Goals

- `freehire --version` / `-v` prints the running binary's version.
- `freehire update` checks GitHub for a newer release and installs it in place.
- `freehire update --check` reports whether an update is available without
  installing it.
- Both support `--json` for agent consumption, consistent with the rest of the
  CLI.

## Non-goals

- No changelog display, no rollback, no auto-update-on-startup.
- No checksum/signature verification of downloaded binaries (no checksums are
  currently published alongside releases).
- No semver ranges, pre-release channels, or `--version=vX.Y.Z` pinning for
  `update` — always installs the latest release.

## Version source

Go's `debug.ReadBuildInfo()` always reports `(devel)` for the main module
under a plain `go build`, even from a tagged git checkout — a real semver only
appears when installed via `go install module@vX.Y.Z`. Since this repo's
release binaries (in `dist/`) are built locally with plain `go build`, that
path won't produce a usable version string.

Instead: a package-level `var Version = "dev"` in `internal/cli`, overridden
at build time via `-ldflags "-X github.com/strelov1/freehire-cli/internal/cli.Version=vX.Y.Z"`.
Builds that skip this flag (e.g. `go run`, plain `go build` during
development) fall back to `"dev"`.

## Components

### 1. `internal/cli/version.go`

- `var Version = "dev"`.
- In `newRootCmd()`, set `root.Version = Version`. Cobra then wires `-v` /
  `--version` automatically and prints `freehire version <Version>` — no new
  subcommand needed (confirmed with the user: flag only, no `freehire version`
  subcommand).

### 2. `internal/selfupdate` (new package)

Self-contained; talks to the GitHub API, not the freehire API, so it does not
depend on `internal/client`.

- `type Release struct { Tag string; Assets map[string]string }` — asset name
  → download URL, populated from the GitHub API response.
- `func LatestRelease(ctx context.Context) (Release, error)` — `GET
  https://api.github.com/repos/strelov1/freehire-cli/releases/latest`, parses
  `tag_name` and `assets[].{name,browser_download_url}`.
- `func IsNewer(current, latest string) bool` — strips a leading `v` from
  both, splits on `.`, compares major/minor/patch as integers left to right.
  Malformed segments compare as 0. No new dependency (existing tags are plain
  `vMAJOR.MINOR.PATCH`).
- `func AssetName() string` — `fmt.Sprintf("freehire_%s_%s", runtime.GOOS,
  runtime.GOARCH)`, matching `install.sh`'s naming.
- `func Apply(ctx context.Context, downloadURL string) error`:
  1. Resolve the running binary's path via `os.Executable()`.
  2. Download `downloadURL` to a temp file in the same directory (so the
     final rename is same-filesystem).
  3. `chmod +x` the temp file.
  4. `os.Rename(tmp, execPath)` over the running binary. Overwriting a
     running executable's file is safe on Unix (the OS keeps the old inode
     for the current process; new invocations see the new file). Windows is
     not a supported OS per `install.sh` (`linux | darwin` only), so no
     rename-while-running workaround is needed there.
  5. On a permission error from either the temp-file write or the rename,
     return an error whose message tells the user to re-run with `sudo`
     (mirrors `install.sh`'s own sudo fallback for `/usr/local/bin`).

### 3. `internal/cli/update.go`

- `newUpdateCmd()`: `freehire update [--check]`.
- Flow:
  1. `selfupdate.LatestRelease(ctx)`.
  2. Compare `Version` (the package var from `version.go`) against the
     release tag via `IsNewer`. If `Version == "dev"`, still perform the
     comparison (an unstamped build is never "newer") but note in the
     human-readable output that the running build is unstamped, so the
     comparison may be inaccurate.
  3. `--check`: print current/latest and whether an update is available;
     exit 0. Never downloads.
  4. No `--check`: if already up to date, say so and exit 0. Otherwise find
     the asset for `AssetName()` in the release, call `selfupdate.Apply`,
     print a confirmation with the new version on success.
  5. `--json` output shape: `{"current": "v0.16.0", "latest": "v0.17.0",
     "updated": false}`. `updated` reflects whether `Apply` ran and
     succeeded (`--check` always reports `false`).
- Register `newUpdateCmd()` alongside the other subcommands in
  `newRootCmd()`.

### 4. `Makefile` (new)

- `build` target: for each of `darwin/amd64`, `darwin/arm64`,
  `linux/amd64`, `linux/arm64`, run
  `GOOS=$os GOARCH=$arch go build -ldflags "-X github.com/strelov1/freehire-cli/internal/cli.Version=$(git describe --tags --dirty --always)" -o dist/freehire_${os}_${arch} ./cmd/freehire`.
  This matches the asset names `install.sh` and `selfupdate.AssetName()`
  expect, and is the target the user will run in place of their current
  manual build step.

## Error handling

- Network failure reaching the GitHub API: return the error as-is (wrapped
  with context, e.g. `"checking latest release: %w"`); cobra's existing
  `SilenceUsage`/error-printing in `Execute()` handles display.
- No release asset matching the current OS/arch: explicit error naming the
  expected asset filename.
- Permission error writing/renaming the binary: explicit error suggesting
  `sudo freehire update`.

## Testing

- `selfupdate`: table-driven tests for `IsNewer` (equal, older, newer,
  malformed segments); `LatestRelease` against an `httptest.Server` stubbing
  the GitHub API shape; `Apply` against a temp dir standing in for the
  executable's directory (inject the exec path rather than calling
  `os.Executable()` directly, so the test doesn't touch the real test
  binary).
- `internal/cli`: `TestVersionFlag` asserting `--version`/`-v` output
  contains `Version`; `update.go` tests using an injected `selfupdate`
  interface (or a package-level var swapped in the test) to avoid real
  network calls, covering: up to date, update available + `--check` (no
  download), update available + apply, `--json` output shape.

## Open questions

None outstanding — all decisions above were confirmed during brainstorming.
