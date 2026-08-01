---
name: freehire-market-fit
description: Use when measuring a set of skills — a CV's stack, or one skill on its own — against live open-vacancy demand on freehire; when telling a candidate which in-demand skills they are missing and what learning each would unlock; or when gauging how much of a filtered market (a role, a region, a seniority) their current stack already covers.
---

# Scoring a stack against the live market

Needs a key: `freehire auth login --token fhk_…` (or `FREEHIRE_TOKEN`).

`market-fit` scores a skill list against the live open-vacancy market for a
filtered role: the headline coverage (`N%` of vacancies list ≥1 of the skills), the
must-have skills held, and the missing skills that unlock the most new vacancies.

```bash
freehire market-fit --skills go,docker,react --category backend   # score a whole stack
freehire market-fit --skills go --country BR                      # one skill = its demand under the filter
freehire --json market-fit --skills go,react --seniority senior | jq '{coverage_percent, gaps}'
```

Here `--skills` is the **measured set** (comma-separated or repeated), *not* a
filter. It takes the same facet flags as `search` to define the role: `--remote
--region --country --city --company --category --role --seniority
--employment-type --english-level --salary-min --visa`.

**Ground the skill slugs first.** Skills are canonical lowercase slugs (`go`,
`react`, `kubernetes`), and a slug the market does not use scores nothing rather
than erroring:

```bash
freehire --json facets --category backend | jq '.facets.skills'
```

Where the stack comes from, if the person has not typed it out: `freehire --json
profile | jq '.skills'` returns what they already saved.

## Reading the result honestly

Coverage is a statement about *vacancy listings*, not about employability. A 70%
coverage means 70% of the filtered vacancies name at least one skill in the set —
it does not mean they would pass 70% of the screens. Say what was measured, under
which filter, and how many vacancies were in scope.

A gap is worth naming only with the number attached: "adding Kubernetes reaches
another 1,240 vacancies in this filter" is usable advice, "you should learn
Kubernetes" is not.

To act on a specific vacancy rather than the market as a whole, use the
**freehire-tailor-cv** skill.
