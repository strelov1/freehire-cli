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
freehire profile                                               # your saved roles, skills, geography and CV (no contacts)
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
freehire inbox push < mail.json                                # upload mail your own client fetched
freehire inbox list --unclassified --body                      # mail still awaiting a verdict, with its text
freehire inbox triage <id> rejection --slug <slug>             # record what a message is + link it
freehire inbox list --link suggested                          # matcher proposals awaiting your word
freehire inbox confirm <id>                                   # accept one of them
freehire inbox application <id> <slug>                        # mail about an untracked application
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

## Application mail — bring your own client

freehire does not fetch your mail. Your own client does — [himalaya], `mbsync`,
`notmuch`, anything that can emit JSON — and `inbox push` hands the result over.
Your agent then reads it and records what each message is, so the tracker stays
current without freehire running a mail connector or a classifier for you.

[himalaya]: https://github.com/pimalaya/himalaya

```bash
himalaya envelope list --output json | jq '[.[] | {
    external_id: .message_id, from_addr: .from.addr, from_name: .from.name,
    subject: .subject, received_at: .date }]' | freehire inbox push

freehire --json inbox list --unclassified --body --limit 50   # what still needs judging, with its text
freehire inbox triage 812 rejection --slug go-dev-acme-t35nijto
```

`external_id` is the deduplication key — use the message's `Message-ID`. Re-pushing
the same id updates that message instead of storing a copy, and never un-reads it,
resurrects a deleted one, or overwrites a verdict. Batches hold up to 100 messages.

`inbox list --body` returns each message's readable text (HTML-only ATS mail arrives
stripped) and marks nothing read — unlike `inbox read`, which is for opening a single
message a person asked to see. Triage signals: `acknowledgement`, `screening`,
`interview_invitation`, `assessment`, `offer`, `rejection`, `info_request`,
`incomplete_application`, `other`. A forward signal on a linked message advances that
application's stage; a settled application is never dragged back into the pipeline.

Only a deterministic match links mail automatically; anything the classifier merely
believes lands as a *suggestion* awaiting your word. `inbox list --link suggested` is
that queue, drained with `inbox confirm <id>` or `inbox reject <id>`. An unconfirmed
suggestion is a link that never happens, so a mailbox nobody confirms looks far less
matched than it is.

`inbox list --link unlinked` is the other queue: mail with no application to attach
to, usually because the application was never recorded. `inbox link` cannot help
there — it needs something to point at. `inbox application <id> <slug>` creates the
application and links the message in one call, dating it by the message rather than
by now(), since the employer replied to something that already existed.

Corrections: `inbox link`/`unlink`, `inbox delete`/`restore`, `inbox read-all`.

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
