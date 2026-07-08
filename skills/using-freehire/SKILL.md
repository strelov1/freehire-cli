---
name: using-freehire
description: Use when searching, filtering, or applying to IT jobs from the terminal via the `freehire` CLI, when an agent needs to discover the job market's filter vocabulary (categories, seniorities, regions, skills), or when measuring a CV's skills against live market demand. Covers auth, market vocabulary discovery, keyword + facet search, market-fit coverage, and application tracking — all with machine-readable `--json` output.
---

# Using the freehire CLI

`freehire` is a single static binary over the [freehire.dev](https://freehire.dev)
job API. It lets an agent search, filter, and apply to IT jobs without a browser,
authenticating with a personal API key. Every command supports `--json` for a raw,
faithful API payload (pipe to `jq`).

## Setup (once)

```bash
freehire auth login --token fhk_xxxxxxxx   # validates the key and stores it (~/.freehire/creds.json)
freehire auth status                       # who am I / which API
```

The key can also come from `FREEHIRE_TOKEN` (no stored file — good for sandboxes).
`--api-url` overrides the base URL for one call. Errors go to stderr with a
non-zero exit; a 401 means "run `freehire auth login`".

## The core loop

**1. Discover what you can filter by — before guessing values.**

```bash
freehire facets                       # every filter's live values + counts, and skills
freehire facets --category backend    # values relevant to a slice (e.g. backend skills)
freehire --json facets | jq '.facets.skills'   # canonical skill slugs with demand
```

`facets` is the vocabulary source of truth: it returns each facet's valid values
(`category`, `seniority`, `regions`, `countries`, `role`, `skills`, `english_level`,
…) with a vacancy count each, plus numeric ranges (`salary_min`, …). **Read it first**
so search/market-fit filters use real values and skills use canonical slugs.

**2. Search with keyword + facet filters.**

```bash
freehire search "golang" --remote --region eu --seniority senior
freehire search "data" --country BR --employment-type full_time
freehire search "backend" --facet source=greenhouse   # any facet via --facet key=value
```

Named flags: `--remote --region --country --city --company --category --role
--seniority --employment-type --english-level --salary-min --visa` (each repeatable).
`--facet key=value` reaches any other facet in the vocabulary. `--skills` here is a
*filter* (jobs listing the skill). `--limit`/`--offset` page.

**3. Open, apply, track.**

```bash
freehire job <slug>                    # full content + posting URL + company slug
freehire company <slug>                # a company and its open jobs
freehire apply <slug>                  # mark applied
freehire save <slug> / unsave <slug>   # bookmark
freehire stage <slug> <stage>          # application stage (applied…offer, or rejected/withdrawn)
freehire note <slug> a quick reminder  # free-text note (no quotes needed)
freehire my --filter applied           # your tracked jobs with stage + note
```

## Market-fit: how well do a CV's skills cover the market

`market-fit` scores a skill list against the live open-vacancy market for a
filtered role: the headline coverage (`N%` of vacancies list ≥1 of the skills), the
must-have skills held, and the missing skills that unlock the most new vacancies.

```bash
freehire market-fit --skills go,docker,react --category backend   # score a whole stack
freehire market-fit --skills go --country BR                      # one skill = its demand under the filter
freehire --json market-fit --skills go,react --seniority senior | jq '{coverage_percent, gaps}'
```

Here `--skills` is the **measured set** (comma-separated or repeated), *not* a
filter — it takes the same facet flags as `search` to define the role. Use it to
tell a candidate which in-demand skills they are missing, or to gauge a single
skill's market demand.

## Tips for agents

- Prefer `--json` and parse with `jq`; human output is for people.
- Start from `freehire facets` to ground every filter value and skill slug in what
  the market actually has — do not invent facet values.
- Skills are canonical slugs (e.g. `go`, `react`, `kubernetes`), lowercase; take
  them from `facets` → `skills`.
- Commands are idempotent where it matters (`apply`, `save`), so retries are safe.
