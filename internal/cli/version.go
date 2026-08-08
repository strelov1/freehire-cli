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
