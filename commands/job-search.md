---
description: Search live IT vacancies on freehire and shortlist the ones worth applying to
argument-hint: [what they are looking for, in their own words]
allowed-tools: Bash(freehire:*)
---

Find jobs on freehire for: **$ARGUMENTS**

Follow the `freehire-job-search` skill. In short:

1. Run `freehire --json profile` first. The person has already saved the roles,
   skills, geography and work format they want — search on that, say what you
   searched on, and ask only about what the profile does not answer. If the
   request above is empty, the profile *is* the request.
2. Run `freehire --json facets` (scoped with `--category` when the role is
   known) before choosing any filter value, so every value and skill slug is one
   the market actually uses.
3. Search with keyword plus facet filters, `--limit 20`. The JSON carries each
   hit's full description, so screen the set in that one call rather than
   fetching postings one by one.
4. Shortlist. For each job you put forward give the title, company, location,
   salary if stated, and one line on why it fits *this* person. Link it as
   `https://freehire.me/jobs/<public_slug>`, one per line.
5. If a posting carries a `ghost` object, relay it with its hedge and its scale
   ("2 of 4 checks fired") and never as "this job is fake".

If they name a company or paste a link freehire has no results for, run
`freehire contribute <url>` rather than reporting a dead end.

End by asking whether to mark any of them applied or saved.
