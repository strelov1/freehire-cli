# Listings

The single source of copy for every directory freehire is listed in. Fill a form or open a
PR by copying from here — never by retyping from memory. Directories render whatever they
were given and never come back to check; copy that drifts stays drifted.

Design: [`docs/superpowers/specs/2026-08-16-catalog-listings-design.md`](superpowers/specs/2026-08-16-catalog-listings-design.md)

## Rules

- **Round scale figures down, hard.** The counts below are deliberate floors, not live
  numbers — a floor stays true as the catalogue grows, so a listing nobody revisits does not
  become a lie. The live figures are on [/open](https://freehire.me/open); cite that page
  anywhere a reader can click through.
  Re-check the floors here whenever a listing is added, and never edit them in a directory
  without editing them here first. This is not hypothetical: `llms.txt` still says
  "200,000+ companies" while the live figure is 294.7K.
- **No version numbers, no tool counts.** Those churn on every release and cannot be kept
  in sync across fourteen listings.
- **Capabilities, not implementation.** A directory of MCP servers already knows it is
  looking at an MCP server; spending the first thirty characters saying so wastes them.
- **Write the domain out: `freehire.me`, not "freehire".** Every listing is a place the
  name can be picked up — by a reader, a crawler, or a model quoting a directory. A bare
  product name is ambiguous and unsearchable; the domain is neither. Put it early in the
  string, since directories truncate from the right.
- Every listing links to **https://freehire.me/cli**. Change it here first if that ever moves.

---

## The MCP server

| Field | Value |
|---|---|
| registry name | `me.freehire/freehire` |
| display title | `freehire` |
| npm package | `freehire-mcp` |
| link | `https://freehire.me/cli` |
| repository | `https://github.com/strelov1/freehire-mcp` |
| license | MIT |

**Shortest — 66 chars.** For a directory's one-line description field (Smithery's, for
one). Drops the verbs and keeps the claim.

```
freehire.me: search 3.3M+ IT jobs from 294K company career boards.
```

**Short — 91 chars.** For `server.json` (hard cap 100) and any field that truncates.

```
freehire.me: search 3.3M+ IT jobs from 294K company boards, track applications, tailor CVs.
```

**Medium — for awesome lists, one line after the link.**

```
Search 3.3M+ IT jobs from any MCP host, over [freehire.me](https://freehire.me) — postings
crawled straight from 294K company career boards, with market-fit scoring, application
tracking and CV tailoring.
```

**Full — for submission forms with a description box.**

```
[freehire.me](https://freehire.me) aggregates IT job postings straight from company career
boards and other public sources — 3.3M+ open roles from 294K companies — normalizes them
into one schema, and tags each with stack, seniority, region and work mode. The freehire.me
MCP server puts that behind tools any MCP host can call: search and filter
the market, score a skill list against live demand, read a posting in full, apply, track
application stages, and tailor a CV against a specific job — all under one personal API
key. Search results carry each posting's full description as markdown, so a host can
screen a result set without a follow-up call per hit.

Free and open source. Works with Claude Desktop, Claude Code, and any MCP-compatible
agent, via npx — no global install.
```

**Keywords.** `mcp`, `jobs`, `job-search`, `hiring`, `ats`, `career`, `recruiting`, `cv`, `resume`

**Install snippet.**

````
```json
{
  "mcpServers": {
    "freehire": {
      "command": "npx",
      "args": ["-y", "freehire-mcp"],
      "env": { "FREEHIRE_TOKEN": "fhk_xxxxxxxx" }
    }
  }
}
```
````

---

## The Claude Code plugin

| Field | Value |
|---|---|
| plugin name | `freehire` — **immutable once published; renaming breaks every install** |
| marketplace | `strelov1/freehire-cli` |
| link | `https://freehire.me/cli` |
| repository | `https://github.com/strelov1/freehire-cli` |

**Short — 87 chars.**

```
Job search, application tracking, market fit and CV tailoring over the freehire.me API.
```

**Medium.**

```
Task-shaped agent skills over the [freehire.me](https://freehire.me) CLI — job search,
application tracking, market fit, CV tailoring and mail triage — plus the /job-search,
/market-fit, /tailor-cv, /track-applications and /triage-inbox commands.
```

**Full.**

```
Five skills and five slash commands that turn Claude Code into a job-hunting workspace on
top of [freehire.me](https://freehire.me), an open aggregator of IT postings pulled
straight from company career boards.

/job-search searches and filters the live market. /market-fit scores your skills against
real demand and names the gaps. /tailor-cv rewrites a CV against a specific posting, by
path, refusing claims your experience does not support. /track-applications keeps stages
and notes. /triage-inbox reads recruiter mail and sorts it.

Every command speaks --json, so output pipes into anything. One API key, shared with the
freehire.me CLI and MCP server.
```

**Keywords.** `jobs`, `job-search`, `career`, `cv`, `resume`, `ats`, `cli`, `productivity`

---

## The product (dev / AI-tool directories)

These directories list **freehire.me itself**. The audience is job seekers, not MCP users —
do not reuse the copy above.

| Field | Value |
|---|---|
| name | freehire |
| link | `https://freehire.me` |
| category | job search / career / developer tools |
| pricing | free, open source |

**Tagline.**

```
freehire.me — the open-source job search engine that reads company career boards directly.
```

**Short.**

```
freehire.me aggregates 3.3M+ IT jobs straight from 294K company career boards — no
reposts, no middlemen, no paywall. Search a normalized, deduplicated market by stack,
seniority, region and work mode. Free and open source, with a public API, a CLI and an MCP
server for agents.
```

**Full.**

```
Most job boards show you postings a recruiter chose to repost. freehire.me reads company
career boards directly — 3.3M+ open roles across 294K companies — normalizes what it finds
into one schema, deduplicates it, and tags every posting with stack, seniority, region and
work mode. What you search is the market, not a sales funnel.

Everything is free and open source. Job search, company data and facets are public and
need no account. Sign in and you also get saved jobs, application tracking with stages
and notes, a fit analysis against your CV, and CV tailoring against a specific posting.

Because the whole thing sits behind a public API, it is equally usable by a person and by
an agent: freehire.me ships a documented HTTP API, a Go CLI, an MCP server for Claude
Desktop and Claude Code, and a custom GPT.
```

**Keywords.** job search, developer jobs, IT jobs, remote jobs, open source, job aggregator,
career, ATS, AI job search

---

## Assets

| Asset | Use |
|---|---|
| `https://freehire.me/favicon.svg` | square vector mark — first choice wherever a directory takes a URL |
| `https://freehire.me/pwa-512x512.png` | square 512×512 PNG — for forms that reject SVG |
| `https://freehire.me/og.png` | rectangular OG card — social previews only, never as an icon |
| `freehire-mcp/assets/icon.png` | the same square mark, vendored for the MCPB bundle |
| screenshots | none prepared; needed by the product directories in [Phase 3](#the-product-dev--ai-tool-directories) |

The square assets are generated from the brand mark by `web/scripts/gen-pwa-icons.mjs` in
the site repo and committed there — they are not one-offs, and they stay correct if the
mark changes.

---

## Status

Mechanic: **push** = PR or CLI, done here · **form** = needs a logged-in human ·
**crawl** = indexes GitHub on its own, nothing to submit

### MCP directories

| Directory | Mechanic | Submitted | Listing |
|---|---|---|---|
| official MCP Registry | push | 2026-08-16 | `me.freehire/freehire` 0.4.2 — [live](https://registry.modelcontextprotocol.io/v0/servers?search=me.freehire/freehire) |
| awesome-mcp-servers | push | 2026-08-16 | [PR #12289](https://github.com/punkpeye/awesome-mcp-servers/pull/12289) — open; Glama badge added after the bot gated it |
| mcp.so | form | — | — |
| PulseMCP | form | — | — |
| Glama | crawl | indexed | https://glama.ai/mcp/servers/strelov1/freehire-mcp — score badge live |
| Smithery | push (`.mcpb`) | 2026-08-16 | [strelov1/freehire](https://smithery.ai/servers/strelov1/freehire) — published, **card still blank** (see below) |

### Claude Code directories

| Directory | Mechanic | Submitted | Listing |
|---|---|---|---|
| Anthropic plugin directory | form | — | — |
| ccplugins/awesome-claude-code-plugins | push | 2026-08-16 | [PR #352](https://github.com/ccplugins/awesome-claude-code-plugins/pull/352) — open |
| hesreallyhim/awesome-claude-code (52k★) | form | — | web issue form only |
| claudemarketplaces.com | crawl | — | — |

### Other agents

MCP is not Claude-only, and neither is the skill format. Every host below reads the same
`freehire-mcp` package — nothing new has to be built, only listed.

| Host | Directory | Mechanic | Submitted | Listing |
|---|---|---|---|---|
| Cline | [cline/mcp-marketplace](https://github.com/cline/mcp-marketplace) | push (issue) | 2026-08-16 | [#2254](https://github.com/cline/mcp-marketplace/issues/2254) — needs a 400×400 logo, which now lives at `freehire-mcp/assets/logo-400.png` |
| goose (52.9k★) | `documentation/static/servers.json` | push | 2026-08-16 | [PR #11280](https://github.com/aaif-goose/goose/pull/11280) |
| OpenClaw | [clawhub.ai](https://clawhub.ai) | form / CLI | — | `clawhub login` then `clawhub skill publish`; needs an account |
| Continue | Continue Hub | form | — | hub blocks; `hub.continue.dev` did not resolve — find the current URL first |
| Cursor, Windsurf, Codex, Gemini CLI | — | — | — | consume MCP directly from config; no directory to submit to |

Two corrections worth keeping: goose moved from `block/goose` to **`aaif-goose/goose`** under
the Linux Foundation, and ClawHub's live domain is **clawhub.ai** — `clawhub.io` serves an
expired certificate.

Cline requires proof that installation works from the README alone. It does: `npx -y
freehire-mcp` from a clean directory answers `initialize` and `tools/list` with no token
set, so setup cannot stall waiting on a credential.

### Skill directories

The plugin ships five skills, which makes it eligible for the skills lists as well as the
plugin ones — a separate ecosystem that grew after the SKILL.md format was opened up. These
lists take a **link**; the skills do not have to be copied into their repos.

| Directory | Stars | Mechanic | Submitted | Listing |
|---|---|---|---|---|
| ComposioHQ/awesome-claude-skills | 72.6k | push | 2026-08-16 | [PR #1650](https://github.com/ComposioHQ/awesome-claude-skills/pull/1650) — open |
| BehiSecc/awesome-claude-skills | 10.0k | push | 2026-08-16 | [PR #580](https://github.com/BehiSecc/awesome-claude-skills/pull/580) — open |
| jqueryscript/awesome-claude-code | 501 | push | 2026-08-16 | [PR #599](https://github.com/jqueryscript/awesome-claude-code/pull/599) — open |
| [skills.sh](https://skills.sh) | — | form | — | Vercel's directory; spans 19 agents, not just Claude |
| [cultofclaude.com](https://cultofclaude.com) | — | form | — | 4,248 skills indexed |
| [agentskill.sh](https://agentskill.sh) | — | form | — | — |
| travisvn/awesome-claude-skills | 14.7k | push | — | **stale** — no commits since 2026-04-28 |
| rohitg00/awesome-claude-code-toolkit | 2.5k | push | — | last pushed 2026-05-12 |

Checked and rejected: `acacess/awesome-jobs` (76★, untouched since 2022).

**Glama gates awesome-mcp-servers.** Glama does index from GitHub on its own — it found
freehire-mcp without a submission — but `awesome-mcp-servers` will not list a server until
Glama's checks pass, and asks for a Glama score badge in the entry. Those checks start the
server from a **Dockerfile** and speak MCP at it, which is why one now lives in
`freehire-mcp`. Introspection answers without a token, so a directory can run the check
without being handed a credential.

Order, if this repeats for another server: Dockerfile → let Glama index → add the badge to
the awesome-mcp-servers entry.

**Smithery, unfinished.** The bundle is accepted and installable, but the catalog card is
empty: description `""`, no icon, no homepage, `tools: 0`, and the name shown is `freehire`
rather than the manifest's `display_name`. Smithery does **not** read listing metadata from
the bundle manifest — even though the manifest inside the published `.mcpb` carries all of
it — and the CLI has no command to set it. Fill it in at
[the server's Smithery page](https://smithery.ai/servers/strelov1/freehire) using the copy
above; until then this listing works but sells nothing.

Also note Smithery validates the manifest's `tools` array more strictly than the MCPB spec
does: the spec allows `{name, description}`, Smithery rejects it with one
"expected object, received undefined" per tool. The manifest therefore sets
`tools_generated: true` and lets the server report its own tools.

`hesreallyhim/awesome-claude-code` takes **no pull requests** — recommendations go through
the web issue form only, submitting via `gh` is explicitly impossible, and the maintainer
asks that a human file it. It is also openly selective: the CONTRIBUTING file warns that
getting listed should not be part of a promotional plan. Treat acceptance as a maybe.
Entry criteria are met either way (repo created 2026-06-13, actively developed).

### Product directories

| Directory | Mechanic | Submitted | Listing |
|---|---|---|---|
| AlternativeTo | form | — | — |
| SaaSHub | form | — | — |
| There's An AI For That | form | — | — |
| Toolify | form | — | — |

### Owned surfaces

Not directories, but the highest-traffic listings we control outright — and the copy
crawl-based directories read.

| Surface | State |
|---|---|
| `strelov1/freehire` README (378★) | build-on row now links the CLI and the MCP server |
| `freehire.me/llms.txt` | names the MCP server |
| `freehire.me/mcp` | 308 → `/cli#mcp` |
| `strelov1/freehire-mcp` README | opens with the scale figures and links freehire.me |
| GitHub topics on `freehire-mcp` / `freehire-cli` | set 2026-08-16 (14 and 15 topics) — crawl-based directories read these |

### Job-search projects

Not directories — open-source tools that need a source of postings. The pitch is different:
they do not want a listing, they want an integration, so lead with the API and offer to write
the adapter rather than asking them to.

| Project | Stars | State |
|---|---|---|
| MadsLorentzen/ai-job-search | 31.9k | **accepted** — ships `.agents/skills/freehire-search/` with a CLI and tests |
| emredurukn/awesome-job-boards | 1.0k | **merged** — [PR #179](https://github.com/emredurukn/awesome-job-boards/pull/179) |
| tramcar/awesome-job-boards | 1.8k | [PR #284](https://github.com/tramcar/awesome-job-boards/pull/284) open since 2026-06-17, no reaction |
| Panniantong/Agent-Reach | 72.3k | [issue #637](https://github.com/Panniantong/Agent-Reach/issues/637) — proposed a zero-config jobs channel |
| DaKheera47/job-ops | 3.9k | [issue #708](https://github.com/DaKheera47/job-ops/issues/708) |
| speedyapply/JobSpy | 4.1k | [issue #381](https://github.com/speedyapply/JobSpy/issues/381) |
| santifer/career-ops | 64k | **closed — do not resubmit.** See below |

**career-ops is closed on purpose, and the reason is strategic.** The first attempt (#1082,
PR #1197) was rejected on policy: career-ops does not wire user data out to third parties,
from anyone. The re-scoped read-only version (#2350) was verified live by the maintainer —
he confirmed the API answers signed-out and the links point at the real ATS — and closed
anyway, because the offers-aggregation layer is reserved as first-party in their roadmap.
Nothing about freehire fixes that. Do not spend another round there.

The lesson generalizes: **a project that plans to build its own aggregator will not adopt
one.** Check the roadmap before writing the pitch.

Checked and rejected as a poor fit: `Paramchoudhary/ResumeSkills` (1.7k★) — its skills are
pure prompt files with no CLI or API calls anywhere, so a skill backed by a live API does
not belong there.

### Package managers

| Target | Mechanic | Submitted | Listing |
|---|---|---|---|
| Homebrew tap `strelov1/freehire` | push | 2026-08-16 | https://github.com/strelov1/homebrew-freehire — `brew install strelov1/freehire/freehire`, audit clean |
| npm `freehire-mcp` | push | 2026-07 | https://www.npmjs.com/package/freehire-mcp |

**Out of scope:** `homebrew-core` (gates on notability).

**Product Hunt is scheduled independently for 26 August 2026** — the banner is already live
on [/open](https://freehire.me/open). It is not part of this sweep, but it sets the
deadline: everything above should be listed *before* that date, so the launch lands on a
product already visible in the directories rather than one that shows up afterwards.
