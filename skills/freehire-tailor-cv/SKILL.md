---
name: freehire-tailor-cv
description: Use when reframing someone's CV toward a specific vacancy through freehire — reading the fit analysis for a tailored copy, fetching or editing the CV document by path, rendering the ATS PDF, or deciding what may and may not be written into a CV. Carries the evidence rule: anything stating what the candidate DID needs an evidence atom they asserted themselves, and a genuine gap is asked about, never invented.
---

# Tailoring a CV to a vacancy (beta)

Needs a key: `freehire auth login --token fhk_…` (or `FREEHIRE_TOKEN`). These
commands act as the user whose key you signed in with, the same as every other
command.

After a fit analysis, a tailored CV can be reframed toward a specific vacancy. The
tailoring workspace on the site creates the tailored copy and shows its **CV id** —
an opaque identifier, visible in the workspace URL (`/tailor/<job>?cv=<id>`):

```bash
freehire cv context <id>              # the fit analysis to reframe toward (JSON)
freehire cv get <id>                  # the current CV document (JSON)
freehire cv edit <id> --set 'path=value'  # one edit; --ops '<json>' or stdin for several
freehire cv render <id> --out cv.pdf  # download the ATS PDF to inspect
```

The id is opaque: copy it, do not construct or guess one. An id that is not the
caller's own comes back as "not found" — the same answer as one that never existed.

## Editing by path

An edit is a kind (`set`, `insert`, `remove`, `move`) and a path into the document.
Paths reach everything: `summary`, `experience[2].bullets[1]`, `experience[0].stack[0]`,
`skills[0].items[3]`, `education[1].degree`, `certifications[0].issuer`, `style.font_size`.
Indices are 0-based, counted over what `cv get` returned.

```bash
freehire cv edit <id> --set 'summary=Senior backend engineer…'
freehire cv edit <id> --set 'experience[0].bullets[1]=Cut p99 latency 40%' --evidence <atom-id>
freehire cv edit <id> --insert 'experience[0].bullets[0]=Ran the migration' --evidence <atom-id>
freehire cv edit <id> --remove 'skills[2]'
freehire cv edit <id> --ops '[{"kind":"move","path":"experience[0].bullets[2]","to":0}]'
```

Send edits that belong together in ONE call: they land as one entry in the candidate's
history and cost one round instead of several. The whole batch applies or none of it does —
an unknown path or an index past the end is a 422 and the CV is untouched.

Every edit is recorded and can be undone on its own from the tailoring workspace, so the
candidate can see exactly what you changed and reverse any one of it.

## The honest wall — never fabricate

Editing with an API key edits as the tailoring agent, and the server holds you to it: the
candidate's own name, email, phone and links are refused, and anything stating what they
DID — a summary, a bullet, a technology, a skill — needs `--evidence <atom-id>`, the id of
something they asserted. An uncited edit refuses the whole batch. Read `cv context` and
split the work:

- `missing_have` requirements: the candidate *has* the evidence but the CV omits it —
  **reframe** an existing bullet toward the vacancy's language (`--set` on the bullet's
  path, grounded in what they already did).
- `missing_gap` requirements: a genuine gap — **ask the candidate first** ("do you know
  X? how did you use it?"). Only write it after they confirm real experience. On "no",
  leave it out; a gap belongs in the cover letter, never keyword-stuffed into the CV.
- A confirmed new fact goes into the tailored CV, and you should offer to also add it to
  the candidate's base CV (`freehire cv edit <base-id> …`) so future tailoring reuses it.

The server sanitizes and validates every patch; bad addressing is a 422. Re-render after
meaningful edits and keep the CV to 1–2 pages.

To measure a stack against the market rather than one vacancy, use the
**freehire-market-fit** skill.
