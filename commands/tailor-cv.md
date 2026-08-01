---
description: Reframe a CV toward one vacancy, citing evidence for every claim
argument-hint: [CV id from the tailoring workspace URL]
allowed-tools: Bash(freehire:*)
---

Tailor the CV: **$ARGUMENTS**

Follow the `freehire-tailor-cv` skill. In short:

1. The argument is the opaque CV id from the tailoring workspace URL
   (`/tailor/<job>?cv=<id>`). If it is missing, ask for it — never construct or
   guess one.
2. Read `freehire cv context <id>` (the fit analysis) and `freehire cv get <id>`
   (the current document) before proposing any edit.
3. Split the work by what `context` says:
   - `missing_have` — they have the evidence, the CV omits it. **Reframe** an
     existing bullet toward the vacancy's language, citing the atom.
   - `missing_gap` — a genuine gap. **Ask them first** whether they have real
     experience, and how they used it. On "no", leave it out; a gap belongs in
     the cover letter, never keyword-stuffed into the CV.
4. Send edits that belong together in ONE `freehire cv edit` call — one entry in
   their history, one round trip, all-or-nothing.
5. Re-render with `freehire cv render <id> --out cv.pdf` and check it stayed
   within 1–2 pages.

**Never write a claim you cannot cite.** Anything stating what the candidate did
needs `--evidence <atom-id>` — the id of something they asserted themselves. The
server refuses an uncited batch outright, and that refusal is the feature, not an
obstacle to route around.

Show them what you changed before you change it, and remind them every edit can
be undone individually from the workspace.
