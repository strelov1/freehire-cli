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
freehire experience list                                       # your whole experience bank (employments + achievements)
freehire experience employments add --company Acme --role SWE  # record a new job or project
freehire experience atoms add --claim "Cut latency 20s to 1s" --metric "20s->1s" --skill go  # record a new achievement
freehire facets                                                # list every filter's live values + counts (what to filter by)
freehire search "golang"                                       # list matching jobs (title · company · location · slug)
freehire search "backend" --remote --region eu --company acme  # facet filters (repeatable: --region, --company)
freehire search "data" --country BR --employment-type full_time --facet source=greenhouse  # any facet via --facet key=value
freehire market-fit --skills go,docker,react --category backend  # how much of the market your skills cover (+ gaps)
freehire market-fit --skills go --country BR                    # one skill probes that skill's demand under the filter
freehire job <slug>                                            # show a job's full content (incl. posting URL + company slug)
freehire company <slug>                                        # show a company and its open jobs
freehire apply <slug>                                          # mark a job applied for your account
freehire apply <slug> --on 2026-07-27                          # ...on the day you actually sent it
freehire save <slug>                                           # bookmark a job for later
freehire unsave <slug>                                         # remove a bookmark
freehire stage <slug> <stage>                                  # set application stage (applied→…→offer, or rejected/withdrawn)
freehire note <slug> a quick reminder                          # attach a free-text note (trailing args; no quotes needed)
freehire my --filter applied                                   # tracked jobs, showing stage + note (all|viewed|saved|applied)
freehire contribute <url>                                      # hand freehire a job link (see below)
freehire contributions                                         # boards you've contributed
freehire ghost report <slug> --applied-on 2026-06-01           # you applied that day and were never answered
freehire ghost retract <slug>                                  # withdraw that report
freehire inbox push < mail.json                                # upload mail your own client fetched
freehire inbox list --unclassified --body                      # mail still awaiting a verdict, with its text
freehire inbox triage <id> rejection --slug <slug>             # record what a message is + link it
freehire inbox list --link suggested                          # matcher proposals awaiting your word
freehire inbox confirm <id>                                   # accept one of them
freehire inbox application <id> <slug>                        # mail about an untracked application
```

**Experience bank.** `experience list` shows every achievement grouped under the
employment it belongs to (achievements with no place come back under `unplaced`) —
copy an id from there to attach a new achievement to it with `atoms add
--employment <id>`. Anything added this way is recorded with `manual` provenance,
which is what lets it later be cited as `cv edit --evidence <id>`. Correcting or
removing an existing entry is not exposed here — do that from the experience page
on the site.

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

## Contributing a link

`freehire contribute <url>` hands over any vacancy or ATS board link. One sequence answers,
whichever surface you use:

| Outcome | Meaning |
|---|---|
| already in the catalogue | we carry that posting; you get its URL |
| added, company already crawled | we read the page and imported it; the rest of that company's roles follow on the next crawl |
| added, company new to us | imported, and its board is queued for crawling — **+1 AI credit** |
| queued | nothing could read the page, so a maintainer will look; not credited |

The board, not the vacancy, is what earns the credit, and only the first person to name a
board earns it — later links to the same company are still recorded (they tell us the board
matters) but pay nothing.

```bash
freehire contribute https://acme.recruitee.com/o/senior-go
freehire --json contribute https://acme.recruitee.com/o/senior-go | jq '.status, .public_slug'
freehire contributions
```

## A posting that may not be real

`freehire job <slug>` shows a signal above the description when a posting looks like it is
not being worked — how it behaves, whether the employer's own careers board carries it, and
what happened to people who applied:

```
Possibly inactive — 2 of 4 checks fired
  · Posting behaves as evergreen
  · Not on the company's own careers board (checked 2026-07-30)
  Not observed: applications here, reports from people.
```

The wording is hedged on purpose, and the unfired checks are printed too: they are what says
how sure the signal is. These are observations about a posting, never a claim about anyone's
intent.

If you applied and were never answered, say so — human evidence is the only thing that can
raise the signal past what a posting's shape can show:

```bash
freehire ghost report senior-go-acme --applied-on 2026-06-01
freehire ghost retract senior-go-acme
```

The date is the day *you* applied, and it is required. Reporting needs a verified email
address, works once per posting, and reaches no moderator — nothing here closes a job.

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

## Agent skills and slash commands

Six agent **skills** ship in [`skills/`](skills/), one per task rather than one per
tool, so an agent loads what the question needs and not the whole surface:

| Skill | Covers |
|---|---|
| [`using-freehire`](skills/using-freehire/SKILL.md) | install, auth, and the map of the rest |
| [`freehire-job-search`](skills/freehire-job-search/SKILL.md) | profile → facets → search → job/company, contributing a link, reading the inactive-posting signal |
| [`freehire-track-applications`](skills/freehire-track-applications/SKILL.md) | apply, save, stage, note, `my`, and reporting a posting that never answered |
| [`freehire-market-fit`](skills/freehire-market-fit/SKILL.md) | scoring a stack against live vacancy demand |
| [`freehire-tailor-cv`](skills/freehire-tailor-cv/SKILL.md) | reframing a CV toward one vacancy, and the evidence rule |
| [`freehire-mail-triage`](skills/freehire-mail-triage/SKILL.md) | pushing, judging and linking application mail |

Drop them into a Claude Code (or compatible) skills directory, or install this repo as a
**Claude Code plugin** to get the skills *and* the slash commands wired up automatically:

```
/plugin marketplace add strelov1/freehire-cli
/plugin install freehire@freehire-cli
```

The plugin adds [`commands/`](commands/): `/job-search`, `/market-fit`,
`/tailor-cv`, `/track-applications` and `/triage-inbox` — each drives its skill's
workflow end to end from one line.

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
