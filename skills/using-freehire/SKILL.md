---
name: using-freehire
description: Use when orienting in the `freehire` CLI as a whole — installing it, authenticating with an API key, working out which of its surfaces answers a job seeker's question, or troubleshooting a 401. The task skills do the work; this one is the map and the setup.
---

# The freehire CLI

`freehire` is a single static binary over the [freehire.me](https://freehire.me)
job API. It lets an agent search, filter, apply to and track IT jobs without a
browser, authenticating with a personal API key. Every command supports `--json`
for a raw, faithful API payload (pipe to `jq`).

## Install and authenticate (once)

```bash
curl -fsSL https://freehire.me/install.sh | sh   # or: go install github.com/strelov1/freehire-cli/cmd/freehire@latest

freehire auth login --token fhk_xxxxxxxx   # validates the key and stores it (~/.freehire/creds.json)
freehire auth status                       # who am I / which API
freehire auth logout                       # removes the stored key
```

Create the key in the web app: freehire.me → account menu → **API keys**.

The key can also come from `FREEHIRE_TOKEN` (no stored file — good for sandboxes),
which takes precedence over the stored one. `--api-url` overrides the base URL for
one call. Errors go to stderr with a non-zero exit; a 401 means "run `freehire auth
login`".

## Which skill does what

| The person wants to… | Skill |
|---|---|
| find jobs, read a posting, hand over a link freehire lacks | **freehire-job-search** |
| record and follow what they applied to; report a posting that never answered | **freehire-track-applications** |
| know how well their stack covers the live market, and what to learn | **freehire-market-fit** |
| reframe their CV toward one vacancy | **freehire-tailor-cv** |
| sort their application mail into the tracker | **freehire-mail-triage** |

## Three rules that hold everywhere

- **Prefer `--json` and parse with `jq`.** Human output is for people.
- **Ground every filter value in `freehire facets`** before using it. It is the
  vocabulary source of truth — facet values and canonical skill slugs (`go`,
  `react`, `kubernetes`, lowercase) both come from there. Do not invent values.
- **The API key is the user's whole account.** Keep it in the harness's secret
  store or `FREEHIRE_TOKEN`, never in a repo or a log. It can be revoked from the
  web app.
