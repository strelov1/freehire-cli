---
name: freehire-track-applications
description: Use when recording or reviewing what someone applied to on freehire — marking a job applied, bookmarking one for later, moving an application through its stages (applied through offer, or rejected/withdrawn), attaching a note, or listing what they are tracking. Also covers reporting a posting that never answered them after they applied (`ghost report`), and retracting that report.
---

# Tracking applications on freehire

Needs a key: `freehire auth login --token fhk_…` (or `FREEHIRE_TOKEN`). Every
command takes `--json`. To *find* the jobs in the first place, use the
**freehire-job-search** skill; every command here takes the same `<slug>`.

## Mark, stage, review

```bash
freehire apply <slug>                    # mark applied
freehire save <slug> / unsave <slug>     # bookmark
freehire stage <slug> <stage>            # applied…offer, or rejected/withdrawn
freehire note <slug> a quick reminder    # free-text note (trailing args; no quotes needed)
freehire my --filter applied             # tracked jobs with stage + note (all|viewed|saved|applied)
```

`apply` and `save` are idempotent, so retries are safe. The API owns the stage
vocabulary — the CLI only lists it in help, it does not validate client-side, so
a rejected stage name is the server telling you the truth.

`my` is the answer to "where am I with everything": it carries each job's stage
and note. A posting that has since been pruned from the catalogue shows as a
dash rather than a blank row.

## A posting that never answered

If the person applied and was never answered, file it — that is the only channel
that gets human evidence into the "possibly inactive" signal other people read:

```bash
freehire ghost report <slug> --applied-on 2026-06-01   # the day THEY applied
freehire ghost retract <slug>                          # withdraw it
```

`--applied-on` is required and never guessed: ask the person, or take it from
`freehire my --filter applied`. The date is the substance of the claim — it decides
when the silence has matured (roughly three weeks) — so a date they did not state is a
claim they did not make.

The rest is the server's: it needs a verified email address (403 otherwise), refuses a
future date or one older than a year (400), a closed posting or a second report on the
same job (409), and caps how many you can file in a day (429). One report is one of
four checks; the stronger wording needs a second person, which is also why a report is
never displayed as coming from one identifiable applicant.

Nothing here reaches a moderator and nothing here closes a job. Do not confuse it
with `freehire contribute` (handing over a job link) or the moderator queue behind
`freehire submissions`.
