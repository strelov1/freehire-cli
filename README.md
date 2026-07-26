# freehire CLI

A small Go CLI over the [freehire](https://freehire.me) job API — built so an
**agent** (or a human) can search, open, and apply to jobs from the terminal,
without a browser. Authenticate once with a personal API key; the key is stored
in `~/.freehire/creds.json`.

## Install

**curl** (prebuilt binary, no Go needed):

```bash
curl -fsSL https://freehire.me/install.sh | sh
```

**Go:**

```bash
go install github.com/strelov1/freehire-cli/cmd/freehire@latest   # installs the `freehire` binary
```

## Authenticate

Create an API key in the web app (freehire.me → account menu → **API keys**),
then:

```bash
freehire auth login --token fhk_xxxxxxxx   # validates the key and stores it
freehire auth status                       # Authenticated as you@example.com @ https://freehire.me
freehire auth logout                       # removes ~/.freehire/creds.json
```

`auth login` validates the key against the API before saving, so a bad key is
never stored. Omit `--token` to be prompted on stdin.

## Use

```bash
freehire facets                                                # list every filter's live values + counts (what to filter by)
freehire search "golang"                                       # list matching jobs (title · company · location · slug)
freehire search "backend" --remote --region eu --company acme  # facet filters (repeatable: --region, --company)
freehire search "data" --country BR --employment-type full_time --facet source=greenhouse  # any facet via --facet key=value
freehire market-fit --skills go,docker,react --category backend  # how much of the market your skills cover (+ gaps)
freehire market-fit --skills go --country BR                    # one skill probes that skill's demand under the filter
freehire job <slug>                                            # show a job's full content (incl. posting URL + company slug)
freehire company <slug>                                        # show a company and its open jobs
freehire apply <slug>                                          # mark a job applied for your account
freehire save <slug>                                           # bookmark a job for later
freehire unsave <slug>                                         # remove a bookmark
freehire stage <slug> <stage>                                  # set application stage (applied→…→offer, or rejected/withdrawn)
freehire note <slug> a quick reminder                          # attach a free-text note (trailing args; no quotes needed)
freehire my --filter applied                                   # tracked jobs, showing stage + note (all|viewed|saved|applied)
```

**Discovering values.** `freehire facets [filters]` lists every filter's live
values with a vacancy count each (and the `skills` vocabulary), so you pass real
values to `search`/`market-fit` instead of guessing. Narrow it with any filter flag
to see the values for that slice (e.g. `freehire facets --category backend`).

**Filters.** `search` and `market-fit` share the same market-filter flags:
`--remote`, `--region`, `--country`, `--city`, `--company`, `--category`, `--role`,
`--seniority`, `--employment-type`, `--english-level`, `--salary-min`, `--visa`
(each named facet is repeatable). Any other facet in the API's vocabulary is
reachable with `--facet key=value` (repeatable), e.g. `--facet source=greenhouse`
or `--facet company_size=startup`.

**market-fit** measures how much of the live open-vacancy market your skills reach
for a filtered role: the headline coverage (`N%` of vacancies list ≥1 of your
skills), the must-have skills you hold, and the missing skills that unlock the most
new vacancies. Pass your skills with `--skills` (comma-separated or repeated) — one
skill probes that single skill's demand; a list scores your whole stack. Note that
here `--skills` is the *measured set*, not a filter (in `search`, `--skills` filters
to jobs listing the skill).

Moderators can author postings (requires the `moderator` role; a regular key gets 403):

```bash
freehire jobs add --url https://acme.example/jobs/1 --title "Senior Go Developer" --company Acme --remote
freehire jobs add --url <url> --source workatastartup --title <t> --company <c> --description "<p>…HTML…</p>"
freehire jobs edit <slug> --title "Staff Go Developer"         # partial: only the flags you pass change
```

`--source` records the posting's real origin (defaults to `manual`); it does not change that
the job is flagged as manually added (that comes from the moderator authorship). `--description`
is stored and rendered as HTML, so pass HTML markup. The URL is the dedup key — re-running `add`
with the same URL updates the posting.

## Bring your own Claude (BYOK)

The freehire assistant normally runs its coding harness on freehire's servers.
`freehire runner` lets it run on **your** machine instead, using **your** Claude
subscription — your model credentials never leave your computer.

```bash
freehire runner
```

Leave it running and use the assistant at
[freehire.me/my/assistant](https://freehire.me/my/assistant) as usual. New
sessions are routed to your machine automatically. Stop with Ctrl-C and they go
back to being server-hosted.

**What you need:** the harness binary, installed and logged in.

```bash
npm i -g @zed-industries/claude-code-acp   # the harness
claude                                     # log in to Claude Code once
```

### What actually runs where

| On your machine | On freehire's servers |
|---|---|
| Claude Code and your credentials | the conversation and its history |
| the model's answers | scheduling, the web UI, your CV and job data |
| a scratch directory per session | which device a session belongs to |

Your Claude credentials are never sent anywhere. The server asks your machine to
start a harness and exchanges the agent protocol with it; the model runs here.

### Why you might want this

- **Your credentials stay yours.** freehire never stores your Claude token.
- **Your subscription, your limits.** Turns are billed to your Claude account.

### Why you might not

- **It only works while the runner runs.** Close the laptop and the assistant
  has nowhere to run; sessions fail until it is back. Nothing queues up.
- **Less isolation than the server has.** freehire's own host restricts what an
  agent can reach on the network. We cannot do that on your machine, so the
  runner instead confines the agent to `freehire` commands only.

### Seeing what it is doing

The runner reports every request it gets:

```
device 63723d49c78481eac2da6b66208a4a7c offering [claude]
connected to https://agent.freehire.dev — waiting for sessions
session 0: server asked for harness "claude"
session 0: started claude-code-acp in ~/.freehire/runner/sessions/0
session 0: harness exited (code 0) after 42 messages
```

Add `--verbose` to log every protocol message — useful when a turn misbehaves,
noisy otherwise (one turn is hundreds of them).

If nothing appears when you send a message, the session was probably created
before the runner connected: routing is decided once, when a session starts.
Begin a new one.

### What the agent may do on your machine

The runner writes a Claude Code config into each session directory that confines
the agent hard:

- **allowed:** `freehire …` commands, skills
- **denied:** editing or writing files, `Task`, `Glob`, `Grep`, `LS`, and —
  unlike on freehire's servers — `WebFetch` and `WebSearch`

The allow-list alone is not a boundary: it does not see `$(…)`, `;`, pipes or
redirection. So a `PreToolUse` hook (`freehire bash-guard`) inspects every Bash
command and denies anything that is not a single clean `freehire` invocation.

`WebFetch`/`WebSearch` stay enabled on freehire's own servers because that host
allowlists outbound traffic, so a successful prompt injection reaches nothing.
Your laptop has no such layer, and the agent reads untrusted text — job
postings — for a living. `freehire` is the only way out here.

### Notes

- Your device gets a stable id in `~/.freehire/runner-id`, so a session can find
  the same machine again after a reconnect.
- Sessions work in `~/.freehire/runner/sessions/<n>/`. It is scratch space —
  your CV, applications and history live on the server.
- The runner authenticates with the API key you already stored; there is nothing
  extra to configure.

## For agents

Pass `--json` for the raw API payload (faithful to the API; ideal for piping):

```bash
freehire --json search "site reliability" --limit 5 | jq '.[].public_slug'
freehire --json search "golang" --limit 3 | jq -r '.[].description'   # full postings, no extra call
freehire --json job <slug> | jq '{title, url}'
```

`search` reads the API's agent endpoint, so every hit already carries the job's
**full description as markdown** — an agent can judge a result set without a
`job <slug>` call per hit. The human table view stays compact (title · company ·
location · slug); the descriptions are in the `--json` payload.

Conventions: results go to **stdout**, errors to **stderr**, and a non-zero exit
code signals failure (e.g. an unauthenticated call exits non-zero with
`run \`freehire auth login\``).

An agent **skill** ships in [`skills/using-freehire/SKILL.md`](skills/using-freehire/SKILL.md):
it teaches the discover → search → apply loop, including `facets` and `market-fit`.
Drop it into a Claude Code (or compatible) skills directory, or install this repo as a
**Claude Code plugin** to get the skill wired up automatically:

```
/plugin marketplace add strelov1/freehire-cli
/plugin install freehire@freehire-cli
```

## Configuration

| What | Source (precedence: env → creds file → default) |
|------|--------------------------------------------------|
| Token | `FREEHIRE_TOKEN` → `~/.freehire/creds.json` |
| API base URL | `FREEHIRE_API_URL` → creds file → `https://freehire.me` |
| Assistant server (`runner`) | `--server` → `https://agent.freehire.dev` |
| Device id (`runner`) | `~/.freehire/runner-id`, generated on first run |

The token can be supplied entirely via `FREEHIRE_TOKEN` (no stored file needed),
which suits CI and ephemeral agent sandboxes. `--api-url` overrides the base URL
for a single invocation (e.g. pointing at a local dev server).

## Develop

```bash
go test ./...        # unit tests (config + client + cli), no network
go build ./...
```

## License

MIT — see [LICENSE](LICENSE). The freehire backend is MIT too.
