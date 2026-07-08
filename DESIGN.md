# freehire-cli — design

A small Go CLI that lets agents (and humans) use the [freehire](https://freehire.dev)
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

## Config & auth (`config`)

- Effective token + API URL resolve with precedence: **env (`FREEHIRE_TOKEN` /
  `FREEHIRE_API_URL`) > `~/.freehire/creds.json` > default `https://freehire.dev`**.
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
  - `Search(q, limit, offset, facets)` → `GET /jobs/search` (+ `meta.total`).
  - `GetJob(slug)` → `GET /jobs/:slug`; `GetCompany(slug)` → `GET /companies/:slug`.
  - `Apply`/`Save(slug)` → `POST`; `Unsave(slug)` → `DELETE /jobs/:slug/{apply,save}`.
  - `MyJobs(filter, limit, offset)` → `GET /me/jobs` (+ `meta.total`).
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
  table (title · company · location · slug) or raw `--json`.
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
