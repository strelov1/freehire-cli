---
description: Score a stack against live vacancy demand and name the gaps worth closing
argument-hint: [skills and/or the role to measure against]
allowed-tools: Bash(freehire:*)
---

Measure market fit for: **$ARGUMENTS**

Follow the `freehire-market-fit` skill. In short:

1. If no skills were named, take them from `freehire --json profile | jq '.skills'`.
2. Ground every slug against `freehire --json facets` — canonical, lowercase. A
   slug the market does not use scores nothing rather than erroring, so an
   ungrounded run quietly under-reports.
3. Run `freehire --json market-fit --skills <set>` with the facet flags that
   define the role they are aiming at (`--category`, `--seniority`, `--region`,
   `--remote`, …). Without a filter you are measuring the whole market, which is
   rarely the question.
4. Report: the coverage percent, how many vacancies were in scope, which
   must-have skills they hold, and the gaps.

State what was measured. Coverage means "this share of the filtered vacancies
name at least one of these skills" — it is not a pass rate and not an
employability score.

Give every gap its number ("adding Kubernetes reaches another 1,240 vacancies
under this filter"). A gap without a number is an opinion.
