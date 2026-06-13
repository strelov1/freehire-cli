# freehire-cli — design

A small Go CLI that lets agents (and humans) use the [freehire](https://freehire.dev)
API without a browser, authenticating with a personal API key. The key is stored
in `~/.freehire/creds.json`.

## Goals / Non-Goals

**Goals:** a single static binary (`freehire`) any agent can run; authenticate
with an `fhk_…` API key; the core agent loop — `search` jobs, fetch a job's
content (`job`), and `apply`; persist the token at `~/.freehire/creds.json`;
machine-readable `--json` output for agents.

**Non-Goals (YAGNI):** save/unsave, `my` listing, companies (easy to add later);
shell-completion docs; goreleaser/homebrew; semantic-search flags.

## Layout

```
main.go                 # thin: cli.Execute()
internal/
  config/   creds.json load/save (0600) + env/default resolution
  client/   API client over net/http: Me, Search, GetJob, Apply (+ APIError)
  cli/      cobra commands: root, auth (login/status/logout), search, job, apply
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
  - `Search(q, limit, offset)` → `GET /jobs/search` (+ `meta.total`).
  - `GetJob(slug)` → `GET /jobs/:slug`.
  - `Apply(slug)` → `POST /jobs/:slug/apply`.

## Commands (`cli`, cobra)

- Global flags: `--json` (raw API data), `--api-url` (override).
- `freehire auth login [--token fhk_…]` — token via flag or stdin prompt;
  validates with `Me`; writes creds; prints `Logged in as <email>`.
  `auth status` — `Me` → `Authenticated as <email> @ <api_url>` or not.
  `auth logout` — removes creds.
- `freehire search <query> [--limit --offset]` — table (title · company ·
  location · slug) or raw `--json`.
- `freehire job <slug>` — job content (title, company, location, url,
  description) or raw `--json`.
- `freehire apply <slug>` — marks applied; `Marked applied: <slug>` or raw.
- Errors → stderr, exit code ≠ 0; 401 → "run `freehire auth login`".

## Testing (TDD)

- `config`: save/load round-trip, `0600` perms, env precedence, `ErrNoToken`
  (temp `$HOME`).
- `client`: against `httptest.Server` — Bearer header set; `Me`/`Search`/`GetJob`/
  `Apply` happy paths; `401`/`404`/`5xx` → `*APIError`.
- `cli`: command wiring against an `httptest.Server` (login writes creds; search/
  apply hit the right path with the token; `--json` passthrough).
- No real network/prod in tests.

## Distribution

`go install github.com/strelov1/freehire-cli@latest` (binary `freehire`).
README with agent examples; minimal GitHub Actions CI (build + test).
