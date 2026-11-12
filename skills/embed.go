// Package skills bundles the agent skills into the binary, so
// `freehire skills install` works without a checkout of this repo.
package skills

import "embed"

// FS holds every skill as <name>/SKILL.md.
//
//go:embed */SKILL.md
var FS embed.FS
