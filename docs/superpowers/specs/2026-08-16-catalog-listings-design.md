# Listing freehire in the tool catalogs

**Date:** 2026-08-16
**Repos touched:** `freehire-cli`, `freehire-mcp`, `hire` (the site)

## The goal

Get the freehire MCP server and the freehire Claude Code plugin listed across the
directories an agent or a developer actually browses, with every listing carrying a
link back to freehire.me.

Fourteen directories — six MCP, four Claude Code, four product — plus a Homebrew tap,
all fed from one canonical copy.

## What exists today

| Artifact | State |
|---|---|
| `freehire-mcp` | published to npm as `freehire-mcp@0.4.1`; ~25 tools; stdio over `npx` |
| `freehire-cli` | Go binary, releases through `v0.18.0` with four platform assets; no CI |
| the plugin | `.claude-plugin/plugin.json` + 5 skills + 5 slash commands; never validated |
| the site (`hire`) | `/cli` carries a full MCP section with a `#mcp` anchor and `SoftwareApplication` JSON-LD |
| `/mcp` on the site | **404** |
| `web/static/llms.txt` | lists the GPT, the ChatGPT guide and the CLI — **does not mention MCP at all** |
| listings | none, anywhere |

## Decisions

These were settled during design and are not open questions:

- **Registry namespace: `me.freehire/freehire`**, authenticated by DNS. The brand is then
  in the server name the agent reads, not just in a metadata field. The alternative
  (`io.github.strelov1/freehire`) is one command cheaper and reads as a personal side
  project. The name cannot practically be changed later.
- **Listing links point at `https://freehire.me/cli`** until a dedicated `/mcp` page exists.
- **Nothing is submitted externally without an explicit go-ahead.** Manifests, copy,
  branches and draft PRs are prepared freely; every push outward is shown first.
- **Smithery is in scope**, via a locally-distributed `.mcpb` bundle.
- **Product Hunt is out of scope, and already scheduled: 26 August 2026** (the banner is
  live on `/open`). A launch cannot be replayed, so it is not folded into a bulk directory
  sweep. It does, however, set this work's deadline — the listings should be live *before*
  the launch, so it lands on a product the directories already know.
- **Scale figures are floors, not live numbers.** Counts appear in the copy, but rounded
  down hard, so a listing nobody revisits stays true as the catalogue grows. `llms.txt`
  already demonstrates the failure mode: it says "200,000+ companies" against a live 294.7K.
- **`homebrew-core` is out of scope.** It gates on notability; a submission now is a
  guaranteed rejection. A personal tap instead.

## Architecture

### One source of copy

`freehire-cli/docs/LISTINGS.md` is the single source for every listing, holding:

- the canonical name, title, and description at three lengths — **≤100 chars** (the hard
  cap the registry schema enforces), ~200 chars (awesome lists), and full prose (forms)
- keywords, the link target, the icon
- separate product-level copy for the dev/AI-tool catalogs, whose audience is job seekers
  rather than MCP users
- a status table: catalog → mechanic → submitted on → listing URL

Copy drift across directories is the failure mode this prevents. It is a documented,
observed problem in the ecosystem, not a hypothetical one — directories render stale
descriptions long after the source README moves on.

The file lives in `freehire-cli` because the work spans three repositories and needs one
home, and because that repo already carries the `docs/superpowers/` convention.

### Three catalog mechanics

Every catalog is one of three kinds, and the kind determines who acts:

- **push** — the registry, and PRs against awesome lists. Fully mine.
- **form** — mcp.so, PulseMCP, and Anthropic's plugin directory. I prepare the text; the
  submission needs your login.
- **crawl** — Glama, claudemarketplaces.com. There is nothing to submit; they index GitHub
  on their own. The lever is the repository README, which *is* the listing copy for these.

The third kind is why README quality is part of this work rather than adjacent to it.

## Phase 0 — foundation

Strictly ordered. Everything downstream depends on it.

1. **Write `docs/LISTINGS.md`** with the canonical copy. Approve the text before step 3 —
   after the npm publish, a typo costs a version bump.
2. **`server.json` in `freehire-mcp`** — schema `2025-12-11`; `name: me.freehire/freehire`;
   `websiteUrl: https://freehire.me/cli`; `repository`; `icons`; and a `packages` entry for
   `npm freehire-mcp@0.4.2`, declaring `FREEHIRE_TOKEN` as an optional secret env var
   (optional because the server also reads `~/.freehire/creds.json`).
3. **`mcpName: "me.freehire/freehire"` in `package.json`, then `npm publish` 0.4.2.**
   The registry reads `mcpName` from the *published* package, so the repo edit alone
   proves nothing. This is why the copy must be final first.
4. **DNS TXT record on Cloudflare** proving control of `freehire.me`, for the DNS login.
   The record is shown before it is added.
5. **Site edits in `hire`:**
   - `web/src/routes/mcp/+page.ts` — redirect `/mcp` → `/cli#mcp`. Needed because some
     directories strip anchors and store the bare path, which today 404s.
   - `web/static/llms.txt` — add the MCP server to *Use it from an assistant*. An agent
     reading llms.txt currently cannot discover that the MCP server exists.

### Icon

Resolved during implementation: the site already ships square brand assets, generated from
the brand mark by `web/scripts/gen-pwa-icons.mjs` and served over HTTPS —
`favicon.svg` and `pwa-512x512.png`. Both go in `server.json`; no new asset is needed, and
nothing has to be cropped from `og.png`.

## Phase 1 — MCP directories (`freehire-mcp`)

| Catalog | Mechanic | Notes |
|---|---|---|
| official MCP Registry | push | `mcp-publisher publish` after `login dns`; `--dry-run` first |
| awesome-mcp-servers (punkpeye) | push | PR; agent PRs fast-track with `🤖🤖🤖` in the title |
| mcp.so | form | submit button or a GitHub issue |
| PulseMCP | form | hand-reviewed |
| Glama | crawl | already indexed or will be; claim only |
| Smithery | push | see below |

**Smithery** is the only entry needing code rather than metadata. It takes either a hosted
HTTPS endpoint or a prebuilt `.mcpb` bundle; freehire has neither. The bundle is built from
the existing stdio server and published with the Smithery CLI. It becomes a new release
artifact that has to be kept in step with npm — the ongoing cost of this catalog.

## Phase 2 — Claude Code plugin (`freehire-cli`)

Gate: run `claude plugin validate` first. The plugin has never been validated, and
`plugin.json`'s `name` is immutable once published — a rename breaks every existing install.

| Catalog | Mechanic | Notes |
|---|---|---|
| `clau.de/plugin-directory-submission` | form | feeds both the official and community marketplaces; PRs opened directly against Anthropic's repos are auto-closed, so there is no push path |
| ccplugins/awesome-claude-code-plugins | push | PR |
| hesreallyhim/awesome-claude-code | push | PR |
| claudemarketplaces.com | crawl | claim at most |

## Phase 3 — dev / AI-tool catalogs

AlternativeTo, SaaSHub, There's An AI For That, Toolify.

These list **freehire.me the product**, not the CLI or the MCP server — their audience is
job seekers. Separate copy, kept in the same `LISTINGS.md`.

## Phase 4 — Homebrew

A personal tap, `strelov1/homebrew-freehire`: a formula pulling the four release binaries
from `v0.18.0`, with SHA256 computed from the published assets and
`homepage "https://freehire.me/cli"`.

**Known consequence, not solved here:** `freehire-cli` has no CI, so releases are built by
hand. The formula will therefore need a manual bump on every release, and a forgotten bump
means `brew install` silently serves a stale version. Automating the release is a separate
piece of work; this spec only notes the debt it creates.

## Out of scope

- Product Hunt (planned separately)
- `homebrew-core`
- a hosted remote MCP endpoint on freehire.me
- release automation for `freehire-cli`
- a dedicated `/mcp` landing page — the redirect covers the listing need; a real page is
  a later call

## How we know it worked

- `curl "https://registry.modelcontextprotocol.io/v0/servers?search=me.freehire/freehire"`
  returns the server with the correct `websiteUrl`
- `https://freehire.me/mcp` returns a redirect, not a 404
- `llms.txt` names the MCP server
- `claude plugin validate` passes clean
- `brew install strelov1/freehire/freehire` installs a working binary reporting `v0.18.0`
- every row in the `LISTINGS.md` status table has a live listing URL
- the description rendered by each directory matches `LISTINGS.md`
