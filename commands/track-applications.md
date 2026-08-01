---
description: Review where every application stands and bring the tracker up to date
argument-hint: [optional: a job slug, or what changed]
allowed-tools: Bash(freehire:*)
---

Review and update tracked applications. Context: **$ARGUMENTS**

Follow the `freehire-track-applications` skill. In short:

1. Start from `freehire --json my --filter applied` — every tracked job with its
   stage and note. Read before writing.
2. Report where things stand: what is waiting on the employer, what is waiting on
   them, and what has gone quiet. Group by stage, not by date.
3. Apply whatever they tell you changed:
   `freehire stage <slug> <stage>` (applied…offer, or rejected/withdrawn),
   `freehire note <slug> …`, `freehire apply <slug>`, `freehire save <slug>`.
4. Flag anything applied over three weeks ago with no reply. If they confirm they
   were never answered, offer `freehire ghost report <slug> --applied-on <date>` —
   and take that date from them or from `my`, never from a guess. The date is the
   substance of the claim.

Do not invent a stage transition the person did not report. The tracker is their
record of what happened, and a stage you moved on their behalf is a fact they
never stated.
