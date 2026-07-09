# freehire CLI

A small Go CLI over the [freehire](https://freehire.dev) job API — built so an
**agent** (or a human) can search, open, and apply to jobs from the terminal,
without a browser. Authenticate once with a personal API key; the key is stored
in `~/.freehire/creds.json`.

## Install

**curl** (prebuilt binary, no Go needed):

```bash
curl -fsSL https://freehire.dev/install.sh | sh
```

**Go:**

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

## For agents

Pass `--json` for the raw API payload (faithful to the API; ideal for piping):

```bash
freehire --json search "site reliability" --limit 5 | jq '.[].public_slug'
freehire --json job <slug> | jq '{title, url}'
```

Conventions: results go to **stdout**, errors to **stderr**, and a non-zero exit
code signals failure (e.g. an unauthenticated call exits non-zero with
`run \`freehire auth login\``).

An agent **skill** ships in [`skills/using-freehire/SKILL.md`](skills/using-freehire/SKILL.md):
drop it into a Claude Code (or compatible) skills directory and the agent learns the
discover → search → apply loop, including `facets` and `market-fit`.

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

## License

MIT — see [LICENSE](LICENSE). The freehire backend is MIT too.
