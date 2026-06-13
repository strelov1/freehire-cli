# freehire CLI

A small Go CLI over the [freehire](https://freehire.dev) job API — built so an
**agent** (or a human) can search, open, and apply to jobs from the terminal,
without a browser. Authenticate once with a personal API key; the key is stored
in `~/.freehire/creds.json`.

## Install

```bash
go install github.com/strelov1/freehire-cli/cmd/freehire@latest   # installs the `freehire` binary
```

## Authenticate

Create an API key in the web app (freehire.dev → account menu → **API keys**),
then:

```bash
freehire auth login --token fhk_xxxxxxxx   # validates the key and stores it
freehire auth status                       # Authenticated as you@example.com @ https://freehire.dev
freehire auth logout                       # removes ~/.freehire/creds.json
```

`auth login` validates the key against the API before saving, so a bad key is
never stored. Omit `--token` to be prompted on stdin.

## Use

```bash
freehire search "golang remote"            # list matching jobs (title · company · location · slug)
freehire job <slug>                        # show a job's full content
freehire apply <slug>                      # mark a job applied for your account
```

## For agents

Pass `--json` for the raw API payload (faithful to the API; ideal for piping):

```bash
freehire --json search "site reliability" --limit 5 | jq '.[].public_slug'
freehire --json job <slug> | jq '{title, url}'
```

Conventions: results go to **stdout**, errors to **stderr**, and a non-zero exit
code signals failure (e.g. an unauthenticated call exits non-zero with
`run \`freehire auth login\``).

## Configuration

| What | Source (precedence: env → creds file → default) |
|------|--------------------------------------------------|
| Token | `FREEHIRE_TOKEN` → `~/.freehire/creds.json` |
| API base URL | `FREEHIRE_API_URL` → creds file → `https://freehire.dev` |

The token can be supplied entirely via `FREEHIRE_TOKEN` (no stored file needed),
which suits CI and ephemeral agent sandboxes. `--api-url` overrides the base URL
for a single invocation (e.g. pointing at a local dev server).

## Develop

```bash
go test ./...        # unit tests (config + client + cli), no network
go build ./...
```
