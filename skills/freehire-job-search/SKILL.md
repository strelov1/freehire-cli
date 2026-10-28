---
name: freehire-job-search
description: Use when finding jobs for someone — searching or filtering the freehire catalogue by keyword, region, seniority, skills, salary or company; reading a posting or a company's open roles; discovering what values a filter accepts; reading the saved job-search profile before asking someone what they want; handing over a job link freehire does not carry yet; or reading the "possibly inactive" signal on a posting. Covers profile, facets, search, job, company and contribute, all with machine-readable `--json` output.
---

# Finding jobs on freehire

`freehire` is a single static binary over the [freehire.me](https://freehire.me)
job API. It searches, filters and opens jobs without a browser. Every command
supports `--json` for a raw, faithful API payload (pipe to `jq`).

Needs a key: `freehire auth login --token fhk_…` (or `FREEHIRE_TOKEN`). A 401
means "run `freehire auth login`".

## The loop

**0. Read the person's own profile before asking them anything.**

```bash
freehire profile                      # roles, skills, skills to avoid, geography, CV
freehire --json profile | jq '.skills, .cv.total_years'
```

They have already told freehire which roles and skills they want, which they
would rather avoid, and where and how they will work. Start from that and say what
you searched on; ask only about what it does not answer. It returns `null` data when
they have saved none — then point them at `https://freehire.me/my/profile` rather
than collecting the same answers in the conversation, because what they save there
also drives their recommendations and alerts. Contact details are never included.

**1. Discover what you can filter by — before guessing values.**

```bash
freehire facets                       # every filter's live values + counts, and skills
freehire facets --category backend    # values relevant to a slice (e.g. backend skills)
freehire --json facets | jq '.facets.skills'   # canonical skill slugs with demand
```

`facets` is the vocabulary source of truth: it returns each facet's valid values
(`category`, `seniority`, `regions`, `countries`, `role`, `skills`, `english_level`,
…) with a vacancy count each, plus numeric ranges (`salary_min`, …). **Read it first**
so search filters use real values and skills use canonical slugs. Skills are
lowercase slugs (`go`, `react`, `kubernetes`) — do not invent facet values.

**2. Search with keyword + facet filters.**

```bash
freehire search "golang" --remote --region eu --seniority senior
freehire search "data" --country BR --employment-type full_time
freehire search "AI" --country IT --employment-type contract --exclude-skill python
freehire search "backend" --facet source=greenhouse   # any facet via --facet key=value
freehire search "keep our payment systems secure" --semantic   # match by meaning, not only words
```

**Query syntax.** The keyword argument is more than a bag of words:

| Write | Means |
|---|---|
| `"product owner"` | every quoted word must match, in any order (not a contiguous phrase) |
| `-junior` | exclude postings that match it |
| `produktchef OR "product owner"` | any alternative matches; uppercase `OR`, at most 5 alternatives, merged and deduplicated server-side |
| `title:"product owner"` | at the start of an alternative: match it in the title only, so a posting that merely mentions the role in its body is left out |

```bash
freehire search 'title:"product owner" -junior OR title:produktchef'
freehire search 'golang OR rust' --remote
```

- **Single-quote the whole query** whenever it has double quotes. The shell
  strips unprotected quotes: `freehire search title:"product owner"` sends
  `title:product owner` and matched 59,406 postings instead of 6,028.
- **Prefer one `OR` call to one call per synonym.** Paging, the total and the
  counts already describe the merged set.
- **Plain words are neither AND nor OR.** The first ones are required and
  trailing ones are dropped when too few match, so word order changes the
  result. Quote the words that must all appear.
- **`--semantic` when the need is described rather than named.** "keep our
  payment systems secure" has no job title in it; with `--semantic` it also
  reaches postings that say the same thing in other words. **Leave it off for a
  job title** ("react developer") **or a non-technical role** ("nurse night
  shift"): there it measurably does worse than plain keywords, because only
  technical postings carry the vectors it matches on. A stderr warning
  `semantic search unavailable` means the results are keyword-only.

Repeatable filter flags (pass again to OR more values): `--region --country --city
--company --category --role --seniority --employment-type --english-level
--exclude-skill --skills --facet`. Single-valued: `--remote` and `--visa` are
toggles, `--salary-min` is one number. `--facet key=value` reaches any other facet
in the vocabulary. `--skills` here is a *filter* (jobs listing the skill).
`--limit`/`--offset` page.

**Filtering things OUT.** `--exclude-skill python` drops jobs **tagged** with that
skill — the way to say "contract work, but not the Python ones". Every other
facet takes the same `_exclude` suffix through `--facet`, e.g. `--facet
company_type_exclude=outstaff`.

Tags come from a curated dictionary read off the description, so treat an
exclusion as a *discovery* filter, not a fit test. Two ways it under-delivers: a
mention the dictionary does not recognise leaves the job tagless and in the
results, and excluding one language says nothing about the stack a job actually
runs on — `--exclude-skill python` still returns roles whose real core is cloud,
CI/CD or SQL.

**Geography widens, it does not narrow.** `--region`, `--country` and `--city`
are ONE OR-group: `--region eu --country IT` means "in Europe **or** in Italy" and
returns everything `--region eu` alone would. To search one country, pass the
country and **drop the region**. The three name a single concept — *where* — so
picking two places reads as "either", which is what makes `--region eu --country BR`
("Europe or Brazil") useful. There is no AND to switch on: `_mode=and` does not
apply to geography.

**Read the warnings on stderr.** A filter param the API does not recognize is
ignored, not refused — the search still runs, just wider. The CLI prints
`warning: ignored unknown filter "country" — did you mean "countries"?` to stderr
(stdout stays clean JSON). A known filter given a value outside its vocabulary
(including an `_exclude` value) prints
`warning: unrecognised skills value "golangg" — results may not be what you meant`.
Usually it narrows to nothing, so a typo reads as an empty market. Skill aliases
(`golang`) resolve on their own; check `freehire facets` for the spelling the
warning rejected. `search`, `facets` and `market-fit` all report both.
Never quote a number from a run that printed one: the count, the distribution or
the coverage percentage answers a broader question than the one asked.

`--json search` returns each hit's **full description as markdown**, so you can
screen a result set in one call — reach for `job <slug>` only when you need a
single posting's other detail. Keep `--limit` small when you read descriptions.

**3. Open what looks right.**

```bash
freehire job <slug>                    # full content + posting URL + company slug (CLOSED line if closed)
freehire similar <slug>                # more like this one, nearest first
freehire company <slug>                # a company and its open jobs
```

To mark, stage and follow what they applied to, use the
**freehire-track-applications** skill.

## Keep watching: job alerts

When the person wants to hear about new postings for a search, save it as an alert.
Run the search first so they see what it catches, then create it with the SAME flags:

```bash
freehire alerts add --skills go --remote --region eu --name "Go EU"
freehire alerts list                   # ids, filters, channels, paused
freehire alerts update <id> --salary-min 5000   # pass only what changes
freehire alerts pause <id>             # hold it; resume <id> brings it back
freehire alerts updates                # what the alerts matched lately
```

Alerts are delivered by email, Telegram, the app or a webhook — never to this
chat. On `update`, any filter flag replaces the alert's whole filter set, so read
it from `alerts list --json` (`.alerts[].search`) and pass every filter it should
keep. To stop one for a while, pause it rather than removing it.

## A job freehire doesn't have

When the person is looking at a vacancy that is not in the catalogue — they pasted a link,
or a search came up empty for a company you know is hiring — hand it over instead of
apologising:

```bash
freehire contribute https://acme.recruitee.com/o/senior-go
freehire --json contribute <url> | jq '.status, .public_slug'
freehire contributions                 # what you've handed over, and what became of it
```

One sequence answers, and `status` says which branch it took:

| `status` | What happened | What to do next |
|---|---|---|
| `found` | we already carry it | use the returned `public_slug` — apply, save, track |
| `tracked` | imported now; we already crawl this company | same, and say its other roles will follow |
| `imported` | imported, and its board is new to us — **+1 AI credit** | same |
| `queued` | nothing could read the page | tell them a maintainer will look; no credit yet |

Reach for this the moment a link is in hand. `found`, `tracked` and `imported` all come back
with a `public_slug`, which is the same handle every other command takes — so contributing a
link and then tracking the job is two calls, not a dead end.

Two things worth knowing so you don't mislead anyone. The credit is for the **board**, not
the vacancy: only the first person to name a company earns it, and later links to that
company are recorded but pay nothing. And `queued` does not mean rejected — it means we could
not read that particular page.

Do not use `freehire submissions` for this. That is the moderator review queue for
hand-authored job cards, a different feature; the `submit` command that fed it is gone.

## A posting that may not be real

`freehire job <slug>` prints a signal block above the description when a posting looks
like it is not being worked. It is computed fresh on every read, and most postings
carry nothing at all:

```
Possibly inactive — 2 of 4 checks fired
  · Posting behaves as evergreen
  · Not on the company's own careers board (checked 2026-07-30)
  Not observed: applications here, reports from people.
```

In `--json` the same thing is the `ghost` object: `level` (`possible` | `likely`),
the `criteria` that fired, `criteria_total`, and — only above the anonymity gate —
`contributors`.

**How to say this to a person.** Report the *facts*, never the intent. The checks
observe how a posting behaves, whether the employer's own board carries it, and what
happened to people who applied; none of them can see whether anyone meant to deceive.
So: "it has been reposted repeatedly and isn't on the company's own careers board" —
not "this is a fake job". Keep the hedge (`possible`, `likely`) and the scale (2 of 4)
when you relay it, and mention what was *not* observed: that is what says how sure the
signal is. It is a reason to check with the company or to lower expectations of a
reply, never a reason to tell someone a job is fake. Two checks out of four is a
lead, not a verdict.

Filing a report — when the person applied and was never answered — belongs to the
**freehire-track-applications** skill.

## Presenting results

Link each job you recommend as `https://freehire.me/jobs/<public_slug>`, one per
line, so a chat client unfurls it into a card.
