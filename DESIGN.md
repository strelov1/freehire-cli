# freehire-cli — design

A small Go CLI that lets agents (and humans) use the [freehire](https://freehire.me)
API without a browser, authenticating with a personal API key. The key is stored
in `~/.freehire/creds.json`.

## Goals / Non-Goals

**Goals:** a single static binary (`freehire`) any agent can run; authenticate
with an `fhk_…` API key; the core agent loop — `search` jobs (with facet
filters), fetch a job's content (`job`), browse a `company`, and `apply`; track
applications (`save`, `stage`, `note`, `my`); persist the token at
`~/.freehire/creds.json`; machine-readable `--json` output for agents.

**Non-Goals (YAGNI):** shell-completion docs; goreleaser/homebrew;
semantic-search flags; client-side stage validation (the API is the source of
truth — the CLI only lists the vocabulary in help).

## Layout

```
cmd/freehire/main.go    # thin entry: cli.Execute() (so `go install` yields `freehire`)
internal/
  config/   creds.json load/save (0600) + env/default resolution
  client/   API client over net/http: Me, Search, GetJob, GetCompany, Apply,
            Save, Unsave, MyJobs, Track (+ APIError)
  cli/      cobra commands: root, auth (login/status/logout), search, job,
            company, apply, save, unsave, stage, note, my
DESIGN.md, README.md, .github/workflows/ci.yml
```

## `runner` — running the assistant's harness locally (BYOK)

`freehire runner` connects this machine to the freehire assistant and runs its
coding harness here, so the user's model-provider credential never reaches
freehire's servers. The session, its journal and the UI stay on the server; only
the harness process is local.

```
this machine                              agent.freehire.dev
┌──────────────────────┐                 ┌────────────────────────────┐
│ freehire runner      │─ WS /runners/ws▶│ authenticates, bridges to   │
│  claude-code-acp     │                 │ the session daemon          │
│  your credentials    │◀── agent ───────│ journal, scheduling, UI     │
└──────────────────────┘    protocol     └────────────────────────────┘
```

**Getting in.** The runner exchanges the stored `fhk_…` key for a short-lived
session token (`POST /auth/runner-token`), which the server issues after asking
freehire whose key it is. The user never sees an account id: handing them one
would invite a typo that routes their sessions to someone else's machine.

**Staying safe — what the server may and may not say.** The server names a
harness from a set this binary knows (`harness.go`); it never describes a
process. No command path, no argv, no settings file, and the environment it may
add is allowlisted server-side. A compromised server must not become code
execution on a user's laptop, so `LookupHarness` matches exactly — no trimming,
no case folding, no path handling, because each of those turns a near-miss into
a match.

**Layout.**

```
internal/runner/
  device.go    stable per-machine id (~/.freehire/runner-id), survives restarts
  harness.go   the allowlist: identifier → command, args, env to strip
  tunnel.go    wire types mirroring the server's tunnel protocol (version 2)
  token.go     fhk_ key → short-lived session token
  link.go      the WebSocket, and the registration that opens it
  session.go   one harness per stream over one connection
  process.go   os/exec plumbing: line framing, env merge, process-group kill
  runner.go    connect, serve, reconnect with capped jittered backoff
```

`session.go` is written against two interfaces — a `link` and a `process` — so
its behaviour is tested without a WebSocket or a real binary.

**Things learned the hard way**, each now covered by a test:

- The working directory must come from *this* side. The server's workspace path
  does not exist here, and a harness handed one dies opening it. The runner
  creates a scratch dir per session and names it in `opened`.
- `CLAUDECODE` must be stripped: Claude Code refuses to start inside another
  Claude Code session, which a developer running this from one inherits.
- The harness's exit must be reported. Without it the server waits on a stream
  that will never produce again, and a turn hangs instead of failing.
- Killing must signal the process group; claude-code-acp spawns children.

**Deliberately absent:** no queueing while offline (a session that cannot reach
its device fails and the caller retries), no mid-turn reconnect (the agent
protocol has no resumable turn), and no persisted token (short-lived, re-minted
on start).

## Config & auth (`config`)

- Effective token + API URL resolve with precedence: **env (`FREEHIRE_TOKEN` /
  `FREEHIRE_API_URL`) > `~/.freehire/creds.json` > default `https://freehire.me`**.
  The token is required (else a "run `freehire auth login`" error).
- `creds.json` = `{"token":"fhk_…","api_url":"…"}`, file mode `0600`, dir `0700`.
- `Load` (missing file → zero, no error), `Save`, `Remove`, `Resolve(getenv)`.

## API client (`client`)

- `New(baseURL, token, *http.Client)`; sends `Authorization: Bearer <token>` on
  every request; base path `/api/v1`.
- Parses the `{ "data": …, "meta": …, "error": … }` envelope. Non-2xx → `*APIError`
  carrying the status (`401` → "unauthorized: run `freehire auth login`", `404`,
  `5xx`).
- Methods return the raw `data` (so `--json` is faithful to the API) and the cli
  decodes typed structs for human output:
  - `Me` → `GET /auth/me` (whoami; works by key).
  - `Search(q, limit, offset, facets)` → `GET /agent/jobs/search` (+ `meta.total`)
    — the programmatic variant of the web's `/jobs/search`: same query, but each
    hit carries the job's full description instead of the index's truncated
    preview. Always requested as markdown (`include_description=true`,
    `description_format=markdown`), so a result set is readable without a
    follow-up `GetJob` per hit.
  - `GetJob(slug)` → `GET /jobs/:slug`; `GetCompany(slug)` → `GET /companies/:slug`.
  - `Apply`/`Save(slug)` → `POST`; `Unsave(slug)` → `DELETE /jobs/:slug/{apply,save}`.
  - `MyJobs(filter, limit, offset)` → `GET /me/tracking` (+ `meta.total`).
  - `Track(slug, {stage?, notes?})` → `PATCH /jobs/:slug/track` (partial update;
    a nil field is omitted so the server leaves that column unchanged).
  - `Coverage({skills, facets})` → `POST /market/coverage` (skills in the JSON
    body, facets as the query string) — market coverage for a skill list.
  - `Facets(facets)` → `GET /jobs/facets` — the market's facet-value distributions
    (the filter/skill vocabulary) under an optional filter.

## Facet filters (`facets.go`)

`search` and `market-fit` share one filter surface (`addFacetFlags` /
`facetsFromFlags`): named convenience flags for the high-traffic facets
(`--remote`, `--region`, `--country`, `--city`, `--company`, `--category`,
`--role`, `--seniority`, `--employment-type`, `--english-level`, `--salary-min`,
`--visa`) plus a generic `--facet key=value` (repeatable) that reaches any facet
param in the API vocabulary. `--skills` is intentionally NOT shared: it filters in
`search` (facet) but is the measured set in `market-fit` (body).

## Commands (`cli`, cobra)

- Global flags: `--json` (raw API data), `--api-url` (override).
- `freehire auth login [--token fhk_…]` — token via flag or stdin prompt;
  validates with `Me`; writes creds; prints `Logged in as <email>`.
  `auth status` — `Me` → `Authenticated as <email> @ <api_url>` or not.
  `auth logout` — removes creds.
- `freehire facets [<facet flags>]` — every filter's live values with counts (+ the
  skills vocabulary and numeric stat ranges); the discovery step so an agent picks
  real values. Count-descending, `--top` caps per-facet values; or raw `--json`.
- `freehire search <query> [--limit --offset <facet flags> --skills]` —
  table (title · company · location · slug) or raw `--json`; the `--json` payload
  also carries each hit's full description as markdown.
- `freehire market-fit --skills <s,…> [<facet flags>]` — market coverage for the
  skill list: `Coverage: N% (covered/total)`, must-have held, and the biggest
  missing-skill gaps; or raw `--json`. One `--skills` value probes a single skill.
- `freehire job <slug>` — job content (title, company + slug, location, url,
  description) or raw `--json`.
- `freehire company <slug>` — the company and its open jobs.
- `freehire apply <slug>` — marks applied; `Marked applied: <slug>` or raw.
- `freehire save|unsave <slug>` — bookmark / remove a bookmark.
- `freehire stage <slug> <stage>` — set application stage (server-validated).
- `freehire note <slug> <text>…` — attach a free-text note (trailing args joined).
- `freehire my [--filter all|viewed|saved|applied]` — tracked jobs with stage + note.
- Errors → stderr, exit code ≠ 0; 401 → "run `freehire auth login`".

## Testing (TDD)

- `config`: save/load round-trip, `0600` perms, env precedence, `ErrNoToken`
  (temp `$HOME`).
- `client`: against `httptest.Server` — Bearer header set; `Me`/`Search`/`GetJob`/
  `GetCompany`/`Apply`/`Save`/`MyJobs`/`Track` happy paths (`Track` sends `PATCH`
  + `Content-Type`, omits nil fields); `401`/`404`/`5xx` → `*APIError`.
- `cli`: command wiring against an `httptest.Server` (login writes creds; search/
  apply/stage/note/company hit the right path with the token; `my` shows stage +
  note; `--json` passthrough).
- No real network/prod in tests.

## Distribution

`go install github.com/strelov1/freehire-cli/cmd/freehire@latest` (binary `freehire`).
README with agent examples; minimal GitHub Actions CI (build + test).
